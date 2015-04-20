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
	// RawMessage allows us to store late-resolved, message-specific nested types
	// requires an extra couple of steps though; even though RawMessage is a []byte, it's not
	// JSON itself. You need to convert JSON to/from RawMessage as well as JSON to/from the structure
	// - see RawMessage's own UnmarshalJSON/MarshalJSON for this extra step
	Params *json.RawMessage
}
type JsonResponse struct {
	Id    int
	Error interface{}
	// RawMessage allows us to store late-resolved, message-specific nested types
	// requires an extra couple of steps though; even though RawMessage is a []byte, it's not
	// JSON itself. You need to convert JSON to/from RawMessage as well as JSON to/from the structure
	// - see RawMessage's own UnmarshalJSON/MarshalJSON for this extra step
	Result *json.RawMessage
}

var (
	latestRequestId int = 1
)

func NewJsonRequest(method string, params interface{}) *JsonRequest {
	ret := &JsonRequest{
		Id:     latestRequestId,
		Method: method,
		Params: &json.RawMessage{},
	}
	// Encode nested struct ready for transmission so that it can be late unmarshalled at the other end
	// Need to do this & declare as RawMessage rather than interface{} in struct otherwise unmarshalling
	// at other end will turn it into a simple array/map
	// Doesn't affect the wire bytes; they're still nested JSON in the same way as if you marshalled the whole struct
	// this is just a golang method to defer resolving on unmarshal
	innerbytes, _ := json.Marshal(params)
	ret.Params.UnmarshalJSON(innerbytes)
	latestRequestId++
	return ret
}

func NewJsonResponse(id int, result interface{}) *JsonResponse {
	ret := &JsonResponse{
		Id:     id,
		Result: &json.RawMessage{},
	}
	// As NewJsonRequest, 2-level encoding for nested result to allow late resolution of custom types
	innerbytes, _ := json.Marshal(result)
	ret.Result.UnmarshalJSON(innerbytes)
	return ret
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
func (self *PersistentTransport) doFullJSONRequestResponse(method string, params interface{}, result interface{}) error {

	req := NewJsonRequest(method, params)
	err := self.sendJSONRequest(req)
	if err != nil {
		return err
	}
	// read response (buffered) up to binary 0 which terminates JSON
	jsonbytes, err := self.BufferedReader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Unable to read response from server: %v", err.Error())
	}
	// remove terminator before unmarshalling
	jsonbytes = jsonbytes[:len(jsonbytes)-1]
	response := JsonResponse{}
	err = json.Unmarshal(jsonbytes, &response)
	if err != nil {
		return fmt.Errorf("Unable to decode JSON response from server: %v\n%v", string(jsonbytes), err.Error())
	}
	// response is populated now
	err = self.checkJSONResponse(req, &response)
	if err != nil {
		return err
	}
	// response.Result is left as raw since it depends on the type of the expected result
	// so now unmarshal the nested part
	nestedbytes, err := response.Result.MarshalJSON()
	if err != nil {
		return fmt.Errorf("Unable to extract type-specific JSON from server: %v\n%v", string(*response.Result), err.Error())
	}
	err = json.Unmarshal(nestedbytes, &result)
	if err != nil {
		return fmt.Errorf("Unable to decode type-specific Result from server: %v\n%v", string(nestedbytes), err.Error())
	}
	// result is now populated
	return nil

}

// Perform a JSON request which just retrieves a raw fixed-length response, a precursor to reading a raw stream of bytes
// We don't use a JSON response becaue the length is undetermined and parsing it out of a stream which will contain raw
// bytes afterwards is unreliable
func (self *PersistentTransport) doJSONRequestRawResponse(method string, params interface{}, responseLength int) ([]byte, error) {

	req := NewJsonRequest(method, params)
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
}

type QueryCapsResponse struct {
	Caps []string
}

// Ask the server for a list of capabilities
func (self *PersistentTransport) QueryCaps() ([]string, error) {
	params := QueryCapsRequest{}
	resp := QueryCapsResponse{}
	err := self.doFullJSONRequestResponse("QueryCaps", &params, &resp)
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
