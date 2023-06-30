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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TwinProduction/go-color"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	ginkgotypes "github.com/onsi/ginkgo/v2/types"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extensionv1alpha1 "github.com/gianlucam76/ytt-controller/api/v1alpha1"
)

var (
	k8sClient client.Client
	scheme    *runtime.Scheme
)

const (
	timeout         = 1 * time.Minute
	pollingInterval = 5 * time.Second
)

func TestFv(t *testing.T) {
	RegisterFailHandler(Fail)

	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.FullTrace = true
	reporterConfig.JSONReport = "out.json"
	report := func(report ginkgotypes.Report) {
		for i := range report.SpecReports {
			specReport := report.SpecReports[i]
			if specReport.State.String() == "skipped" {
				GinkgoWriter.Printf(color.Colorize(color.Blue, fmt.Sprintf("[Skipped]: %s\n", specReport.FullText())))
			}
		}
		for i := range report.SpecReports {
			specReport := report.SpecReports[i]
			if specReport.Failed() {
				GinkgoWriter.Printf(color.Colorize(color.Red, fmt.Sprintf("[Failed]: %s\n", specReport.FullText())))
			}
		}
	}
	ReportAfterSuite("report", report)

	RunSpecs(t, "FV Suite", suiteConfig, reporterConfig)
}

var _ = BeforeSuite(func() {
	ctrl.SetLogger(klog.Background())

	restConfig := ctrl.GetConfigOrDie()
	// To get rid of the annoying request.go log
	restConfig.QPS = 100
	restConfig.Burst = 100

	scheme = runtime.NewScheme()

	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	Expect(extensionv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(sourcev1.AddToScheme(scheme)).To(Succeed())

	var err error
	k8sClient, err = client.New(restConfig, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})
