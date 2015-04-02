package providers

import (
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"io"
	"net/url"
)

// The smart sync provider implements everything the standard SyncProvider does, but in addition
// provides methods to exchange binary deltas rather than entire files (as chunks).
// It performs all of its work over I/O streams, and requires a corresponding server implementation
// at the other end (reference implementation in package 'git-lob-server').
// This implementation doesn't handle how that connection is established & authenticated, it just
// deals with the underlying communication once the connection is established.
type SmartSyncProvider struct {
	// The remote we're working with right now
	remoteName string
	// The parsed url we're using
	serverUrl *url.URL
	// Reader which we use to pull bytes from the server
	reader io.ReadCloser
	// Writer which we use to send bytes to the server
	writer io.WriteCloser
	// capabilities which the server has indicated it supports
	serverCaps []string
	// capabilities which are enabled
	enabledCaps []string
}

// Smart Protocol definition
// -------------------------
// Assuming that a connection has been established & authenticated, the exchange occurs
// as a series of operations & acknowledgements. Multiple operations can be performed over
// one connection. Each operation is identified with a JSON-RPC format request from the client,
// identifying the requested method, parameters, and an id. The server will respond with a
// similarly JSON-RPC standard reply. http://www.jsonrpc.org/specification

// However rather than encumber the transfer of binary data with a conversion to/from base64
// or even base85, raw binary content will be sent 'in between' JSON-RPC requests. This is not
// strictly standard but it works much better. So a server response for a proposed upload
// from the client would be a kind of 'go ahead' signal, after which the client should transfer
// the number of bytes advertised in the request in a raw stream. The server will then respond
// with another confirmation once all the bytes are received.

// The smart protocol still supports the same simple chunked file upload of the basic sync;
// this is used if binary deltas are not supported, or are impractical/worse for this file.
// However instead of uploading the file by the filename and mirroring the same file structure,
// the data is sent to the server with information about what type it is and what chunk number it is,
// and the server is free to store that however it likes, so long as it respond on that basis.

// Method:  query_caps
// Purpose: Asks the server to return its supported capabilities
// Params:  None
// Result:  Array of strings identifying capabilities the server supports. So far only
//          one is defined: "binary_delta"

// Method:  set_caps
// Purpose: Tells the server that the client wants to enable a list of capabilities. All omitted
//          caps are assumed to be disabled
// Params:  Array of strings identifying caps to enable, must have been present in query_caps response.
// Result:  "OK" on success (error should also be populated on error)

// Method:  file_exists
// Purpose: Find out whether a given file (metadata or chunk) exists on the server already
// Params:  lobSHA (string) - the SHA of the binary file in question
//          type (string) - "meta" or "chunk"
//          chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)
// Result:  True or False

// Method:  file_exists_of_size
// Purpose: Find out whether a given file (metadata or chunk) exists on the server already
//          and is of the size specified
// Params:  lobSHA (string) - the SHA of the binary file in question
//          type (string) - "meta" or "chunk"
//          chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)
//          size (Number) - size in bytes
// Result:  True or False

// Method:      upload_file
// Purpose:     Upload a single file (metadata or chunk). This does not deal with binary deltas,
//              only with the simple chunked upload of big files. However the server is free to
//              store these however it likes.
// Params:      lobSHA (string) - the SHA of the binary file in question
//              type (string) - "meta" or "chunk"
//              chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)
//              size (Number) - size in bytes
// Result:      OK if clear to send. Note server must accept upload if client requests it even
//              if it has the file already (--force). Client will use file_exists_of_size to
//              make it's own decision on whether to upload or not.
// POST:        Immediately after Result:OK, a BINARY STREAM of bytes will be sent by the client
//              to the server of length 'size' above.
// POST Result: OK if server received all the bytes and stored the file successfully

// Method:      download_file
// Purpose:     Download a single file (metadata or chunk). This does not deal with binary deltas,
//              only with the simple chunked upload of big files. However the server is free to
//              store these however it likes.
// Params:      lobSHA (string) - the SHA of the binary file in question
//              type (string) - "meta" or "chunk"
//              chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)
//              size (Number) - size in bytes
// Result:      OK if server has the data to send.
// POST:        Immediately after Result:OK, a BINARY STREAM of bytes will be sent by the server
//              to the client of length 'size' above. The client must read all the bytes.
// POST Result: No post result is required to be sent from client to server on receipt of all
//              the bytes (server doesn't care). After all bytes have been read the server is ready
//              for a new command.

// Method:  has_complete_lob
// Purpose: Out of a list of LOB SHAs in order of preference, return which one (if any) the server
//          has a complete copy of already. This is used to probe for previous versions of a file
//          for the client to send a binary delta of
// Params:  lobshas - array of strings identifying LOBs in order of preference (usually ancestors of a file)
// Result:  sha - first sha in the list that server has a complete file copy of. The server should confirm
//          that all data is present but does not need to check the sha integrity (done post delta application)

func (*SmartSyncProvider) TypeID() string {
	return "smart"
}

func (*SmartSyncProvider) HelpTextSummary() string {
	return `smart: communicates with a git-lob compatible server to exchange binaries`
}

func (*SmartSyncProvider) HelpTextDetail() string {
	return `The "smart" provider transfers files solely by talking to service hosted on
the remote binary store which can communicate using the git-lob protocol. Many
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
	if remoteName != self.remoteName || self.reader == nil || self.writer == nil {
		if self.serverUrl == nil {
			err := self.retrieveUrl(remoteName)
			if err != nil {
				return err
			}
		}
		// TODO, use serverURL to establish connection via connection factory

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
	force bool, callback SyncProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}

	// TODO
	return nil
}

func (self *SmartSyncProvider) Download(remoteName string, filenames []string, toDir string,
	force bool, callback SyncProgressCallback) error {

	err := self.connect(remoteName)
	if err != nil {
		return err
	}
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
