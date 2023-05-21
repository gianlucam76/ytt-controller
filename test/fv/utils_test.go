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
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/util"

	"github.com/projectsveltos/libsveltos/lib/utils"
)

const (
	defaultNamespace = "default"
	configMapKind    = "ConfigMap"
	separator        = "---\n"
)

// Byf is a simple wrapper around By.
func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...)) // ignore_by_check
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}

func collectContent(data string) []*unstructured.Unstructured {
	policies := make([]*unstructured.Unstructured, 0)

	elements := strings.Split(data, separator)
	for i := range elements {
		section := removeCommentsAndEmptyLines(elements[i])

		if section == "" {
			continue
		}

		policy, err := utils.GetUnstructured([]byte(section))
		Expect(err).To(BeNil())
		Expect(policy).ToNot(BeNil())

		policies = append(policies, policy)
	}

	return policies
}

func removeCommentsAndEmptyLines(text string) string {
	commentLine := regexp.MustCompile(`(?m)^\s*#([^#].*?)$`)
	result := commentLine.ReplaceAllString(text, "")
	emptyLine := regexp.MustCompile(`(?m)^\s*$`)
	result = emptyLine.ReplaceAllString(result, "")
	return result
}
