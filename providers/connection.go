package providers

import (
	"io"
	"net/url"
)

// Our definition of the interface we need to communicate with a smart server
type Connection interface {
	// Import methods from ReadWriteCloser
	io.ReadWriteCloser
}

// Interface for a factory which creates connections for a type of service
type ConnectionFactory interface {
	// Does this factory want to handle the URL passed in?
	WillHandleUrl(u *url.URL) bool
	// Provide a new connection
	Connect(u *url.URL) (Connection, error)
}

var (
	connectionFactories []ConnectionFactory
)

// Registers an instance of a ConnectionFactory for creating connections
// Must only be called from the main thread, not thread safe
// Later factories registered will take precedence over earlier ones (including core)
func RegisterConnectionFactory(f ConnectionFactory) {
	connectionFactories = append(connectionFactories, f)
}

// Retrieve the best ConnectionFactory for a given URL (or nil)
func GetConnectionFactory(u *url.URL) ConnectionFactory {
	// Iterate in reverse order
	for i := len(connectionFactories) - 1; i > 0; i-- {
		if connectionFactories[i].WillHandleUrl(u) {
			return connectionFactories[i]
		}
	}
	return nil
}

// Install the core providers
func InitCoreConnectionFactories() {
	// Only SSH for now
	RegisterSshConnectionFactory()
}
