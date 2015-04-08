package providers

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// SSH connection to smart server
// Works by invoking ssh/plink/tortoise_plink and connecting stdout/stdin
type SshConnection struct {
	// The command which is running ssh
	cmd *exec.Cmd
	// The timeout period for operations (not automatic when communicating with Cmd)
	timeout time.Time
}

// factory for creating SSH connections
type SshConnectionFactory struct {
}

// Standardise bare URLs of the form user@host.com:path/to/repo
func (*SshConnectionFactory) cleanupBareUrl(u *url.URL) *url.URL {
	// Support ssh://user@host.com/path/to/repo and user@host.com:path/to/repo
	// The latter is entirely stored in the 'Path' field of url.URL though, so prefix
	if u.Scheme == "" && u.Path != "" {
		// Replace the path separator colon with /
		// Remember custom ports can include : too user@host.com:999:path/to/repo, must preserve
		parts := strings.Split(u.Path, ":")
		var newPath string
		if len(parts) > 2 { // port included; really should only ever be 3 parts
			newPath = fmt.Sprintf("%v:%v", parts[0], strings.Join(parts[1:], "/"))
		} else {
			newPath = strings.Join(parts, "/")
		}
		newUrlStr := fmt.Sprintf("ssh://%v", newPath)
		newu, err := url.Parse(newUrlStr)
		if err == nil {
			return newu
		}
	}
	return u
}

// Pull out the host & port for use on the command line from an already cleaned URL
func (*SshConnectionFactory) getHostAndPort(cleanedUrl *url.URL) (host, port string) {
	// Host includes host & port when custom ports are used
	// Note, not trying to validate host here, simple approach (this simple regex supports non-FQ and IP addresses as a bonus)
	// this would allow non-RFC compliant domain names but we don't care
	regex := regexp.MustCompile(`^([^\:]+)(?:\:(\d+))?$`)
	host = ""
	port = ""
	if match := regex.FindStringSubmatch(cleanedUrl.Host); match != nil {
		host = match[1]
		if len(match) > 2 {
			port = match[2]
		}
	}
	return
}

func (self *SshConnectionFactory) WillHandleUrl(u *url.URL) bool {
	if u.Scheme == "ssh" {
		return true
	}

	// try cleaning a bare URL user@host.com:something/something
	newu := self.cleanupBareUrl(u)
	return newu.Scheme == "ssh"
}
func (*SshConnectionFactory) Connect(u *url.URL) (Connection, error) {
	// todo
	return nil, nil
}

func RegisterSshConnectionFactory() {
	RegisterConnectionFactory(&SshConnectionFactory{})
}
