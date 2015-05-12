package smart

import (
	"io"
	"net/url"
)

type TransportProgressCallback func(bytesDone, totalBytes int64)

// The transport interface abstracts away how the smart provider talks to the server
// It might do this over a persistent SSH connection, sending data across in/out streams,
// or it might process each request as a discrete request/response pair over REST
// Note each transport instance is stateful and associated with a server/connection, see SmartTransportFactory for how they are created
type Transport interface {
	// Release any resources associated with this transport (including closing any persistent connections)
	Release()
	// Ask the server for a list of capabilities
	QueryCaps() ([]string, error)
	// Request that the server enable capabilities for this exchange (note, non-persistent transports can store & send this with every request)
	SetEnabledCaps(caps []string) error

	// Return whether LOB metadata exists on the server (also returns size)
	MetadataExists(lobsha string) (ex bool, sz int64, e error)
	// Return whether LOB chunk content exists on the server
	ChunkExists(lobsha string, chunk int) (ex bool, sz int64, e error)
	// Return whether LOB chunk content exists on the server, and is of a specific size
	ChunkExistsAndIsOfSize(lobsha string, chunk int, sz int64) (bool, error)
	// Entire LOB exists? Also returns entire content size
	LOBExists(lobsha string) (ex bool, sz int64, e error)

	// Upload metadata for a LOB (from a stream); no progress callback as very small
	UploadMetadata(lobsha string, sz int64, data io.Reader) error
	// Upload chunk content for a LOB (from a stream); must call back progress
	UploadChunk(lobsha string, chunk int, sz int64, data io.Reader, callback TransportProgressCallback) error
	// Download metadata for a LOB (to a stream); no progress callback as very small
	DownloadMetadata(lobsha string, out io.Writer) error
	// Download chunk content for a LOB (from a stream); must call back progress
	// This is a non-delta download operation, just provide entire chunk content
	DownloadChunk(lobsha string, chunk int, out io.Writer, callback TransportProgressCallback) error

	// Return the LOB which the server has a complete copy of, from a list of candidates
	// Server must test in the order provided & return the earliest one which is complete on the server
	// Server doesn't have to test full integrity of LOB, just completeness (check size against meta)
	// Return a blank string if none are available
	GetFirstCompleteLOBFromList(candidateSHAs []string) (string, error)
	// Upload a binary delta to apply against a LOB the server already has, to generate a new LOB
	// Deltas apply to whole LOB content and are not per-chunk
	// Returns a boolean to determine whether the upload was accepted or not (server may prefer not to accept, not an error)
	// In the case of false return, client will fall back to non-delta upload.
	// On true, server must return nil error only after data is fully received, applied, saved as targetSHA and the
	// integrity confirmed by recalculating the SHA of the final patched data.
	UploadDelta(baseSHA, targetSHA string, deltaSize int64, data io.Reader, callback TransportProgressCallback) (bool, error)
	// Prepare a binary delta between 2 LOBs and report the size
	DownloadDeltaPrepare(baseSHA, targetSHA string) (int64, error)
	// Generate (if not already cached) and download a binary delta that the client can apply locally to generate a new LOB
	// Deltas apply to whole LOB content and are not per-chunk
	// The server should respect sizeLimit and if the delta is larger than that, abandon the process
	// Return a bool to indicate whether the delta went ahead or not (client will fall back to non-delta on false)
	DownloadDelta(baseSHA, targetSHA string, sizeLimit int64, out io.Writer, callback TransportProgressCallback) (bool, error)
}

// Interface for a factory which creates persistent transports for use by SmartSyncProvider
type TransportFactory interface {
	// Does this factory want to handle the URL passed in?
	WillHandleUrl(u *url.URL) bool
	// Provide a new, connected (may not be persistent, but if not test connection/auth) transport for given URL
	Connect(u *url.URL) (Transport, error)
}

var (
	transportFactories []TransportFactory
)

// Registers an instance of a SmartTransportFactory for creating connections
// Must only be called from the main thread, not thread safe
// Later factories registered will take precedence over earlier ones (including core)
func RegisterTransportFactory(f TransportFactory) {
	transportFactories = append(transportFactories, f)
}

// Retrieve the best ConnectionFactory for a given URL (or nil)
func GetTransportFactory(u *url.URL) TransportFactory {
	// Iterate in reverse order
	for i := len(transportFactories) - 1; i >= 0; i-- {
		if transportFactories[i].WillHandleUrl(u) {
			return transportFactories[i]
		}
	}
	return nil
}
