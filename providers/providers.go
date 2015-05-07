package providers

import (
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"io"
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
	// Validate that the passed in file exists on the named remote
	// filename is relative to the root of the store
	FileExists(remoteName, filename string) bool
	// Validate that the passed in file exists on the named remote and is of the correct size
	// filename is relative to the root of the store
	FileExistsAndIsOfSize(remoteName, filename string, sz int64) bool
}

// Smart sync provider interface with more options
type SmartSyncProvider interface {
	// Everything from core SyncProvider
	SyncProvider

	// Plus entire LOB-oriented calls

	// Whether a LOB exists in full on the remote, and gets its size
	LOBExists(remoteName, sha string) (ex bool, sz int64)
	// Prepare a delta from a list of candidate shas and report the size of it, the chosen base SHA. If this fails caller should use standard Download()
	PrepareDeltaForDownload(remoteName, sha string, candidateBaseSHAs []string) (sz int64, base string, e error)
	// Download delta of LOB content (must be applied later)
	DownloadDelta(remoteName, basesha, targetsha string, out io.Writer, callback SyncProgressCallback) error
	// Return the LOB which the server has a complete copy of, from a list of candidates
	// Server must test in the order provided & return the earliest one which is complete on the server
	// Server doesn't have to test full integrity of LOB, just completeness (check size against meta)
	// Return a blank string if none are available
	GetFirstCompleteLOBFromList(remoteName string, candidateSHAs []string) (string, error)
	// Upload delta of LOB content (must be calculated first)
	UploadDelta(remoteName, basesha, targetsha string, in io.Reader, size int64, callback SyncProgressCallback) error
}

// Callback when progress is made uploading / downloading
// fileInProgress: relative path of file, isSkipped: whether file was up to date, bytesDone/totalBytes: progress for current file
// return true to abort the process for this and all other files in the batch
type SyncProgressCallback func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool)

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

// Retrieve all sync providers, keyed on name
func GetSyncProviders() map[string]SyncProvider {
	return syncProviders
}

// Retrieve a SyncProvider with the associated typeID
func GetSyncProvider(typeID string) (SyncProvider, error) {
	p, ok := syncProviders[typeID]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Requested unknown SyncProvider: %v", typeID))
	}
	return p, nil
}

// 'Upgrade' a pointer to a SyncProvider to a SmartSyncProvider, if possible (returns nil if not)
func UpgradeToSmartSyncProvider(provider SyncProvider) SmartSyncProvider {
	switch p := provider.(type) {
	case SmartSyncProvider:
		return p
	default:
		return nil
	}

}

// Install the core providers
func InitCoreProviders() {
	RegisterSyncProvider(&FileSystemSyncProvider{})
	RegisterSyncProvider(&S3SyncProvider{})
}

// Get the provider name specified for the named remote in the current git repo
// May return "" if not specified
func GetProviderNameForRemote(remoteName string) string {
	return util.GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.git-lob-provider", remoteName)]
}

// Get the provider for a given remote, and validate that it's configured correctly
func GetProviderForRemote(remoteName string) (SyncProvider, error) {
	providerName := GetProviderNameForRemote(remoteName)
	if providerName == "" {
		return nil, fmt.Errorf("Config parameter 'git-lob-provider' is missing from remote '%v'", remoteName)
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

// This is a passthrough reader that reports progress, for cases where you need to give a Reader to a lower level process
type SyncProgressReader struct {
	internalReader io.Reader
	filename       string
	totalBytes     int64
	callback       SyncProgressCallback

	Aborted   bool
	BytesRead int64
}

func (self *SyncProgressReader) Read(p []byte) (n int, err error) {
	// Make sure that we call progress callback on a reasonable frequency
	const readerBufferSize = 131072
	pos := 0
	n = 0
	for remainder := len(p); remainder > 0; {
		readlen := readerBufferSize
		if remainder < readlen {
			readlen = remainder
		}
		var c int
		c, err = self.internalReader.Read(p[pos : pos+readlen])
		n += c
		self.BytesRead += int64(c)
		if c > 0 && self.callback != nil && self.totalBytes > 0 {
			if self.callback(self.filename, util.ProgressTransferBytes, int64(self.BytesRead), self.totalBytes) {
				// Abort if requested
				self.Aborted = true
				return
			}
		}
		if err != nil {
			break
		}
		pos += c
		remainder -= c

	}
	return
}

func NewSyncProgressReader(r io.Reader, filename string, totalBytes int64, callback SyncProgressCallback) *SyncProgressReader {
	return &SyncProgressReader{r, filename, totalBytes, callback, false, 0}
}
