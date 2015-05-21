package smart

import (
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// factory for creating SSH connections
type SshTransportFactory struct {
}

// Standardise bare URLs of the form user@host.com:path/to/repo
func (*SshTransportFactory) cleanupBareUrl(u *url.URL) *url.URL {
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
func (*SshTransportFactory) getHostAndPort(cleanedUrl *url.URL) (host, port string) {
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

func (self *SshTransportFactory) WillHandleUrl(u *url.URL) bool {
	if u.Scheme == "ssh" {
		return true
	}

	// try cleaning a bare URL user@host.com:something/something
	newu := self.cleanupBareUrl(u)
	return newu.Scheme == "ssh"
}
func (self *SshTransportFactory) Connect(u *url.URL) (Transport, error) {
	ssh := os.Getenv("GIT_SSH")
	basessh := filepath.Base(ssh)
	// Strip extension for easier comparison
	if ext := filepath.Ext(basessh); len(ext) > 0 {
		basessh = basessh[:len(basessh)-len(ext)]
	}
	isPlink := strings.EqualFold(basessh, "plink")
	isTortoise := strings.EqualFold(basessh, "tortoiseplink")
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

	util.LogDebugf("Connecting to %v over SSH...", host)

	// Let's invoke ssh
	args := make([]string, 0, 2)
	if isTortoise {
		// TortoisePlink requires the -batch argument to behave like ssh/plink
		args = append(args, "-batch")
	}
	if port != "" {
		if isPlink || isTortoise {
			args = append(args, "-P")
		} else {
			args = append(args, "-p")
		}
		args = append(args, port)
	}
	args = append(args, host)

	// Now add remote program and path
	args = append(args, util.GlobalOptions.SSHServerCommand)
	// u.Path includes a preceding '/', strip off manually
	// rooted paths in the URL will be '//path/to/blah'
	// this is just how Go's URL parsing works
	path := urlCleaned.Path
	if len(path) > 0 && strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	args = append(args, path)

	util.LogDebugf("SSH command is: %v %v", ssh, strings.Join(args, " "))

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

	conn := &SshConnection{
		cmd:    cmd,
		stdin:  inp,
		stdout: outp,
		stderr: errp,
	}

	util.LogDebugf("SSH connection successful to %v", host)

	return NewPersistentTransport(conn), nil

}

func RegisterSshTransportFactory() {
	RegisterTransportFactory(&SshTransportFactory{})
}

// Underlying SSH connection to smart server, for use with PersistentTransport
// Works by invoking ssh/plink/tortoise_plink and connecting stdout/stdin
type SshConnection struct {
	// The command which is running ssh
	cmd *exec.Cmd
	// Streams for communicating
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// SSH Connection implementation
func (self *SshConnection) Read(p []byte) (n int, err error) {
	return self.stdout.Read(p)
}
func (self *SshConnection) Write(p []byte) (n int, err error) {
	return self.stdin.Write(p)
}
func (self *SshConnection) Close() error {
	// Docs say "It is incorrect to call Wait before all writes to the pipe have completed."
	// But that actually means in parallel https://github.com/golang/go/issues/9307 so we're ok here
	errbytes, readerr := ioutil.ReadAll(self.stderr)
	if readerr == nil && len(errbytes) > 0 {
		// Copy to our stderr for info
		fmt.Fprintf(os.Stderr, "Messages from SSH server:\n%v", string(errbytes))
	}
	err := self.cmd.Wait()
	if err != nil {
		return fmt.Errorf("Error closing ssh connection: %v\nstderr: %v", err.Error(), string(errbytes))
	}

	return nil

}
