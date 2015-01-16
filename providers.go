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
	// of incorrect size.
	// For each file, if the local copy already has this file and it's the same size, skip.
	// Must only return nil if all files were successfully uploaded
	// If force = true, files should be downloaded even if they're already there & the correct size
	// Files not found should not cause an error, since we do not query presence beforehand. Instead,
	// the callback should be called with ProgressNotFound and proceed to the next one.
	Download(remoteName string, filenames []string, toDir string, force bool, callback SyncProgressCallback) error
}

// Callback when progress is made uploading / downloading
// fileInProgress: relative path of file, isSkipped: whether file was up to date, bytesDone/totalBytes: progress for current file
// return true to abort the process for this and all other files in the batch
type SyncProgressCallback func(fileInProgress string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool)

// Providers implementing this interface provide smart sync capabilities
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

// Get the provider name specified for the named remote in the current git repo
// May return "" if not specified
func GetProviderNameForRemote(remoteName string) string {
	return GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.git-lob-provider", remoteName)]
}

// Get the provider for a given remote, and validate that it's configured correctly
func GetProviderForRemote(remoteName string) (SyncProvider, error) {
	providerName := GetProviderNameForRemote(remoteName)
	if providerName == "" {
		return nil, fmt.Errorf("Configuration remote.%v.git-lob-provider is missing", remoteName)
	}
	provider, err := GetSyncProvider(providerName)
	if err != nil {
		return nil, err
	}
	err = provider.ValidateConfig(remoteName)
	if err != nil {
		return nil, err
	}
	return provider, nil
}
