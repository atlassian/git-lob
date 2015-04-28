package smart

import (
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"net/url"
)

// The smart sync provider implements everything the standard SyncProvider does, but in addition
// provides methods to exchange binary deltas rather than entire files (as chunks).
// It can operate in 2 modes; 'persistent' mode where a connection is re-used for many requests
// (only possible with options like SSH), or 'transient' mode where all requests & responses are
// separate round-trips (e.g. REST). The Transport interface provides the abstraction required
// for that.
type SmartSyncProvider struct {
	// The remote we're working with right now (for cached info)
	remoteName string
	// The parsed url we're using
	serverUrl *url.URL

	// The transport which is providing the underlying operations
	transport Transport
	// capabilities which the server has indicated it supports
	serverCaps []string
	// capabilities which are enabled
	enabledCaps []string
}

// See doc/smart_protocol.md for protocol definition

func (*SmartSyncProvider) TypeID() string {
	return "smart"
}

func (*SmartSyncProvider) HelpTextSummary() string {
	return `smart: communicates with a git-lob compatible server to exchange binaries`
}

func (*SmartSyncProvider) HelpTextDetail() string {
	return `The "smart" provider transfers files by talking to service hosted on
the remote binary store which can communicate using a git-lob protocol. Many
transports are supportable so long as client and server can establish comms. 
The reference implementation git-lob-server supports communicating over SSH.

The smart provider is capable of optimising uploads and downloads by exchanging
binary deltas with the server. Smart servers can also implement other features
like proxy caching.

Required parameters in remote section of .gitconfig:
    git-lob-url    URL which can be used to establish a connection
                   (SSH URLs only for now - more options in future)

Example configuration:
    [remote "origin"]
        url = git@blah.com/your/usual/git/repo
        git-lob-provider = smart
        git-lob-url = me@someserver.com/path/to/binary/store

When uploading & downloading, to avoid partially written files when interrupted
a temporary file is created first, then moved to the final location on 
completion. While we clean up files on error and exit, if forcibly interrupted
temporary files may remain; these are called 'tempupload*' and 'tempdownload*'
in the target file structure and can be safely deleted if older than 24h.
`
}

func (self *SmartSyncProvider) ValidateConfig(remoteName string) error {
	return self.retrieveUrl(remoteName)
}

func (self *SmartSyncProvider) retrieveUrl(remoteName string) error {
	urlsetting := fmt.Sprintf("remote.%v.git-lob-url", remoteName)
	urlstr := util.GlobalOptions.GitConfig[urlsetting]
	if urlstr == "" {
		return fmt.Errorf("Configuration invalid for 'smart', missing setting %v", urlsetting)
	}
	// Check URL is valid
	u, err := url.Parse(urlstr)
	if err != nil {
		return fmt.Errorf("Invalid git-lob-url setting '%v': %v", urlstr, err.Error())
	}
	self.serverUrl = u
	return nil
}

// Internal method to make sure we've established a connection
// we re-use connections where possible (TODO disconnection issues?)
func (self *SmartSyncProvider) connect(remoteName string) error {
	if remoteName != self.remoteName || self.transport == nil {
		if self.transport != nil {
			self.transport.Release()
			self.transport = nil
		}
		self.serverCaps = nil
		self.enabledCaps = nil
		if self.serverUrl == nil {
			err := self.retrieveUrl(remoteName)
			if err != nil {
				return err
			}
		}
		// TODO, use serverURL to establish transport

		self.remoteName = remoteName

		err := self.determineCaps()
		if err != nil {
			return err
		}
	}
	return nil
}

// Negotiate with the server to determine capabilities
func (self *SmartSyncProvider) determineCaps() error {
	// TODO
	return nil
}

func (self *SmartSyncProvider) Upload(remoteName string, filenames []string, fromDir string,
	force bool, callback TransportProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}

	// TODO
	return nil
}

// Redefine this so we don't have a circular package reference
type SyncProgressCallback func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool)

func (self *SmartSyncProvider) Download(remoteName string, filenames []string, toDir string,
	force bool, callback SyncProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}

	// MUST check existence before calling download, use callback to report if missing
	// TODO

	return nil
}

func (self *SmartSyncProvider) FileExists(remoteName, filename string) bool {
	err := self.connect(remoteName)
	if err != nil {
		return false
	}

	// TODO
	return false
}
func (self *SmartSyncProvider) FileExistsAndIsOfSize(remoteName, filename string, sz int64) bool {
	err := self.connect(remoteName)
	if err != nil {
		return false
	}
	// TODO
	return false
}

// TODO add additional methods for deltas

// Init core smart providers
func InitCoreProviders() {
	// SSH transport
	RegisterSshTransportFactory()
}
