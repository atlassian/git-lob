package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestCalculateFileSHA(t *testing.T) {
	// Success condition
	// Create binary file
	f, err := ioutil.TempFile("", "hashtest")
	if err != nil {
		t.Errorf("Unable to create test file")
		return
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	for i := 0; i < 128; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			f.Write(bytes.Repeat([]byte{j}, 16))
		}
	}
	// This was calculated with 'shasum' on Mac OS X with this file content
	const correctSHA = "772157c6ef480852edf921f5924b1ca582b0d78f"

	testSHA, err := CalculateFileSHA(f.Name())
	if err != nil {
		t.Errorf("Error calculating SHA: %v", err)
	}
	if testSHA != correctSHA {
		t.Errorf("Expected SHA to be: %v but CalculateFileSHA reported: %v", correctSHA, testSHA)
	}

	// Fail condition (try invalid file)
	testSHA, err = CalculateFileSHA("/Users/imaginaryperson/this/does/not/exist")
	// Should not panic
	if err == nil {
		t.Error("Expected error on calculating SHA for non-existent file")
	}

}
