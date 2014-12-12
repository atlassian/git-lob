package main

import (
	"errors"
	"fmt"
)

// General interface that all sync providers must implement
type SyncProvider interface {
	// Return the type identifier for this provider
	TypeID() string

	// Return this provider's help text summary for inclusion in the response to
	// 'git-lob providers' (where all are listed)
	HelpTextSummary() string

	// Return this provider's full help text, including all details of configuration
	// parameters in gitconfig, for use as a response to
	// 'git-lob provider <TypeID>'
	HelpTextDetail() string

	// Return whether the configuration for a given remote is valid
	ValidateConfig(remoteName string) error
}

// Callback when progress is made uploading / downloading
// fileInProgress: relative path of file, isSkipped: whether file was up to date, percent: progress
// return true to abort the process for this and all other files in the batch
type SyncProgressCallback func(fileInProgress string, isSkipped bool, percent int) (abort bool)

// Providers implementing this interface provide basic sync capabilities
// These providers require no server-side processing, only simple file access
// but are limited to a direct file-based representation of storage and are less efficent
type BasicSyncProvider interface {
	SyncProvider

	// Upload a given list of files (binary storage). The paths are relative to fromDir and are provided
	// like that to make it easier for the provider since it will likely use the same
	// relative paths inside its own storage
	// For each file, if the remote already has this file and it's the same size, skip.
	// Must only return nil if remote is considered fully up to date with these files
	// If force = true, files should be uploaded even if they're already there & the correct size
	Upload(remoteName string, filenames []string, fromDir string, force bool, callback SyncProgressCallback) error
	// Download the list of files (binary storage). The paths are relative and files should
	// be placed relative to toDir. Ideally in-progress downloads should go to other locations
	// and be moved to the final location on success, although git-lob will detect files
	// on incorrect size.
	// There is no need to check the presence of local files before downloading, the caller
	// will have already done that (and if the file is already there, it means the caller
	// wishes for it to be re-downloaded).
	// Must only return nil if all files were successfully uploaded
	Download(remoteName string, filenames []string, toDir string, callback SyncProgressCallback) error
}

// Providers implementing this interface provide smary sync capabilities
// These providers require server-side processing and are free to store data how they like
// so long as they can fulfil the interface. Provides support for binary deltas to
// speed up data transfers in both directions
type SmartSyncProvider interface {
	SyncProvider

	// TODO
}

var (
	syncProviders map[string]SyncProvider = make(map[string]SyncProvider, 0)
)

// Registers an instance of a SyncProvider for later use
// Must only be called from the main thread, not thread safe
// Repeat calls for providers using the same TypeID will overrule previous
func RegisterSyncProvider(p SyncProvider) error {
	// Must implement dumb/smart protocol (or both)
	_, basicok := p.(BasicSyncProvider)
	_, smartok := p.(SmartSyncProvider)

	if !basicok && !smartok {
		return errors.New("Provider must implement at least one of BasicSyncProvider/SmartSyncProvider")
	}

	// Allow overwrite of previously registered
	syncProviders[p.TypeID()] = p

	return nil
}

// Retrieve a SyncProvider with the associated typeID
func GetSyncProvider(typeID string) (SyncProvider, error) {
	p, ok := syncProviders[typeID]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Requested unknown SyncProvider: %v", typeID))
	}
	return p, nil
}

// Install the core providers
func InitCoreProviders() {
	RegisterSyncProvider(&FileSystemSyncProvider{})
}

func cmdListProviders() int {
	fmt.Println()
	fmt.Println("Available remote providers:")
	for _, p := range syncProviders {
		fmt.Println(" *", p.HelpTextSummary())
	}
	fmt.Println()
	return 0
}

func cmdProviderDetails() int {
	fmt.Println()
	// Potentially list many
	ret := 0
	for _, arg := range GlobalOptions.Args {
		p, err := GetSyncProvider(arg)
		if err != nil {
			fmt.Println(err)
			ret++
		} else {
			fmt.Println(p.HelpTextDetail())
			fmt.Println()
		}

	}
	return ret
}
