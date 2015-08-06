package main

import (
	"testing"

	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
)

func TestAll(t *testing.T) {
	// Connect Ginkgo to Gomega
	RegisterFailHandler(Fail)

	// Run everything
	RunSpecs(t, "Git Lob Serve Test Suite")
}
