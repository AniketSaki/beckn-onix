package definition

import "context"

// Virtualizer defines the interface for invoking a Restate virtualizer service.
type Virtualizer interface {
	// Invoke sends the beckn request payload to the Restate server for the given
	// service name, returning any invocation error.
	Invoke(ctx context.Context, service string, payload []byte) error
}

// VirtualizerProvider initializes a new Virtualizer instance with the given config.
type VirtualizerProvider interface {
	New(ctx context.Context, config map[string]string) (Virtualizer, func(), error)
}
