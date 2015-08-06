package format_test

import (
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"

	"testing"
)

func TestFormat(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Format Suite")
}
