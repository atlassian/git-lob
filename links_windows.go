// +build windows

package main

// Windows-specific dll functions

import (
	"os"
	"syscall"
	"unsafe"
)

// Create a hard link to a file
// This link can be deleted like any other file afterwards
func CreateHardLink(target, link string) error {
	kern32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kern32.NewProc("CreateHardLinkW")
	link16, err := syscall.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	target16, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ret, _, err := proc.Call(
		uintptr(unsafe.Pointer(link16)),
		uintptr(unsafe.Pointer(target16)),
		0)

	if ret == 0 {
		// zero return means failure in Win API
		// err already contains result of GetLastError
		return err
	}
	return nil
}

// Get the number of hard links to a given file (min 1)
func GetHardLinkCount(target string) (linkCount int, err error) {
	// syscall already has a number of windows calls already, including
	// GetFileInformationByHandle, it just doesn't expose the number of links right
	// now, since they don't support hard links on Windows
	// For this reason we can use Go's file open commands & pass the fd to the Win API
	file, err := os.OpenFile(target, os.O_RDONLY, 0666)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var d syscall.ByHandleFileInformation
	err = syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &d)
	if err != nil {
		return 0, err
	}

	return int(d.NumberOfLinks), nil
}
