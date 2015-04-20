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

func NewJsonRequest(method string, params interface{}) (*JsonRequest, error) {
	ret := &JsonRequest{
		Id:     latestRequestId,
		Method: method,
	}
	var err error
	ret.Params, err = embedStructInJsonRawMessage(params)
	latestRequestId++
	return ret, err
}

func NewJsonResponse(id int, result interface{}) (*JsonResponse, error) {
	ret := &JsonResponse{
		Id: id,
	}
	var err error
	ret.Result, err = embedStructInJsonRawMessage(result)
	return ret, err
}
func NewJsonErrorResponse(id int, err interface{}) *JsonResponse {
	ret := &JsonResponse{
		Id:    id,
		Error: err,
	}
	return ret
}

func embedStructInJsonRawMessage(in interface{}) (*json.RawMessage, error) {
	// Encode nested struct ready for transmission so that it can be late unmarshalled at the other end
	// Need to do this & declare as RawMessage rather than interface{} in struct otherwise unmarshalling
	// at other end will turn it into a simple array/map
	// Doesn't affect the wire bytes; they're still nested JSON in the same way as if you marshalled the whole struct
	// this is just a golang method to defer resolving on unmarshal
	ret := &json.RawMessage{}
	innerbytes, err := json.Marshal(in)
	if err != nil {
		return ret, fmt.Errorf("Unable to marshal struct to JSON: %v %v", in, err.Error())
	}
	err = ret.UnmarshalJSON(innerbytes)
	if err != nil {
		return ret, fmt.Errorf("Unable to convert JSON to RawMessage: %v %v", string(innerbytes), err.Error())
	}

	return ret, nil

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

	req, err := NewJsonRequest(method, params)
	if err != nil {
		return err
	}
	err = self.sendJSONRequest(req)
	if err != nil {
		return err
	}
	err = self.readFullJSONResponse(req, result)
	if err != nil {
		return err
	}
	// result is now populated
	return nil

}

