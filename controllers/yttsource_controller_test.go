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
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gianlucam76/ytt-controller/controllers"
)

var _ = Describe("YttSource Controller", func() {

	It("extractTarGz extracts tar.gz", func() {
		defer os.RemoveAll("testdata")
		createTarGz("testdata/test.tar.gz")

		// Create a temporary directory to use as the destination for the extracted files
		dest, err := os.MkdirTemp("", "test")
		Expect(err).To(BeNil())
		defer os.RemoveAll(dest)

		// Extract the test tarball to the destination
		err = controllers.ExtractTarGz("testdata/test.tar.gz", dest)
		Expect(err).To(BeNil())

		// Check that the extracted files match the expected contents
		expectedContents := map[string]string{
			"test.txt":         "This is a test file.",
			"testdir/test.txt": "This is another test file.",
		}
		for path, expectedContents := range expectedContents {
			filePath := filepath.Join(dest, path)
			var contents []byte
			contents, err = os.ReadFile(filePath)
			Expect(err).To(BeNil())
			Expect(string(contents)).To(Equal(expectedContents))
		}

		// Check that no additional files were extracted
		extraFilePath := filepath.Join(dest, "testdir", "extra.txt")
		_, err = os.Stat(extraFilePath)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func createTarGz(dest string) {
	// Create the test directory and some test files.
	err := os.MkdirAll("testdata/testdir", 0755)
	Expect(err).To(BeNil())
	err = os.WriteFile("testdata/test.txt", []byte("This is a test file."), 0600)
	Expect(err).To(BeNil())
	err = os.WriteFile("testdata/testdir/test.txt", []byte("This is another test file."), 0600)
	Expect(err).To(BeNil())

	// Create the testdata/test.tar.gz file.
	file, err := os.Create(dest)
	Expect(err).To(BeNil())
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	err = filepath.Walk("testdata/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = path[len("testdata")+1:]
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarWriter, file)
		return err
	})
	Expect(err).To(BeNil())
}
