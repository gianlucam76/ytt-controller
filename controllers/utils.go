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
	archivetar "archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extensionv1alpha1 "github.com/gianlucam76/ytt-controller/api/v1alpha1"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
)

//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

const (
	permission0600 = 0600
	permission0755 = 0755
	maxSize        = int64(20 * 1024 * 1024)
)

func InitScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := sourcev1.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := sourcev1b2.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := extensionv1alpha1.AddToScheme(s); err != nil {
		return nil, err
	}
	return s, nil
}

// getKeyFromObject returns the Key that can be used in the internal reconciler maps.
func getKeyFromObject(scheme *runtime.Scheme, obj client.Object) *corev1.ObjectReference {
	addTypeInformationToObject(scheme, obj)

	apiVersion, kind := obj.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()

	return &corev1.ObjectReference{
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		Kind:       kind,
		APIVersion: apiVersion,
	}
}

func addTypeInformationToObject(scheme *runtime.Scheme, obj client.Object) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		panic(1)
	}

	for _, gvk := range gvks {
		if gvk.Kind == "" {
			continue
		}
		if gvk.Version == "" || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		break
	}
}

func getReferenceAPIVersion(yttSource *extensionv1alpha1.YttSource) string {
	switch yttSource.Spec.Kind {
	case string(libsveltosv1alpha1.ConfigMapReferencedResourceKind):
		return corev1.SchemeGroupVersion.String()
	case string(libsveltosv1alpha1.SecretReferencedResourceKind):
		return corev1.SchemeGroupVersion.String()
	case sourcev1b2.OCIRepositoryKind:
	case sourcev1b2.BucketKind:
		return sourcev1b2.GroupVersion.String()
	case sourcev1.GitRepositoryKind:
		return sourcev1.GroupVersion.String()
	}

	return ""
}

// getConfigMap retrieves any ConfigMap from the given name and namespace.
func getConfigMap(ctx context.Context, c client.Client, configmapName types.NamespacedName) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Namespace: configmapName.Namespace,
		Name:      configmapName.Name,
	}
	if err := c.Get(ctx, configMapKey, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

// getSecret retrieves any Secret from the given secret name and namespace.
func getSecret(ctx context.Context, c client.Client, secretName types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: secretName.Namespace,
		Name:      secretName.Name,
	}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		return nil, err
	}

	if secret.Type != libsveltosv1alpha1.ClusterProfileSecretType {
		return nil, libsveltosv1alpha1.ErrSecretTypeNotSupported
	}

	return secret, nil
}

func extractTarGz(src, dest string) error {
	// Open the tarball for reading
	tarball, err := os.Open(src)
	if err != nil {
		return err
	}
	defer tarball.Close()

	// Create a gzip reader to decompress the tarball
	gzipReader, err := gzip.NewReader(io.LimitReader(tarball, maxSize))
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	// Create a tar reader to read the uncompressed tarball
	tarReader := archivetar.NewReader(gzipReader)

	// Iterate over each file in the tarball and extract it to the destination
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Clean(header.Name))
		if !strings.HasPrefix(target, dest) {
			return fmt.Errorf("tar archive entry %q is outside of destination directory", header.Name)
		}

		switch header.Typeflag {
		case archivetar.TypeDir:
			if err := os.MkdirAll(target, permission0755); err != nil {
				return err
			}
		case archivetar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, io.LimitReader(tarReader, maxSize)); err != nil {
				return err
			}
			file.Close()
		}
	}

	return nil
}
