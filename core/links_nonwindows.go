// +build !windows

package core

import (
	"os"
	"syscall"
)

// Create a hard link to a file
// This link can be deleted like any other file afterwards
func CreateHardLink(target, link string) error {

	// Go supports hard links natively for mac & linux
	return os.Link(target, link)
}

// Get the number of hard links to a given file (min 1)
func GetHardLinkCount(target string) (linkCount int, err error) {
	// number of links is available in stat_t but not translated into Go's FileInfo
	// Go returns the original stat_t result in FileInfo.sys though
	// See source https://golang.org/src/pkg/os/stat_darwin.go (and _linux.go)
	fi, err := os.Stat(target)
	if err != nil {
		return 0, err
	}
	var s *syscall.Stat_t
	s = fi.Sys().(*syscall.Stat_t)
	return int(s.Nlink), nil
}
