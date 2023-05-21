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

package fv_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	extensionv1alpha1 "github.com/gianlucam76/ytt-controller/api/v1alpha1"
)

// ConfigMap ytt contains
/*
		apiVersion: v1
	    kind: Service
	    metadata:
	      name: sample-app
	      labels:
	        environment: staging
	    spec:
	      selector:
	        app: sample-app
	      ports:
	      - protocol: TCP
	        port: 80
	        targetPort: 8080
	    ---
	    apiVersion: apps/v1
	    kind: Deployment
	    metadata:
	      name: sample-app
	      labels:
	        environment: staging
	    spec:
	      replicas: 1
	      selector:
	        matchLabels:
	          environment: staging
	      template:
	        metadata:
	          labels:
	            environment: staging
	        spec:
	          containers:
	          - name: sample-app
	            image: nginx:latest
	            imagePullPolicy: IfNotPresent
	            ports:
	            - containerPort: 8080
*/

// ConfigMap ytt-overlays contains
/*
   apiVersion: extensions/v1beta1
   kind: Ingress
   metadata:
     name: example-ingress
     annotations:
       ingress.kubernetes.io/rewrite-target: /
       nginx.ingress.kubernetes.io/limit-rps: 2000
       nginx.ingress.kubernetes.io/enable-access-log: "true"
   ---
   apiVersion: extensions/v1beta1
   kind: Ingress
   metadata:
     name: example-ingress
     annotations:
       nginx.ingress.kubernetes.io/limit-rps: 2000
       nginx.ingress.kubernetes.io/enable-access-log: "true"
*/

var _ = Describe("Ytt", Serial, func() {
	const (
		namePrefix                   = "ytt-cm-"
		yttConfigMapName             = "ytt"
		yttConfigWithOverlaysMapName = "ytt-overlays"
	)

	It("Process a ConfigMap with YTT files", Label("FV"), func() {
		verifyYttSourceWithConfigMap(namePrefix, yttConfigMapName, 3)
		verifyYttSourceWithConfigMap(namePrefix, yttConfigWithOverlaysMapName, 2)
	})
})

func verifyYttSourceWithConfigMap(namePrefix, yttConfigMapName string, expectedResources int) {
	Byf("Verifying ConfigMap %s exists. It is created by Makefile", yttConfigMapName)
	yttConfigMap := &corev1.ConfigMap{}
	Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: defaultNamespace, Name: yttConfigMapName},
		yttConfigMap)).To(Succeed())

	Expect("Verifying ConfigMap %s contains ytt.tar.gz", yttConfigMapName)
	Expect(yttConfigMap.BinaryData).ToNot(BeNil())
	_, ok := yttConfigMap.BinaryData["ytt.tar.gz"]
	Expect(ok).To(BeTrue())

	Byf("Creating a YttSource referencing this ConfigMap")
	yttSource := &extensionv1alpha1.YttSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + randomString(),
			Namespace: randomString(),
		},
		Spec: extensionv1alpha1.YttSourceSpec{
			Namespace: yttConfigMap.Namespace,
			Name:      yttConfigMap.Name,
			Kind:      configMapKind,
			Path:      "./",
		},
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: yttSource.Namespace,
		},
	}
	Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())

	Expect(k8sClient.Create(context.TODO(), yttSource)).To(Succeed())

	Byf("Verifying YttSource %s/%s Status", yttSource.Namespace, yttSource.Name)
	Eventually(func() bool {
		currentYttSource := &extensionv1alpha1.YttSource{}
		err := k8sClient.Get(context.TODO(),
			types.NamespacedName{Namespace: yttSource.Namespace, Name: yttSource.Name},
			currentYttSource)
		if err != nil {
			return false
		}
		if currentYttSource.Status.FailureMessage != nil {
			return false
		}
		if currentYttSource.Status.Resources == "" {
			return false
		}
		return true
	}, timeout, pollingInterval).Should(BeTrue())

	Byf("Verifying YttSource %s/%s Status.Resources", yttSource.Namespace, yttSource.Name)

	currentYttSource := &extensionv1alpha1.YttSource{}
	Expect(k8sClient.Get(context.TODO(),
		types.NamespacedName{Namespace: yttSource.Namespace, Name: yttSource.Name},
		currentYttSource)).To(Succeed())

	resources := collectContent(currentYttSource.Status.Resources)
	Expect(len(resources)).To(Equal(expectedResources))

	Expect(k8sClient.Delete(context.TODO(), ns)).To(Succeed())
}
