package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bitbucket.org/sinbad/git-lob/providers/smart"
	"bytes"
	"net"
	"os"
	"path/filepath"
)

var _ = Describe("git-lob-serve tests", func() {
	Context("Test individual server requests", func() {

		var config *Config
		var repopath string

		BeforeEach(func() {
			config = NewConfig()
			config.BasePath = filepath.Join(os.TempDir(), "git-lob-serve-test")
			os.MkdirAll(config.BasePath, 0755)
			repopath = "test/repo"

		})
		AfterEach(func() {
			os.RemoveAll(config.BasePath)
		})

		It("Queries capabilities (client + reference server)", func() {
			cli, srv := net.Pipe()
			var outerr bytes.Buffer
			// 'Serve' is the real server function, usually connected to stdin/stdout but to pipe for test
			go Serve(srv, srv, &outerr, config, repopath)
			defer cli.Close()

			trans := smart.NewPersistentTransport(cli)
			caps, err := trans.QueryCaps()
			Expect(err).To(BeNil(), "Should be no error")
			Expect(caps).To(ConsistOf([]string{"binary_delta"}))
			Expect(outerr.String()).To(HaveLen(0), "Nothing should be written to stderr")

		})

	})

})
