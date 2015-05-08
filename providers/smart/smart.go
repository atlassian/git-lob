package smart

import (
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The smart sync provider implements everything the standard SyncProvider does, but in addition
// provides methods to exchange binary deltas rather than entire files (as chunks).
// It can operate in 2 modes; 'persistent' mode where a connection is re-used for many requests
// (only possible with options like SSH), or 'transient' mode where all requests & responses are
// separate round-trips (e.g. REST). The Transport interface provides the abstraction required
// for that.
type SmartSyncProviderImpl struct {
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

func (*SmartSyncProviderImpl) TypeID() string {
	return "smart"
}

func (*SmartSyncProviderImpl) HelpTextSummary() string {
	return `smart: communicates with a git-lob compatible server to exchange binaries`
}

func (*SmartSyncProviderImpl) HelpTextDetail() string {
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

func (self *SmartSyncProviderImpl) ValidateConfig(remoteName string) error {
	return self.retrieveUrl(remoteName)
}

func (self *SmartSyncProviderImpl) retrieveUrl(remoteName string) error {
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
func (self *SmartSyncProviderImpl) connect(remoteName string) error {
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
		// use serverURL to establish transport
		tf := GetTransportFactory(self.serverUrl)
		if tf == nil {
			return fmt.Errorf("Unsupported URL: %v", self.serverUrl)
		}
		var err error
		self.transport, err = tf.Connect(self.serverUrl)
		if err != nil {
			return err
		}
		self.remoteName = remoteName

		err = self.determineCaps()
		if err != nil {
			return err
		}
	}
	return nil
}

// Negotiate with the server to determine capabilities
func (self *SmartSyncProviderImpl) determineCaps() error {
	var err error
	self.serverCaps, err = self.transport.QueryCaps()
	if err != nil {
		return err
	}
	// Always enable deltas if available
	self.enabledCaps = nil
	for _, c := range self.serverCaps {
		if c == "binary_delta" {
			self.enabledCaps = append(self.enabledCaps, c)
		}
		// nothing else for now
	}
	err = self.transport.SetEnabledCaps(self.enabledCaps)
	if err != nil {
		return err
	}

	return nil
}

// This is the file-based upload (i.e. a meta or a chunk) so no deltas here
// Client will use delta alts if it wants
func (self *SmartSyncProviderImpl) Upload(remoteName string, filenames []string, fromDir string,
	force bool, callback providers.SyncProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}

	var errorList []string
	for _, filename := range filenames {
		// Allow aborting
		newerrs, abort := self.uploadSingleFile(remoteName, filename, fromDir, force, callback)
		errorList = append(errorList, newerrs...)
		if abort {
			break
		}
	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}

// This is the file-based download (i.e. a meta or a chunk) so no deltas here
// Client will use delta alts if it wants
func (self *SmartSyncProviderImpl) Download(remoteName string, filenames []string, toDir string,
	force bool, callback providers.SyncProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}

	var errorList []string
	for _, filename := range filenames {
		// Allow aborting
		newerrs, abort := self.downloadSingleFile(remoteName, filename, toDir, force, callback)
		errorList = append(errorList, newerrs...)
		if abort {
			break
		}
	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}

func (self *SmartSyncProviderImpl) parseFilename(filename string) (sha string, ischunk bool, chunk int) {
	parts := strings.FieldsFunc(filename, func(r rune) bool {
		switch r {
		case '/', '_':
			return true
		}
		return false
	})
	if len(parts) < 2 {
		// Invalid
		return "", false, 0
	}
	// last part will be 'meta' or a number
	suffix := parts[len(parts)-1]
	// second to last will be sha
	thesha := parts[len(parts)-2]
	if suffix == "meta" {
		return thesha, false, 0
	} else {
		c, _ := strconv.ParseInt(suffix, 10, 32)
		return thesha, true, int(c)
	}
}

func (self *SmartSyncProviderImpl) FileExists(remoteName, filename string) bool {
	err := self.connect(remoteName)
	if err != nil {
		return false
	}

	sha, ischunk, chunk := self.parseFilename(filename)
	var exists bool
	if ischunk {
		exists, _, _ = self.transport.ChunkExists(sha, chunk)
	} else {
		exists, _, _ = self.transport.MetadataExists(sha)
		return exists
	}
	return exists
}
func (self *SmartSyncProviderImpl) FileExistsAndIsOfSize(remoteName, filename string, sz int64) bool {
	err := self.connect(remoteName)
	if err != nil {
		return false
	}
	sha, ischunk, chunk := self.parseFilename(filename)
	var exists bool
	if ischunk {
		exists, _ = self.transport.ChunkExistsAndIsOfSize(sha, chunk, sz)
	} else {
		// Never check size for meta
		exists, _, _ = self.transport.MetadataExists(sha)
	}
	return exists
}

func (self *SmartSyncProviderImpl) downloadSingleFile(remoteName, filename, toDir string,
	force bool, callback providers.SyncProgressCallback) (errorList []string, abort bool) {

	sha, ischunk, chunk := self.parseFilename(filename)
	var exists bool
	var sz int64
	if ischunk {
		exists, sz, _ = self.transport.ChunkExists(sha, chunk)
	} else {
		exists, sz, _ = self.transport.MetadataExists(sha)
	}
	if !exists {
		if callback != nil {
			if callback(filename, util.ProgressNotFound, 0, 0) {
				return errorList, true
			}
		}
		// Note how we don't add an error to the returned error list
		// As per provider docs, we simply tell callback it happened & treat it
		// as a skipped item otherwise, since caller can only request files & not know
		// if they're on the remote or not
		// Keep going with other files
		return errorList, false
	}

	destfilename := filepath.Join(toDir, filename)
	if !force {
		// Check existence & size before downloading
		if destfi, err := os.Stat(destfilename); err == nil {
			// File exists locally, check the size
			if destfi.Size() == sz {
				// File already present and correct size, skip
				if callback != nil {
					if callback(filename, util.ProgressSkip, sz, sz) {
						return errorList, true
					}
				}
				return errorList, false
			}
		}
	}

	// Make sure dest dir exists
	parentDir := filepath.Dir(destfilename)
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		msg := fmt.Sprintf("Unable to create dir %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	// Create a temporary file to copy, avoid issues with interruptions
	// Note this isn't a valid thing to do in security conscious cases but this isn't one
	// by opening the file we will get a unique temp file name (albeit a predictable one)
	outf, err := ioutil.TempFile(parentDir, "tempdownload")
	if err != nil {
		msg := fmt.Sprintf("Unable to create temp file for download in %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	tmpfilename := outf.Name()
	// This is safe to do even though we manually close & rename because both calls are no-ops if we succeed
	defer func() {
		outf.Close()
		os.Remove(tmpfilename)
	}()
	var abortAfterThisFile bool
	localcallback := func(bytesDone, totalBytes int64) {
		if callback != nil {
			if callback(filename, util.ProgressTransferBytes, bytesDone, totalBytes) {
				// Can't abort in the middle of a transfer with smart protocol
				abortAfterThisFile = true
			}
		}
	}
	// Initial callback
	if callback != nil {
		if callback(filename, util.ProgressTransferBytes, 0, sz) {
			return errorList, true
		}
	}
	if ischunk {
		err = self.transport.DownloadChunk(sha, chunk, outf, localcallback)
	} else {
		err = self.transport.DownloadMetadata(sha, outf)
	}
	outf.Close()
	if err != nil {
		os.Remove(tmpfilename)
		msg := fmt.Sprintf("Problem while downloading %v from %v: %v", filename, remoteName, err)
		errorList = append(errorList, msg)
		return errorList, abortAfterThisFile
	}
	// Move to correct location - remove before to deal with force or bad size cases
	os.Remove(destfilename)
	os.Rename(tmpfilename, destfilename)
	return errorList, abortAfterThisFile
}

func (self *SmartSyncProviderImpl) uploadSingleFile(remoteName, filename, fromDir string,
	force bool, callback providers.SyncProgressCallback) (errorList []string, abort bool) {

	// Check to see if the file is already there, right size
	srcfilename := filepath.Join(fromDir, filename)
	srcfi, err := os.Stat(srcfilename)
	if err != nil {
		if callback != nil {
			if callback(filename, util.ProgressNotFound, 0, 0) {
				return errorList, true
			}
		}
		msg := fmt.Sprintf("Unable to stat %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		// Keep going with other files
		return errorList, false
	}

	if !force {
		// Check existence & size before uploading
		if self.FileExistsAndIsOfSize(remoteName, filename, srcfi.Size()) {
			// File already present and correct size, skip
			if callback != nil {
				if callback(filename, util.ProgressSkip, srcfi.Size(), srcfi.Size()) {
					return errorList, true
				}
			}
			return errorList, false
		}
	}

	sha, ischunk, chunk := self.parseFilename(filename)

	// Initial callback
	if callback != nil {
		if callback(filename, util.ProgressTransferBytes, 0, srcfi.Size()) {
			return errorList, true
		}
	}
	var abortAfterThisFile bool
	localcallback := func(bytesDone, totalBytes int64) {
		if callback != nil {
			if callback(filename, util.ProgressTransferBytes, bytesDone, totalBytes) {
				// Can't abort in the middle of a transfer with smart protocol
				abortAfterThisFile = true
			}
		}
	}
	inf, err := os.OpenFile(srcfilename, os.O_RDONLY, 0644)
	if err != nil {
		msg := fmt.Sprintf("Unable to read input file for upload %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		return errorList, abortAfterThisFile
	}
	defer inf.Close()
	if ischunk {
		err = self.transport.UploadChunk(sha, chunk, srcfi.Size(), inf, localcallback)
	} else {
		err = self.transport.UploadMetadata(sha, srcfi.Size(), inf)
	}
	if err != nil {
		msg := fmt.Sprintf("Problem while uploading %v to %v: %v", srcfilename, remoteName, err)
		errorList = append(errorList, msg)
	}
	if callback != nil {
		if callback(filename, util.ProgressTransferBytes, srcfi.Size(), srcfi.Size()) {
			return errorList, true
		}
	}

	return errorList, abortAfterThisFile

}

// Whether a LOB exists in full on the remote, and gets its size
func (self *SmartSyncProviderImpl) LOBExists(remoteName, sha string) (ex bool, sz int64) {
	err := self.connect(remoteName)
	if err != nil {
		return false, 0
	}

	exists, sz, _ := self.transport.LOBExists(sha)
	return exists, sz
}

func (self *SmartSyncProviderImpl) PrepareDeltaForDownload(remoteName, sha string, candidateBaseSHAs []string) (size int64, base string, e error) {
	err := self.connect(remoteName)
	if err != nil {
		return 0, "", err
	}
	baseSHA, err := self.transport.GetFirstCompleteLOBFromList(candidateBaseSHAs)
	if err != nil {
		return 0, "", err
	}
	if baseSHA == "" {
		// no common base
		return 0, "", nil
	}
	sz, err := self.transport.DownloadDeltaPrepare(baseSHA, sha)
	if err != nil {
		return 0, baseSHA, err
	}
	return sz, baseSHA, nil
}

// Download delta of LOB content (must be applied later)
func (self *SmartSyncProviderImpl) DownloadDelta(remoteName, basesha, targetsha string, out io.Writer, callback providers.SyncProgressCallback) error {
	err := self.connect(remoteName)
	if err != nil {
		return err
	}
	description := fmt.Sprintf("Delta %v..%v", basesha[:7], targetsha[:7])
	localcallback := func(bytesDone, totalBytes int64) {
		callback(description, util.ProgressTransferBytes, bytesDone, totalBytes)
	}
	ok, err := self.transport.DownloadDelta(basesha, targetsha, 1024*1024*1024, out, localcallback)
	if !ok {
		return fmt.Errorf("Server chose not to provide a delta for %v", targetsha)
	}
	return err
}

func (self *SmartSyncProviderImpl) GetFirstCompleteLOBFromList(remoteName string, candidateSHAs []string) (string, error) {
	err := self.connect(remoteName)
	if err != nil {
		return "", err
	}
	return self.transport.GetFirstCompleteLOBFromList(candidateSHAs)
}

// Upload delta of LOB content (must be calculated first)
func (self *SmartSyncProviderImpl) UploadDelta(remoteName, basesha, targetsha string, in io.Reader, size int64, callback providers.SyncProgressCallback) error {
	err := self.connect(remoteName)
	if err != nil {
		return err
	}
	description := fmt.Sprintf("Delta %v..%v", basesha[:7], targetsha[:7])
	localcallback := func(bytesDone, totalBytes int64) {
		callback(description, util.ProgressTransferBytes, bytesDone, totalBytes)
	}
	ok, err := self.transport.UploadDelta(basesha, targetsha, size, in, localcallback)
	if !ok {
		return fmt.Errorf("Server chose not to accept a delta for %v", targetsha)
	}
	return err
}

// Init core smart providers
func InitCoreProviders() {
	// SSH transport
	RegisterSshTransportFactory()
	// Smart sync provider is a single instance which uses the transports to figure out concrete connection
	// from a URL. Only implementation right now is persistent/SSH but can have different modes (e.g. transient)
	// and different underlying network protocols (e.g. REST)
	providers.RegisterSyncProvider(&SmartSyncProviderImpl{})
}
