/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/fluxcd/pkg/http/fetch"
	"github.com/fluxcd/pkg/tar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	yttcmd "github.com/vmware-tanzu/carvel-ytt/pkg/cmd/template"
	yttui "github.com/vmware-tanzu/carvel-ytt/pkg/cmd/ui"
	yttfiles "github.com/vmware-tanzu/carvel-ytt/pkg/files"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	extensionv1alpha1 "github.com/gianlucam76/ytt-controller/api/v1alpha1"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	libsveltosset "github.com/projectsveltos/libsveltos/lib/set"
)

// YttSourceReconciler reconciles a YttSource object
type YttSourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ConcurrentReconciles int
	PolicyMux            sync.Mutex                                    // use a Mutex to update Map as MaxConcurrentReconciles is higher than one
	ReferenceMap         map[corev1.ObjectReference]*libsveltosset.Set // key: Referenced object; value: set of all YTTSources referencing the resource
	YttSourceMap         map[types.NamespacedName]*libsveltosset.Set   // key: YTTSource namespace/name; value: set of referenced resources
}

//+kubebuilder:rbac:groups=extension.projectsveltos.io,resources=yttsources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=extension.projectsveltos.io,resources=yttsources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=extension.projectsveltos.io,resources=yttsources/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=gitrepositories,verbs=get;watch;list
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=gitrepositories/status,verbs=get;watch;list
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=ocirepositories,verbs=get;watch;list
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=ocirepositories/status,verbs=get;watch;list
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=buckets,verbs=get;watch;list
//+kubebuilder:rbac:groups="source.toolkit.fluxcd.io",resources=buckets/status,verbs=get;watch;list

func (r *YttSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(logs.LogInfo).Info("Reconciling")

	// Fecth the YttSource instance
	yttSource := &extensionv1alpha1.YttSource{}
	if err := r.Get(ctx, req.NamespacedName, yttSource); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to fetch YttSource")
		return reconcile.Result{}, errors.Wrapf(
			err,
			"Failed to fetch YttSource %s",
			req.NamespacedName,
		)
	}

	logger = logger.WithValues("yttSource", req.String())

	helper, err := patch.NewHelper(yttSource, r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to init patch helper")
	}

	// Always close the scope when exiting this function so we can persist any YttSource
	// changes.
	defer func() {
		err = r.Close(ctx, yttSource, helper)
		if err != nil {
			reterr = err
		}
	}()

	// Handle deleted YttSource
	if !yttSource.DeletionTimestamp.IsZero() {
		r.cleanMaps(yttSource)
		return reconcile.Result{}, nil
	}

	// Handle non-deleted YttSource
	var resources string
	resources, err = r.reconcileNormal(ctx, yttSource, logger)
	if err != nil {
		msg := err.Error()
		yttSource.Status.FailureMessage = &msg
		yttSource.Status.Resources = ""
	} else {
		yttSource.Status.FailureMessage = nil
		yttSource.Status.Resources = resources
	}

	return reconcile.Result{}, err
}

func (r *YttSourceReconciler) reconcileNormal(
	ctx context.Context,
	yttSource *extensionv1alpha1.YttSource,
	logger logr.Logger,
) (string, error) {

	logger.V(logs.LogInfo).Info("Reconciling YttSource")

	r.updateMaps(yttSource, logger)

	tmpDir, err := r.prepareFileSystem(ctx, yttSource, logger)
	if err != nil {
		return "", err
	}

	if tmpDir == "" {
		return "", nil
	}

	defer os.RemoveAll(tmpDir)

	// check build path exists
	dirPath := filepath.Join(tmpDir, yttSource.Spec.Path)
	_, err = os.Stat(dirPath)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("ytt path not found: %v", err))
		return "", err
	}

	// create and invoke ytt "template" command
	templatingOptions := yttcmd.NewOptions()

	input, err := templatesAsInput(dirPath, yttSource, logger)
	if err != nil {
		return "", err
	}

	// equivalent to `--data-value-yaml`
	templatingOptions.DataValuesFlags.KVsFromYAML = []string{}

	noopUI := yttui.NewCustomWriterTTY(false, noopWriter{}, noopWriter{})

	// Evaluate the template given the configured data values...
	output := templatingOptions.RunWithFiles(input, noopUI)
	if output.Err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to execute RunWithFiles: %v", output.Err))
		return "", output.Err
	}

	// output.DocSet contains the full set of resulting YAML documents, in order.
	bs, err := output.DocSet.AsBytes()
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to get result: %v", err))
		return "", err
	}

	logger.V(logs.LogInfo).Info("Reconciling YttSource success")
	return string(bs), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *YttSourceReconciler) SetupWithManager(mgr ctrl.Manager,
) (controller.Controller, error) {

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&extensionv1alpha1.YttSource{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Build(r)
	if err != nil {
		return nil, errors.Wrap(err, "error creating controller")
	}

	// When ConfigMap changes, according to ConfigMapPredicates,
	// one or more YttSources need to be reconciled.
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueYttSourceForReference),
		ConfigMapPredicates(mgr.GetLogger().WithValues("predicate", "configmappredicate")),
	)
	if err != nil {
		return nil, err
	}

	// When Secret changes, according to SecretPredicates,
	// one or more YttSources need to be reconciled.
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueYttSourceForReference),
		SecretPredicates(mgr.GetLogger().WithValues("predicate", "secretpredicate")),
	)

	return c, err
}

