package providers

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SSH connection to smart server
// Works by invoking ssh/plink/tortoise_plink and connecting stdout/stdin
type SshConnection struct {
	// The command which is running ssh
	cmd *exec.Cmd
	// Streams for communicating
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	// The timeout period for operations (not automatic when communicating with Cmd)
	timeout time.Duration
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
func (self *SshConnectionFactory) Connect(u *url.URL) (Connection, error) {
	ssh := os.Getenv("GIT_SSH")
	isPlink := strings.EqualFold(filepath.Base(ssh), "plink")
	isTortoise := strings.EqualFold(filepath.Base(ssh), "tortoiseplink")
	if ssh == "" {
		ssh = "ssh"
	}
	// Clean up bare git@blah.com:port:path styles
	// we want to identify host & port, easiest to pull out of URL than parsing ourselves
	urlCleaned := self.cleanupBareUrl(u)
	// Cleaned URLs always have an ssh scheme
	if urlCleaned.Scheme != "ssh" {
		return nil, fmt.Errorf("%v is not a valid SSH URL", u.String())
	}
	host, port := self.getHostAndPort(urlCleaned)
	if host == "" {
		return nil, fmt.Errorf("No valid host found in url %v", u.String())
	}

	// Let's invoke ssh
	args := make([]string, 0, 2)
	if isTortoise {
		// TortoisePlink requires the -batch argument to behave like ssh/plink
		args = append(args, "-batch")
	}
	if port != "" {
		if isPlink {
			args = append(args, "-P")
		} else {
			args = append(args, "-p")
		}
		args = append(args, port)
	}
	args = append(args, host)
	cmd := exec.Command(ssh, args...)

	outp, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to ssh stdout: %v", err.Error())
	}
	errp, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to ssh stderr: %v", err.Error())
	}
	inp, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to ssh stdin: %v", err.Error())
	}
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("Unable to start ssh command: %v", err.Error())
	}

	return &SshConnection{
		cmd:     cmd,
		stdin:   inp,
		stdout:  outp,
		stderr:  errp,
		timeout: time.Second * 30,
	}, nil

}

func RegisterSshConnectionFactory() {
	RegisterConnectionFactory(&SshConnectionFactory{})
}

// SSH Connection implementation
func (self *SshConnection) Read(p []byte) (n int, err error) {
	// TODO
	return 0, nil
}
func (self *SshConnection) Write(p []byte) (n int, err error) {
	// TODO
	return 0, nil
}
func (self *SshConnection) Close() error {
	// Docs say "It is incorrect to call Wait before all writes to the pipe have completed."
	// But that actually means in parallel https://github.com/golang/go/issues/9307 so we're ok here
	err := self.cmd.Wait()
	if err != nil {
		errbytes, readerr := ioutil.ReadAll(self.stderr)
		if readerr != nil {
			return fmt.Errorf("Error closing ssh connection: %v", err.Error())
		} else {
			return fmt.Errorf("Error closing ssh connection: %v\nstderr: %v", err.Error(), string(errbytes))
		}
	}

	return nil

}