func extractStructFromJsonRawMessage(raw *json.RawMessage, out interface{}) error {
	nestedbytes, err := raw.MarshalJSON()
	if err != nil {
		return fmt.Errorf("Unable to extract type-specific JSON from server: %v\n%v", string(*raw), err.Error())
	}
	err = json.Unmarshal(nestedbytes, &out)
	if err != nil {
		return fmt.Errorf("Unable to decode type-specific Result from server: %v\n%v", string(nestedbytes), err.Error())
	}
	return nil

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

func (self *PersistentTransport) readJSONResponse() (*JsonResponse, error) {
	jsonbytes, err := self.BufferedReader.ReadBytes(byte(0))
	if err != nil {
		return nil, fmt.Errorf("Unable to read response from server: %v", err.Error())
	}
	// remove terminator before unmarshalling
	jsonbytes = jsonbytes[:len(jsonbytes)-1]
	response := &JsonResponse{}
	err = json.Unmarshal(jsonbytes, response)
	if err != nil {
		return nil, fmt.Errorf("Unable to decode JSON response from server: %v\n%v", string(jsonbytes), err.Error())
	}
	return response, nil
}

// Check a response object; req can be nil, if so doesn't check that Ids match
func (self *PersistentTransport) checkJSONResponse(req *JsonRequest, resp *JsonResponse) error {
	if resp.Error != nil {
		return fmt.Errorf("Error response from server: %v", resp.Error)
	}
	if req != nil && req.Id != resp.Id {
		return fmt.Errorf("Response from server has wrong Id, request: %d response: %d", req.Id, resp.Id)
	}
	return nil
}

// Read a JSON response, check it, and pull out the nested method-specific & write to result
// originalReq is optional and can be left nil but if supplied Ids will be checked for matching
func (self *PersistentTransport) readFullJSONResponse(originalReq *JsonRequest, result interface{}) error {
	// read response (buffered) up to binary 0 which terminates JSON
	response, err := self.readJSONResponse()
	if err != nil {
		return err
	}
	// early validation
	err = self.checkJSONResponse(originalReq, response)
	if err != nil {
		return err
	}
	// response.Result is left as raw since it depends on the type of the expected result
	// so now unmarshal the nested part
	err = extractStructFromJsonRawMessage(response.Result, &result)
	if err != nil {
		return err
	}
	return nil
}

const PersistentTransportBufferSize = int64(131072)

func (self *PersistentTransport) sendRawData(sz int64, source io.Reader, callback TransportProgressCallback) error {

	if sz == 0 {
		return nil
	}

	var copysize int64 = 0
	for {
		c := PersistentTransportBufferSize
		if (sz - copysize) < c {
			c = sz - copysize
		}
		if c <= 0 {
			break
		}
		n, err := io.CopyN(self.Connection, source, c)
		copysize += n
		if n > 0 && callback != nil && sz > 0 {
			callback(copysize, sz)
		}
		if err != nil {
			return err
		}
	}
	if copysize != sz {
		return fmt.Errorf("Transferred bytes did not match expected size; transferred %d, expected %d", copysize, sz)
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
	EnableCaps []string
}
type SetEnabledCapsResponse struct {
}

// Request that the server enable capabilities for this exchange (note, non-persistent transports can store & send this with every request)
func (self *PersistentTransport) SetEnabledCaps(caps []string) error {
	params := SetEnabledCapsRequest{EnableCaps: caps}
	resp := SetEnabledCapsResponse{}
	err := self.doFullJSONRequestResponse("SetEnabledCaps", &params, &resp)
	if err != nil {
		return err
	}
	return nil
}

type FileExistsRequest struct {
	LobSHA   string
	Type     string
	ChunkIdx int
}
type FileExistsResponse struct {
	Result bool
}

// Return whether LOB metadata exists on the server
func (self *PersistentTransport) MetadataExists(lobsha string) (bool, error) {
	params := FileExistsRequest{
		LobSHA: lobsha,
		Type:   "meta",
	}
	resp := FileExistsResponse{}
	err := self.doFullJSONRequestResponse("FileExists", &params, &resp)
	if err != nil {
		return false, err
	}
	return resp.Result, nil
}

// Return whether LOB chunk content exists on the server
func (self *PersistentTransport) ChunkExists(lobsha string, chunk int) (bool, error) {
	params := FileExistsRequest{
		LobSHA:   lobsha,
		Type:     "chunk",
		ChunkIdx: chunk,
	}
	resp := FileExistsResponse{}
	err := self.doFullJSONRequestResponse("FileExists", &params, &resp)
	if err != nil {
		return false, err
	}
	return resp.Result, nil
}

type FileExistsOfSizeRequest struct {
	LobSHA   string
	Type     string
	ChunkIdx int
	Size     int64
}

// Return whether LOB chunk content exists on the server, and is of a specific size
func (self *PersistentTransport) ChunkExistsAndIsOfSize(lobsha string, chunk int, sz int64) (bool, error) {
	params := FileExistsOfSizeRequest{
		LobSHA:   lobsha,
		Type:     "chunk",
		ChunkIdx: chunk,
		Size:     sz,
	}
	resp := FileExistsResponse{}
	err := self.doFullJSONRequestResponse("FileExistsOfSize", &params, &resp)
	if err != nil {
		return false, err
	}
	return resp.Result, nil
}

type UploadFileRequest struct {
	LobSHA   string
	Type     string
	ChunkIdx int
	Size     int64
}
type UploadFileStartResponse struct {
	OKToSend bool
}
type UploadFileCompleteResponse struct {
	ReceivedOK bool
}

// Upload metadata for a LOB (from a stream); no progress callback as very small
func (self *PersistentTransport) UploadMetadata(lobsha string, sz int64, data io.Reader) error {
	params := UploadFileRequest{
		LobSHA: lobsha,
		Type:   "meta",
		Size:   sz,
	}
	resp := UploadFileStartResponse{}
	err := self.doFullJSONRequestResponse("UploadFile", &params, &resp)
	if err != nil {
		return fmt.Errorf("Error while uploading metadata for %v (while sending UploadFile JSON request): %v", lobsha, err.Error())
	}
	if resp.OKToSend {
		// Send that data (all at once, metafiles aren't big)
		err = self.sendRawData(sz, data, nil)
		if err != nil {
			return fmt.Errorf("Error while uploading metadata for %v (while sending raw content): %v", lobsha, err.Error())
		}
		// Now read response to sent data
		received := UploadFileCompleteResponse{}
		err = self.readFullJSONResponse(nil, &received)
		if err != nil {
			return fmt.Errorf("Error while uploading metadata for %v (response to raw content): %v", lobsha, err.Error())
		}

	} else {
		return fmt.Errorf("Server rejected request to upload metadata for %v (no other error)")
	}
	return nil
}

// Upload chunk content for a LOB (from a stream); must call back progress
func (self *PersistentTransport) UploadChunk(lobsha string, chunk int, sz int64, data io.Reader, callback TransportProgressCallback) error {
	// TODO
	return nil

}

// Download metadata for a LOB (to a stream); no progress callback as very small
func (self *PersistentTransport) DownloadMetadata(lobsha string, out io.Writer) error {
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
func (self *PersistentTransport) GetFirstCompleteLOBFromList(candidateSHAs []string) (string, error) {
	// TODO
	return "", nil

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
