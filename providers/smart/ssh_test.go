package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"net/url"
)

var _ = Describe("SSH", func() {

	Context("Low level URL tests", func() {

		factory := &SshTransportFactory{}
		It("Correctly parses bare URLs", func() {
			// Make sure we can handle all forms of SSH URL
			// Bare url with no port and relative path
			u, err := url.Parse("git@host.com:path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com"), "Host should be parsed correctly")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("/path/to/repo"), "Path should be parsed correctly; path will have '/' added but that doesn't make it rooted")

			host, port := factory.getHostAndPort(u)
			Expect(host).To(Equal("host.com"), "Host should be extracted correctly")
			Expect(port).To(Equal(""), "Port should be extracted correctly")

			// Clean bare url with no FQDN & simple repo
			u, err = url.Parse("git@host:repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host"), "Host should be parsed correctly")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("/repo"), "Path should be parsed correctly; path will have '/' added but that doesn't make it rooted")

			// Bare url with no port and rooted path
			u, err = url.Parse("git@host.com:/rooted/path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com"), "Host should be parsed correctly")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("//rooted/path/to/repo"), "Path should be parsed correctly; double // makes it rooted")

			// Bare url with custom port
			u, err = url.Parse("git@host.com:1002:path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com:1002"), "Host should be parsed correctly and include port")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("/path/to/repo"), "Path should be parsed correctly; path will have '/' added but that doesn't make it rooted")

			host, port = factory.getHostAndPort(u)
			Expect(host).To(Equal("host.com"), "Host should be extracted correctly")
			Expect(port).To(Equal("1002"), "Host should be extracted correctly")
		})
		It("Correctly parses standard URLs", func() {
			// Make sure we can handle all forms of SSH URL
			// Standard url with no port and relative path
			u, err := url.Parse("ssh://git@host.com/path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com"), "Host should be parsed correctly")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("/path/to/repo"), "Path should be parsed correctly; path will have '/' added but that doesn't make it rooted")

			host, port := factory.getHostAndPort(u)
			Expect(host).To(Equal("host.com"), "Host should be extracted correctly")
			Expect(port).To(Equal(""), "Port should be extracted correctly")

			// Standard url with no port and rooted path
			u, err = url.Parse("ssh://git@host.com//rooted/path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com"), "Host should be parsed correctly")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("//rooted/path/to/repo"), "Path should be parsed correctly; double // makes it rooted")

			// Standard url with custom port
			u, err = url.Parse("ssh://git@host.com:1002/path/to/repo")
			Expect(err).To(BeNil(), "Should not be a problem parsing initial URL")
			Expect(factory.WillHandleUrl(u)).To(BeTrue(), "Should handle URL")
			// At this point the entire URL will just be in Path
			// So clean it up
			u = factory.cleanupBareUrl(u)
			Expect(u.Scheme).To(Equal("ssh"), "Scheme should be parsed correctly")
			Expect(u.Host).To(Equal("host.com:1002"), "Host should be parsed correctly and include port")
			Expect(u.User.Username()).To(Equal("git"), "User should be parsed correctly")
			Expect(u.Path).To(Equal("/path/to/repo"), "Path should be parsed correctly; path will have '/' added but that doesn't make it rooted")

			host, port = factory.getHostAndPort(u)
			Expect(host).To(Equal("host.com"), "Host should be extracted correctly")
			Expect(port).To(Equal("1002"), "Port should be extracted correctly")
		})

	})

})
