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
}

func (i *NotFoundError) Error() string {
	return i.Message
}

// Create a new NotFound error
func NewNotFoundError(msg string) error {
	return &NotFoundError{msg}
}

// Is an error a NotFoundError?
func IsNotFoundError(err error) bool {
	switch err.(type) {
	case *NotFoundError:
		return true
	default:
		return false
	}
}