func (r *YttSourceReconciler) WatchForFlux(mgr ctrl.Manager, c controller.Controller) error {
	// When a Flux source (GitRepository/OCIRepository/Bucket) changes, one or more YttSources
	// need to be reconciled.

	err := c.Watch(&source.Kind{Type: &sourcev1.GitRepository{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueYttSourceForFluxSources),
		FluxSourcePredicates(r.Scheme, mgr.GetLogger().WithValues("predicate", "fluxsourcepredicate")),
	)
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &sourcev1b2.OCIRepository{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueYttSourceForFluxSources),
		FluxSourcePredicates(r.Scheme, mgr.GetLogger().WithValues("predicate", "fluxsourcepredicate")),
	)
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &sourcev1b2.Bucket{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueYttSourceForFluxSources),
		FluxSourcePredicates(r.Scheme, mgr.GetLogger().WithValues("predicate", "fluxsourcepredicate")),
	)
}

func (r *YttSourceReconciler) getReferenceMapForEntry(entry *corev1.ObjectReference) *libsveltosset.Set {
	s := r.ReferenceMap[*entry]
	if s == nil {
		s = &libsveltosset.Set{}
		r.ReferenceMap[*entry] = s
	}
	return s
}

func (r *YttSourceReconciler) getCurrentReference(yttSource *extensionv1alpha1.YttSource) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: getReferenceAPIVersion(yttSource),
		Kind:       yttSource.Spec.Kind,
		Namespace:  yttSource.Spec.Namespace,
		Name:       yttSource.Spec.Name,
	}
}

func (r *YttSourceReconciler) updateMaps(yttSource *extensionv1alpha1.YttSource, logger logr.Logger) {
	logger.V(logs.LogDebug).Info("update policy map")
	ref := r.getCurrentReference(yttSource)

	currentReference := &libsveltosset.Set{}
	currentReference.Insert(ref)

	r.PolicyMux.Lock()
	defer r.PolicyMux.Unlock()

	// Get list of References not referenced anymore by YttSource
	var toBeRemoved []corev1.ObjectReference
	yttSourceName := types.NamespacedName{Namespace: yttSource.Namespace, Name: yttSource.Name}
	if v, ok := r.YttSourceMap[yttSourceName]; ok {
		toBeRemoved = v.Difference(currentReference)
	}

	yttSourceInfo := getKeyFromObject(r.Scheme, yttSource)
	// For currently referenced instance, add YttSource as consumer
	r.getReferenceMapForEntry(ref).Insert(yttSourceInfo)

	// For each resource not reference anymore, remove YttSource as consumer
	for i := range toBeRemoved {
		referencedResource := toBeRemoved[i]
		r.getReferenceMapForEntry(&referencedResource).Erase(
			yttSourceInfo,
		)
	}

	// Update list of resources currently referenced by YttSource
	r.YttSourceMap[yttSourceName] = currentReference
}

func (r *YttSourceReconciler) cleanMaps(yttSource *extensionv1alpha1.YttSource) {
	r.PolicyMux.Lock()
	defer r.PolicyMux.Unlock()

	delete(r.YttSourceMap, types.NamespacedName{Namespace: yttSource.Namespace, Name: yttSource.Name})

	yttSourceInfo := getKeyFromObject(r.Scheme, yttSource)

	for i := range r.ReferenceMap {
		yttSourceSet := r.ReferenceMap[i]
		yttSourceSet.Erase(yttSourceInfo)
	}
}

func (r *YttSourceReconciler) prepareFileSystem(ctx context.Context,
	yttSource *extensionv1alpha1.YttSource, logger logr.Logger) (string, error) {

	ref := r.getCurrentReference(yttSource)

	if ref.Kind == string(libsveltosv1alpha1.ConfigMapReferencedResourceKind) {
		return prepareFileSystemWithConfigMap(ctx, r.Client, ref, logger)
	} else if ref.Kind == string(libsveltosv1alpha1.SecretReferencedResourceKind) {
		return prepareFileSystemWithSecret(ctx, r.Client, ref, logger)
	}

	return prepareFileSystemWithFluxSource(ctx, r.Client, ref, logger)
}

func prepareFileSystemWithConfigMap(ctx context.Context, c client.Client,
	ref *corev1.ObjectReference, logger logr.Logger) (string, error) {

	configMap, err := getConfigMap(ctx, c, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name})
	if err != nil {
		return "", err
	}

	return prepareFileSystemWithData(configMap.BinaryData, ref, logger)
}

