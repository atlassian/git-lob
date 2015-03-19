package main

import (
	"fmt"
)

// Custom error type to indicate an integrity error, listing problem SHAs
type IntegrityError struct {
	FailedSHAs        []string
	AdditionalMessage string
}

func (i *IntegrityError) Error() string {
	ret := fmt.Sprintf("One or more SHAs failed integrity: %v", i.FailedSHAs)
	if i.AdditionalMessage != "" {
		ret = fmt.Sprintf("%v\n%v", ret, i.AdditionalMessage)
	}
	return ret
}

// Create a new IntegrityError
func NewIntegrityError(shas []string) error {
	return &IntegrityError{shas, ""}
}
func NewIntegrityErrorWithAdditionalMessage(shas []string, msg string) error {
	return &IntegrityError{shas, msg}
}

// Is an error an IntegrityError?
func IsIntegrityError(err error) bool {
	switch err.(type) {
	case *IntegrityError:
		return true
	default:
		return false
	}
}

// Custom error type to indicate a 'not found' condition
// This type of error may be expected or tolerable so identify separately
type NotFoundError struct {
	Message string
	Path    string
}

func (i *NotFoundError) Error() string {
	return i.Message
}

// Create a new NotFound error
func NewNotFoundError(msg, path string) error {
	return &NotFoundError{msg, path}
}

// Custom error type to indicate a 'not found' condition for a list of SHAs
// This type of error may be expected or tolerable so identify separately
type NotFoundForSHAsError struct {
	SHAsNotFound []string
}

func (i *NotFoundForSHAsError) Error() string {
	return fmt.Sprintf("Data missing for SHAs: %v", i.SHAsNotFound)
}

// Create a new NotFound error
func NewNotFoundForSHAsError(shas []string) error {
	return &NotFoundForSHAsError{shas}
}

// Is an error a NotFoundError?
func IsNotFoundError(err error) bool {
	switch err.(type) {
	case *NotFoundError:
		return true
	case *NotFoundForSHAsError:
		return true
	default:
		return false
	}
}

// Custom error type to indicate a 'wrong size' condition
// This means a file is on disk but is the wrong size
type WrongSizeError struct {
	Message  string
	Filename string
}

func (i *WrongSizeError) Error() string {
	return i.Message
}

// Create a new WrongSize error
func NewWrongSizeError(msg, filename string) error {
	return &WrongSizeError{msg, filename}
}

// Is an error a NotFoundError?
func IsWrongSizeError(err error) bool {
	switch err.(type) {
	case *WrongSizeError:
		return true
	default:
		return false
	}
}
