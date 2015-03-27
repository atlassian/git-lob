package core

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
)

const NUMBUFS = 4

type BufferData struct {
	BufIdx int
	Count  int
}

func CalculateFileSHA(fullpath string) (retsha string, err error) {

	// Channel indicating data in a buffer is ready
	datachan := make(chan BufferData, NUMBUFS)
	// Channel indicating a buffer is free to insert data into
	bufferfreechan := make(chan int, NUMBUFS)
	// 4x the buffers & we'll allocate BUFSIZE sections
	mainbuf := make([]byte, BUFSIZE*NUMBUFS)
	bufslicearray := make([][]byte, NUMBUFS)
	for i := 0; i < NUMBUFS; i++ {
		bufslicearray[i] = mainbuf[i*BUFSIZE : (i+1)*BUFSIZE]
		// Initialise free channel too
		bufferfreechan <- i
	}
	// Open file
	f, err := os.Open(fullpath)
	if err != nil {
		return "", errors.New("Can't open file to sha " + fullpath)
	}
	defer f.Close()

	// Start goroutine to read data (async I/O)
	go func(f *os.File, freechan chan int, datachan chan BufferData) {
		for {
			// Get free buffer to write to
			bufidx := <-freechan
			slice := bufslicearray[bufidx]
			n, fileerr := f.Read(slice)
			datachan <- BufferData{bufidx, n}
			if fileerr != nil {
				if fileerr != io.EOF {
					err = fileerr
				}
				// includes EOF
				break
			}
		}
		// Indicate completion
		close(datachan)
	}(f, bufferfreechan, datachan)

	// Receive data from datachan, process then add buffer back to free list
	sha := sha1.New()
	for resp := range datachan {
		if resp.Count > 0 {
			dataslice := bufslicearray[resp.BufIdx]
			sha.Write(dataslice[:resp.Count])
		}
		// Return buffer used to free list
		bufferfreechan <- resp.BufIdx
	}

	return fmt.Sprintf("%x", string(sha.Sum(nil))), err
}
