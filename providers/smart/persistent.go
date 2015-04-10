package smart

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Transport implementation that uses a persistent connection to perform many
// operations in serial, without having to initiate a connection each time
// Most common use of this is SSH, althouth the underlying connection streams are
// abstracted to allow other connections if required.
type PersistentTransport struct {
	// The persistent connection we're using (implemented by another class)
	Connection io.ReadWriteCloser
}

// Note *not* using net/rpc and net/rpc/jsonrpc because we want more control
// golang's rpc requires a certain method format (Object.Method) and also doesn't easily
// support interleaving with raw byte streams like we need to.
// as per http://www.jsonrpc.org/specification
type JsonRequest struct {
	// Go's JSON support only uses public fields but JSON-RPC requires lower case
	Id     int
	Method string
}
type JsonRpcResponse struct {
	Id    int
	Error interface{}
}

var (
	latestRequestId int = 1
)

func InitJsonRequest(req *JsonRequest) {
	req.Id = latestRequestId
	latestRequestId++
}

// Create a new persistent transport & connect
func NewPersistentTransport(conn io.ReadWriteCloser) *PersistentTransport {
	return &PersistentTransport{
		Connection: conn,
	}
}

// Release any resources associated with this transport (including any persostent connections)
func (self *PersistentTransport) Release() {
	if self.Connection != nil {
		self.Connection.Close()
		self.Connection = nil
	}
}

// Perform a full JSON-RPC call with JSON request and response
func (self *PersistentTransport) doFullJSONRequestResponse(req interface{}, response interface{}) (interface{}, error) {

	err := self.sendJSONRequest(req)
	if err != nil {
		return nil, err
	}
	// read response (buffered up to 200 bytes)
	// TODO
	return nil, nil

}

// Perform a JSON request which just retrieves a raw fixed-length response, a precursor to reading a raw stream of bytes
// We don't use a JSON response becaue the length is undetermined and parsing it out of a stream which will contain raw
// bytes afterwards is unreliable
func (self *PersistentTransport) doJSONRequestRawResponse(req interface{}, responseLength int) ([]byte, error) {

	err := self.sendJSONRequest(req)
	if err != nil {
		return nil, err
	}
	// read response (exactly responseLengthBytes)
	// TODO
	return nil, nil
}

// Send a JSON request but don't read any response
func (self *PersistentTransport) sendJSONRequest(req interface{}) error {
	if self.Connection == nil {
		return errors.New("Not connected")
	}

	reqbytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("Error encoding %v to JSON: %v", err.Error())
	}
	_, err = self.Connection.Write(reqbytes)
	if err != nil {
		return fmt.Errorf("Error writing request bytes to connection: %v", err.Error())
	}
	return nil
}

// Just a specially identified persistent connection error so we can re-try
type ConnectionError error

type QueryCapsRequest struct {
	JsonRequest
}

type QueryCapsResponse struct {
	JsonRpcResponse
	Caps []string
}

// Ask the server for a list of capabilities
func (self *PersistentTransport) QueryCaps() ([]string, error) {
	req := &QueryCapsRequest{}
	InitJsonRequest(&req.JsonRequest)
	req.Method = "QueryCaps"
	resp := QueryCapsResponse{}
	result, err := self.doFullJSONRequestResponse(req, &resp)

	_ = result
	_ = err
	//capsResult := QueryCapsResponse(result)

	// TODO
	return nil, nil
}

type SetEnabledCapsRequest struct {
	EnableCaps []string
}

// Request that the server enable capabilities for this exchange (note, non-persistent transports can store & send this with every request)
func (self *PersistentTransport) SetEnabledCaps(caps []string) error {
	// TODO
	return nil
}

// Return whether LOB metadata exists on the server
func (self *PersistentTransport) MetadataExists(lobsha string) bool {
	// TODO
	return false
}

// Return whether LOB chunk content exists on the server
func (self *PersistentTransport) ChunkExists(lobsha string, chunk int) bool {
	// TODO
	return false
}

// Return whether LOB chunk content exists on the server, and is of a specific size
func (self *PersistentTransport) ChunkExistsAndIsOfSize(lobsha string, chunk int, sz int64) bool {
	// TODO
	return false
}

// Upload metadata for a LOB (from a stream); must call back progress
func (self *PersistentTransport) UploadMetadata(lobsha string, data io.Reader, callback TransportProgressCallback) error {
	// TODO
	return nil
}

// Upload chunk content for a LOB (from a stream); must call back progress
func (self *PersistentTransport) UploadChunk(lobsha string, chunk int, sz int64, data io.Reader, callback TransportProgressCallback) error {
	// TODO
	return nil

}

// Download metadata for a LOB (to a stream); must call back progress
func (self *PersistentTransport) DownloadMetadata(lobsha string, out io.Writer, callback TransportProgressCallback) error {
	// TODO
	return nil

}

// Download chunk content for a LOB (from a stream); must call back progress
// This is a non-delta download operation, just provide entire chunk content
func (self *PersistentTransport) DownloadChunk(lobsha string, chunk int, out io.Writer, callback TransportProgressCallback) error {
	// TODO
	return nil

}

// Return the LOB which the server has a complete copy of, from a list of candidates
// Server must test in the order provided & return the earliest one which is complete on the server
// Server doesn't have to test full integrity of LOB, just completeness (check size against meta)
// Return a blank string if none are available
func (self *PersistentTransport) GetFirstCompleteLOBFromList(candidateSHAs []string) string {
	// TODO
	return ""

}

// Upload a binary delta to apply against a LOB the server already has, to generate a new LOB
// Deltas apply to whole LOB content and are not per-chunk
// Returns a boolean to determine whether the upload was accepted or not (server may prefer not to accept, not an error)
// In the case of false return, client will fall back to non-delta upload.
// On true, server must return nil error only after data is fully received, applied, saved as targetSHA and the
// integrity confirmed by recalculating the SHA of the final patched data.
func (self *PersistentTransport) UploadDelta(baseSHA, targetSHA string, deltaSize int64, data io.Reader, callback TransportProgressCallback) (bool, error) {
	// TODO
	return false, nil

}

// Generate and download a binary delta that the client can apply locally to generate a new LOB
// Deltas apply to whole LOB content and are not per-chunk
// The server should respect sizeLimit and if the delta is larger than that, abandon the process
// Return a bool to indicate whether the delta went ahead or not (client will fall back to non-delta on false)
func (self *PersistentTransport) DownloadDelta(baseSHA, targetSHA string, sizeLimit int64, out io.Writer, callback TransportProgressCallback) (bool, error) {
	// TODO
	return false, nil

}
