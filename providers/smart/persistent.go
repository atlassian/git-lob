package smart

import (
	"bufio"
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
	// Buffered reader we use to scan for ends of JSON
	BufferedReader *bufio.Reader
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
type JsonResponse struct {
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
		Connection:     conn,
		BufferedReader: bufio.NewReader(conn),
	}
}

// Release any resources associated with this transport (including any persostent connections)
func (self *PersistentTransport) Release() {
	self.BufferedReader = nil
	if self.Connection != nil {
		self.Connection.Close()
		self.Connection = nil
	}
}

// Perform a full JSON-RPC call with JSON request and response
func (self *PersistentTransport) doFullJSONRequestResponse(req interface{}, response interface{}) error {

	err := self.sendJSONRequest(req)
	if err != nil {
		return err
	}
	// read response (buffered) up to binary 0 which terminates JSON
	jsonbytes, err := self.BufferedReader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Unable to read response from server: %v", err.Error())
	}
	err = json.Unmarshal(jsonbytes, response)
	if err != nil {
		return fmt.Errorf("Unable to decode JSON response from server: %v\n%v", string(jsonbytes), err.Error())
	}
	// response is populated now
	return nil

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
	resp := make([]byte, responseLength)
	n, err := self.BufferedReader.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("Unable to read raw response: %v", err.Error())
	} else if n != responseLength {
		return nil, fmt.Errorf("Raw response was unexpeced length, actual: %d expected: %d", n, responseLength)
	}
	return resp, nil
}

// Send a JSON request but don't read any response
func (self *PersistentTransport) sendJSONRequest(req interface{}) error {
	if self.Connection == nil || self.BufferedReader == nil {
		return errors.New("Not connected")
	}

	reqbytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("Error encoding %v to JSON: %v", err.Error())
	}
	// Append the binary 0 delimiter that server uses to read up to
	reqbytes = append(reqbytes, byte(0))
	_, err = self.Connection.Write(reqbytes)
	if err != nil {
		return fmt.Errorf("Error writing request bytes to connection: %v", err.Error())
	}

	return nil
}

func (self *PersistentTransport) checkJSONResponse(req *JsonRequest, resp *JsonResponse) error {
	if resp.Error != nil {
		return fmt.Errorf("Error response from server: %v", resp.Error)
	}
	if req.Id != resp.Id {
		return fmt.Errorf("Response from server has wrong Id, request: %d response: %d", req.Id, resp.Id)
	}
	return nil
}

// Just a specially identified persistent connection error so we can re-try
type ConnectionError error

type QueryCapsRequest struct {
	JsonRequest
}

type QueryCapsResponse struct {
	JsonResponse
	Caps []string
}

// Ask the server for a list of capabilities
func (self *PersistentTransport) QueryCaps() ([]string, error) {
	req := &QueryCapsRequest{}
	InitJsonRequest(&req.JsonRequest)
	req.Method = "QueryCaps"
	resp := QueryCapsResponse{}
	err := self.doFullJSONRequestResponse(req, &resp)
	if err != nil {
		return nil, err
	}
	err = self.checkJSONResponse(&req.JsonRequest, &resp.JsonResponse)
	if err != nil {
		return nil, err
	}
	return resp.Caps, nil
}

type SetEnabledCapsRequest struct {
	JsonRequest
	EnableCaps []string
}
type SetEnabledCapsResponse struct {
	JsonResponse
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
