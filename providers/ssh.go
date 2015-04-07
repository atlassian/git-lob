package providers

import (
	"fmt"
	"net/url"
	"os/exec"
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
