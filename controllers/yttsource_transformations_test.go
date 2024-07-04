/*
Copyright 2023. projectsveltos.io. All rights reserved.

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

package controllers_test

import (
	"context"
	"sync"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	extensionv1beta1 "github.com/gianlucam76/ytt-controller/api/v1beta1"
	"github.com/gianlucam76/ytt-controller/controllers"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	libsveltosset "github.com/projectsveltos/libsveltos/lib/set"
)

var _ = Describe("YttSourceTransformation map functions", func() {
	It("RequeueYttSourceForReference returns YttSource referencing a given ConfigMap", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}

		controllers.AddTypeInformationToObject(scheme, configMap)

		yttSource0 := &extensionv1beta1.YttSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: extensionv1beta1.YttSourceSpec{
				Namespace: configMap.Namespace,
				Name:      configMap.Name,
				Kind:      string(libsveltosv1beta1.ConfigMapReferencedResourceKind),
			},
		}

		yttSource1 := &extensionv1beta1.YttSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: extensionv1beta1.YttSourceSpec{
				Namespace: randomString(),
				Name:      configMap.Name,
				Kind:      string(libsveltosv1beta1.ConfigMapReferencedResourceKind),
			},
		}

		initObjects := []client.Object{
			configMap,
			yttSource0,
			yttSource1,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		reconciler := &controllers.YttSourceReconciler{
			Client:       c,
			Scheme:       scheme,
			ReferenceMap: make(map[corev1.ObjectReference]*libsveltosset.Set),
			YttSourceMap: make(map[types.NamespacedName]*libsveltosset.Set),
			PolicyMux:    sync.Mutex{},
		}

		set := libsveltosset.Set{}
		key := corev1.ObjectReference{APIVersion: configMap.APIVersion,
			Kind: string(libsveltosv1beta1.ConfigMapReferencedResourceKind), Namespace: configMap.Namespace, Name: configMap.Name}

		set.Insert(&corev1.ObjectReference{APIVersion: extensionv1beta1.GroupVersion.String(),
			Kind: extensionv1beta1.YttSourceKind, Namespace: yttSource0.Namespace, Name: yttSource0.Name})
		reconciler.ReferenceMap[key] = &set

		requests := controllers.RequeueYttSourceForReference(reconciler, context.TODO(), configMap)
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Name).To(Equal(yttSource0.Name))
		Expect(requests[0].Namespace).To(Equal(yttSource0.Namespace))

		set.Insert(&corev1.ObjectReference{APIVersion: extensionv1beta1.GroupVersion.String(),
			Kind: extensionv1beta1.YttSourceKind, Namespace: yttSource1.Namespace, Name: yttSource1.Name})
		reconciler.ReferenceMap[key] = &set

		requests = controllers.RequeueYttSourceForReference(reconciler, context.TODO(), configMap)
		Expect(requests).To(HaveLen(2))
		Expect(requests).To(ContainElement(
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: yttSource0.Namespace, Name: yttSource0.Name}}))
		Expect(requests).To(ContainElement(
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: yttSource1.Namespace, Name: yttSource1.Name}}))
	})
})

var _ = Describe("YttSourceTransformation map functions", func() {
	It("RequeueYttSourceForFluxSources returns YttSource referencing a given GitRepository", func() {
		gitRepo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}

		controllers.AddTypeInformationToObject(scheme, gitRepo)

		yttSource0 := &extensionv1beta1.YttSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: extensionv1beta1.YttSourceSpec{
				Namespace: gitRepo.Namespace,
				Name:      gitRepo.Name,
				Kind:      sourcev1.GitRepositoryKind,
			},
		}

		yttSource1 := &extensionv1beta1.YttSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: extensionv1beta1.YttSourceSpec{
				Namespace: gitRepo.Namespace,
				Name:      randomString(),
				Kind:      sourcev1.GitRepositoryKind,
			},
		}

		initObjects := []client.Object{
			gitRepo,
			yttSource0,
			yttSource1,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		reconciler := &controllers.YttSourceReconciler{
			Client:       c,
			Scheme:       scheme,
			ReferenceMap: make(map[corev1.ObjectReference]*libsveltosset.Set),
			YttSourceMap: make(map[types.NamespacedName]*libsveltosset.Set),
			PolicyMux:    sync.Mutex{},
		}

		set := libsveltosset.Set{}
		key := corev1.ObjectReference{APIVersion: gitRepo.APIVersion,
			Kind: sourcev1.GitRepositoryKind, Namespace: gitRepo.Namespace, Name: gitRepo.Name}

		set.Insert(&corev1.ObjectReference{APIVersion: extensionv1beta1.GroupVersion.String(),
			Kind: extensionv1beta1.YttSourceKind, Namespace: yttSource0.Namespace, Name: yttSource0.Name})
		reconciler.ReferenceMap[key] = &set

		requests := controllers.RequeueYttSourceForReference(reconciler, context.TODO(), gitRepo)
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Name).To(Equal(yttSource0.Name))
		Expect(requests[0].Namespace).To(Equal(yttSource0.Namespace))

		set.Insert(&corev1.ObjectReference{APIVersion: extensionv1beta1.GroupVersion.String(),
			Kind: extensionv1beta1.YttSourceKind, Namespace: yttSource1.Namespace, Name: yttSource1.Name})
		reconciler.ReferenceMap[key] = &set

		requests = controllers.RequeueYttSourceForReference(reconciler, context.TODO(), gitRepo)
		Expect(requests).To(HaveLen(2))
		Expect(requests).To(ContainElement(
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: yttSource0.Namespace, Name: yttSource0.Name}}))
		Expect(requests).To(ContainElement(
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: yttSource1.Namespace, Name: yttSource1.Name}}))
	})
})
