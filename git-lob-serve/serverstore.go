package main

import (
	"bitbucket.org/sinbad/git-lob/providers/smart"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// A server could choose to store LOBs however it likes
// For simplicity, this server chooses to store the LOB files in the same structure as the client does,
// with the addition that it also stores cached binary deltas.

// In order to keep this reference implementation cleaner, we don't rely on any of the storage code in the
// client 'core' package - we could do but it would make it look more complex & intertwined with the client than it is,
// and most of the client features aren't useful. So we'll duplicate a little bit of the functionality from core
// here, deliberately, to illustrate how the server is completely independent in its storage model and to
// avoid pulling in client-centric logic that isn't appropriate.

// For convenience we pull in the JSON structures from the 'smart' package but in another language you
// can marshal/unmarshal to JSON however you like. We also use the 'util' package from client for convenience
// but that's just simple boilerplate stuff

// Get the absolute path to the directory containing LOB files for a given SHA
// Does not create the directory nor validate that config is correct
func getLOBDir(sha string, config *Config, path string) string {
	return filepath.Join(config.BasePath, path, sha[:3], sha[3:6])
}

// Get the absolute path of a LOB chunk file
// Does not create the directory nor validate that config is correct
func getLOBChunkFilePath(sha string, chunk int, config *Config, path string) string {
	return filepath.Join(getLOBDir(sha, config, path), fmt.Sprintf("%v_%d", sha, chunk))
}

// Get the absolute path of a LOB meta file
// Does not create the directory nor validate that config is correct
func getLOBMetaFilePath(sha string, config *Config, path string) string {
	return filepath.Join(getLOBDir(sha, config, path), sha+"_meta")
}

// Generic method to get file path based on type (meta/chunk)
// Does not create the directory nor validate that config is correct
func getLOBFilePath(sha, filetype string, chunk int, config *Config, path string) string {
	if filetype == "chunk" {
		return getLOBChunkFilePath(sha, chunk, config, path)
	} else if filetype == "meta" {
		return getLOBMetaFilePath(sha, config, path)
	}
	// error
	return ""
}

func fileExists(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	freq := smart.FileExistsRequest{}
	err := smart.ExtractStructFromJsonRawMessage(req.Params, &freq)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	result := smart.FileExistsResponse{}
	file := getLOBFilePath(freq.LobSHA, freq.Type, freq.ChunkIdx, config, path)
	if file == "" {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unsupported file type: %v", freq.Type))
	}

	result.Result = util.FileExists(file)

	resp, err := smart.NewJsonResponse(req.Id, result)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	return resp
}

func fileExistsOfSize(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	freq := smart.FileExistsOfSizeRequest{}
	err := smart.ExtractStructFromJsonRawMessage(req.Params, &freq)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	result := smart.FileExistsResponse{}
	file := getLOBFilePath(freq.LobSHA, freq.Type, freq.ChunkIdx, config, path)
	if file == "" {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unsupported file type: %v", freq.Type))
	}

	result.Result = util.FileExistsAndIsOfSize(file, freq.Size)

	resp, err := smart.NewJsonResponse(req.Id, result)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	return resp
}

func ensureDirExists(dir string, cfg *Config) error {
	if !util.DirExists(dir) {
		// Get permissions from base path & match (or default to user/group write)
		mode := os.FileMode(0775)
		s, err := os.Stat(cfg.BasePath)
		if err == nil {
			mode = s.Mode()
		}
		return os.MkdirAll(dir, mode)
	}
	return nil
}

const transferBufferSize = int64(128 * 1024)

func uploadFile(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	upreq := smart.UploadFileRequest{}
	err := smart.ExtractStructFromJsonRawMessage(req.Params, &upreq)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	startresult := smart.UploadFileStartResponse{}
	startresult.OKToSend = true
	// Send start response immediately
	resp, err := smart.NewJsonResponse(req.Id, startresult)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	err = sendResponse(resp, out)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	// Next from client should be byte stream of exactly the stated number of bytes
	// Write to temporary file then move to final on success
	file := getLOBFilePath(upreq.LobSHA, upreq.Type, upreq.ChunkIdx, config, path)
	if file == "" {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unsupported file type: %v", upreq.Type))
	}

	// Now open temp file to write to
	outf, err := ioutil.TempFile("", "tempchunk")
	defer outf.Close()
	n, err := io.CopyN(outf, in, upreq.Size)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unable to read data: %v", err.Error()))
	} else if n != upreq.Size {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Received wrong number of bytes %d (expected %d)", n, upreq.Size))
	}

	receivedresult := smart.UploadFileCompleteResponse{}
	receivedresult.ReceivedOK = true
	var receiveerr string
	// force close now before defer so we can copy
	err = outf.Close()
	if err != nil {
		receivedresult.ReceivedOK = false
		receiveerr = fmt.Sprintf("Error when closing temp file: %v", err.Error())
	} else {
		// ensure final directory exists
		ensureDirExists(filepath.Dir(file), config)
		// Move temp file to final location
		err = os.Rename(outf.Name(), file)
		if err != nil {
			receivedresult.ReceivedOK = false
			receiveerr = fmt.Sprintf("Error when closing temp file: %v", err.Error())
		}

	}

	resp, _ = smart.NewJsonResponse(req.Id, receivedresult)
	if receiveerr != "" {
		resp.Error = receiveerr
	}

	return resp

}

func downloadFilePrepare(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	downreq := smart.DownloadFilePrepareRequest{}
	err := smart.ExtractStructFromJsonRawMessage(req.Params, &downreq)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	file := getLOBFilePath(downreq.LobSHA, downreq.Type, downreq.ChunkIdx, config, path)
	if file == "" {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unsupported file type: %v", downreq.Type))
	}
	result := smart.DownloadFilePrepareResponse{}
	s, err := os.Stat(file)
	if err != nil {
		// file doesn't exist, this should not have been called
		return smart.NewJsonErrorResponse(req.Id, "File doesn't exist")
	}
	result.Size = s.Size()
	resp, err := smart.NewJsonResponse(req.Id, result)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	return resp

}

func downloadFileStart(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	downreq := smart.DownloadFileStartRequest{}
	err := smart.ExtractStructFromJsonRawMessage(req.Params, &downreq)
	if err != nil {
		// Serve() copes with converting this to stderr rather than JSON response
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	file := getLOBFilePath(downreq.LobSHA, downreq.Type, downreq.ChunkIdx, config, path)
	if file == "" {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unsupported file type: %v", downreq.Type))
	}
	// check size
	s, err := os.Stat(file)
	if err != nil {
		// file doesn't exist, this should not have been called
		return smart.NewJsonErrorResponse(req.Id, "File doesn't exist")
	}
	if s.Size() != downreq.Size {
		// This won't work!
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("File sizes disagree (client: %d server: %d)", downreq.Size, s.Size()))
	}

	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, err.Error())
	}
	defer f.Close()

	n, err := io.Copy(out, f)
	if err != nil {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Error copying data to output: %v", err.Error()))
	}
	if n != s.Size() {
		return smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Amount of data copied disagrees (expected: %d actual: %d)", s.Size(), n))
	}

	// Don't return a response, only response is byte stream above except in error cases
	return nil
}

func pickCompleteLOB(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	// TODO
	return nil
}

func uploadDelta(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	// TODO
	return nil
}

func downloadDeltaPrepare(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	// TODO
	return nil
}
func downloadDeltaStart(req *smart.JsonRequest, in io.Reader, out io.Writer, config *Config, path string) *smart.JsonResponse {
	// TODO
	return nil
}