func prepareFileSystemWithSecret(ctx context.Context, c client.Client,
	ref *corev1.ObjectReference, logger logr.Logger) (string, error) {

	secret, err := getSecret(ctx, c, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name})
	if err != nil {
		return "", err
	}

	return prepareFileSystemWithData(secret.Data, ref, logger)
}

func prepareFileSystemWithData(binaryData map[string][]byte,
	ref *corev1.ObjectReference, logger logr.Logger) (string, error) {

	key := "ytt.tar.gz"
	binaryTarGz, ok := binaryData[key]
	if !ok {
		return "", fmt.Errorf("%s missing", key)
	}

	// Create tmp dir.
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("ytt-%s-%s",
		ref.Namespace, ref.Name))
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
		return "", err
	}

	filePath := path.Join(tmpDir, key)

	err = os.WriteFile(filePath, binaryTarGz, permission0600)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to write file %s: %v", filePath, err))
		return "", err
	}

	tmpDir = path.Join(tmpDir, "extracted")

	err = extractTarGz(filePath, tmpDir)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to extract tar.gz: %v", err))
		return "", err
	}

	logger.V(logs.LogDebug).Info("extracted .tar.gz")
	return tmpDir, nil
}

func prepareFileSystemWithFluxSource(ctx context.Context, c client.Client,
	ref *corev1.ObjectReference, logger logr.Logger) (string, error) {

	fluxSource, err := getSource(ctx, c, ref)
	if err != nil {
		return "", err
	}

	if fluxSource == nil {
		return "", fmt.Errorf("source %s %s/%s not found",
			ref.Kind, ref.Namespace, ref.Name)
	}

	if fluxSource.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		logger.V(logs.LogInfo).Info(msg)
		return "", err
	}

	// Create tmp dir.
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("kustomization-%s-%s",
		ref.Namespace, ref.Name))
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
		return "", err
	}

	artifactFetcher := fetch.NewArchiveFetcher(
		1,
		tar.UnlimitedUntarSize,
		tar.UnlimitedUntarSize,
		os.Getenv("SOURCE_CONTROLLER_LOCALHOST"),
	)

	// Download artifact and extract files to the tmp dir.
	err = artifactFetcher.Fetch(fluxSource.GetArtifact().URL, fluxSource.GetArtifact().Digest, tmpDir)
	if err != nil {
		return "", err
	}

	return tmpDir, nil
}

func getSource(ctx context.Context, c client.Client, ref *corev1.ObjectReference) (sourcev1.Source, error) {
	var src sourcev1.Source
	namespacedName := types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}

	switch ref.Kind {
	case sourcev1.GitRepositoryKind:
		var repository sourcev1.GitRepository
		err := c.Get(ctx, namespacedName, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		src = &repository
	case sourcev1b2.OCIRepositoryKind:
		var repository sourcev1b2.OCIRepository
		err := c.Get(ctx, namespacedName, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return src, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		src = &repository
	case sourcev1b2.BucketKind:
		var bucket sourcev1b2.Bucket
		err := c.Get(ctx, namespacedName, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return src, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		src = &bucket
	default:
		return src, fmt.Errorf("source `%s` kind '%s' not supported",
			ref.Name, ref.Kind)
	}
	return src, nil
}

// templatesAsInput conveniently wraps one or more strings, each in a files.File, into a template.Input.
func templatesAsInput(dirPath string, yttSource *extensionv1alpha1.YttSource,
	logger logr.Logger) (yttcmd.Input, error) {

	// Get all files in the directory
	currentFiles, err := getFilesRecursively(dirPath)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to list files in directory %s: %v", dirPath, err))
		return yttcmd.Input{}, err
	}

	var files []*yttfiles.File
	for i := range currentFiles {
		content, err := readFileContent(currentFiles[i])
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("Failed to read file %s: %v", currentFiles[i], err))
			return yttcmd.Input{}, err
		}
		file, err := yttfiles.NewFileFromSource(yttfiles.NewBytesSource(fmt.Sprintf("tpl%d-%s-%s/%s", i, yttSource.Namespace, yttSource.Name, currentFiles[i]),
			[]byte(content)))
		if err != nil {
			return yttcmd.Input{}, err
		}
		files = append(files, file)
	}

	return yttcmd.Input{Files: files}, nil
}

// getFilesRecursively returns a list of all files in a directory and its subdirectories.
func getFilesRecursively(dir string) ([]string, error) {
	var fileList []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileList = append(fileList, path)
		}
		return nil
	})

	return fileList, err
}

// readFileContent reads and returns the content of a file.
func readFileContent(file string) (string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type noopWriter struct{}

func (w noopWriter) Write(data []byte) (int, error) { return len(data), nil }

// Close closes the current scope persisting the YttSource status.
func (s *YttSourceReconciler) Close(ctx context.Context, yttSource *extensionv1alpha1.YttSource,
	patchHelper *patch.Helper) error {

	return patchHelper.Patch(
		ctx,
		yttSource,
	)
}
