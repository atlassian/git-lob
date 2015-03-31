package providers

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	"bufio"
	cryptorand "crypto/rand"
	"crypto/sha1"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
)

// generate a list of (relative) file names
// if depth > 0 then generates 'num' files at each level
// and 'numdirs' dirs with 'num' files at each depth level
func GetRandomListOfFilesForTest(num, depth, numdirs int) []string {
	ret := make([]string, 0, num*depth+1)
	// Pre-declare required for anonymous recursion
	var recursefunc func(dir string, depth int)
	sha := sha1.New()

	recursefunc = func(dir string, d int) {
		for f := 0; f < num; f++ {
			// Use SHA to generate unique names
			randStr := strconv.Itoa(rand.Int())
			sha.Write([]byte(randStr))
			shaStr := fmt.Sprintf("%x", string(sha.Sum(nil)))
			ret = append(ret, filepath.Join(dir, fmt.Sprintf("%v.bin", shaStr)))
		}
		if d > 0 {
			// Dirs
			for f := 0; f < numdirs; f++ {
				randStr := strconv.Itoa(rand.Int())
				sha.Write([]byte(randStr))
				shaStr := fmt.Sprintf("%x", string(sha.Sum(nil)))
				subdir := filepath.Join(dir, shaStr[:4])
				recursefunc(subdir, d-1)
			}
		}

	}
	recursefunc("", depth)
	return ret
}

// Create a file with random data of size sz
func CreateRandomFileForTest(sz int64, filename string) {
	os.MkdirAll(filepath.Dir(filename), 0755)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		Fail(fmt.Sprintf("Can't create test file %v: %v", filename, err))
	}
	defer f.Close()
	// random data
	fileWriter := bufio.NewWriter(f)
	_, err = io.CopyN(fileWriter, cryptorand.Reader, sz)
	fileWriter.Flush()
	if err != nil {
		Fail(fmt.Sprintf("Can't write random data to test file %v: %v", filename, err))
	}

}
