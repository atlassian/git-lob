package main

import (
	"bitbucket.org/sinbad/git-lob/providers/smart"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type MethodFunc func(req *smart.JsonRequest, config *Config, path string) *smart.JsonResponse

var methodMap = map[string]MethodFunc{
	"QueryCaps":      queryCaps,
	"SetEnabledCaps": setCaps,
}

func Serve(config *Config, path string) int {

	// Read input from client on stdin, buffered so we can detect terminators for JSON

	rdr := bufio.NewReader(os.Stdin)
	// we keep reading until stdin is closed
	for {
		jsonbytes, err := rdr.ReadBytes(byte(0))
		if err != nil {
			if err == io.EOF {
				// normal exit
				break
			}
			fmt.Fprintf(os.Stderr, "Unable to read from client: %v\n", err.Error())
			return 21
		}
		// slice off the terminator
		jsonbytes = jsonbytes[:len(jsonbytes)-1]
		var req smart.JsonRequest
		err = json.Unmarshal(jsonbytes, &req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to unmarhsal JSON: %v: %v\n", string(jsonbytes), err.Error())
			return 22
		}

		// Get function to handle method
		f, ok := methodMap[req.Method]
		var resp *smart.JsonResponse
		if !ok {
			// Since it was valid JSON otherwise, send error as response
			resp = smart.NewJsonErrorResponse(req.Id, fmt.Sprintf("Unknown method %v", req.Method))
		} else {
			// method found, process
			resp = f(&req, config, path)
		}
		// There may not have been a JSON response; that might be because method just streams bytes
		// in which case we just ignore this bit
		if resp != nil {
			responseBytes, err := json.Marshal(resp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to marhsal JSON response: %v: %v\n", resp, err.Error())
				return 23
			}
			// null terminate response
			responseBytes = append(responseBytes, byte(0))
			os.Stdout.Write(responseBytes)
		}

		// Ready for next request from client

	}

	return 0
}
