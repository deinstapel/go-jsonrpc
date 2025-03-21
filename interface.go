// Package jsonrpc implements the JSON-RPC 2.0 protocol over various transports.
//
// The JSON-RPC 2.0 specification can be found at https://www.jsonrpc.org/specification.
// This package provides a bidirectional peer-to-peer implementation that supports:
// - RPC method calls with responses
// - Notifications without response
// - Subscriptions for ongoing event notification
//
// The core of the package is the Peer interface, which represents a JSON-RPC 2.0
// connection to another peer. It handles message serialization, routing, and lifecycle
// management. The actual transport layer (WebSockets, TCP, etc.) is abstracted through
// the Transport interface.
//
// Example usage:
//
//	transport := NewWebSocketTransport(conn)
//	peer := jsonrpc.NewPeer(context.Background(), transport)
//
//	// Register an RPC handler
//	peer.RegisterRPC("echo", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
//	    var msg string
//	    if err := json.Unmarshal(params, &msg); err != nil {
//	        return nil, err
//	    }
//	    return msg, nil
//	})
//
//	// Subscribe to notifications
//	peer.Subscribe("update", func(ctx context.Context, params json.RawMessage) {
//	    // Handle the notification
//	})
//
//	// Make an RPC call
//	result, err := peer.Call("remote.method", someParams)
//
//	// Send a notification
//	peer.Notify("event.happened", eventData)
package jsonrpc

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Transport wraps an aribtrary underlying connection between both peers.
// The transport channel should be *reliable* (= no message loss).
// Messages *may* arrive out of order.
type Transport interface {
	// Messages returns a channel of incoming messages.
	// The channel should be closed by the transport when the connection is closed.
	Messages() <-chan []byte
	// Send sends a message over the transport.
	// This *must* be a no-op if the connection is already closed.
	Send([]byte) error

	// Close closes the connection.
	// The transport *must* make sure this is a no-op if the connection is already closed.
	Close()

	// OnClose sets a function to be called when the connection is closed.
	// The transport *must* guarantee that this function will be called exactly once per connection.
	OnClose(fn func())
}

// Peer represents a JSON-RPC peer.
type Peer interface {

	// Call sends an RPC request to the peer with the specified method and parameters.
	// If the peer connection is closed, it returns ErrTransportClosed.
	// The params argument is marshaled to JSON before being sent.
	// If you wish to already pass an encoded message into the params argument, use json.RawMessage.
	// If the context is canceled, the context error will be propagated into resultErr
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)

	// Notify sends a notification to the peer with the specified method and parameters.
	// If the peer connection is closed, it returns ErrTransportClosed.
	// The params argument is marshaled to JSON before being sent.
	// If you wish to already pass an encoded message into the params argument, use json.RawMessage.
	Notify(method string, params any) error

	// RegisterRPC registers a handler function for a specific RPC method.
	// The handler will be called when a request for the specified method is received.
	// Handler will be called in its own goroutine
	// Handler Context will be canceled if transport closes or parent context closes
	//
	// Parameters:
	//   - method: The name of the RPC method to register
	//   - handler: The RPCHandler function to call when this method is invoked
	//
	// Returns:
	//   - ErrTransportClosed: If the peer's transport is already closed
	//   - ErrMethodAlreadyRegistered: If the method has already been registered
	//   - nil: On successful registration
	RegisterRPC(method string, handler RPCHandler) error

	// Subscribe registers a handler function for a specific subscription method. If the transport is closed
	// or if a subscription for the given method already exists, it returns an error.
	//
	// The handler will be called when a notification for the subscribed method is received.
	// Handler will be called in its own goroutine
	// Handler Context will be canceled if transport closes or parent context closes
	//
	// Parameters:
	//   - method: The method name to subscribe to.
	//   - handler: The SubscriptionHandler function to be called when notifications arrive.
	//
	// Returns:
	//   - ErrTransportClosed if the transport is closed
	//   - ErrAlreadySubscribed if the method is already subscribed
	//   - nil: on successful subscription
	Subscribe(method string, handler SubscriptionHandler) error

	// OnClose sets a function to be called when the peer is closed
	// Is guaranteed to be called exactly once per peer
	// If invoked multiple times, overwrites the previous function
	// is called after all internal cleanups are done
	// all calls are guaranteed to be canceled before this is called
	OnClose(fn func())

	// Close closes the peer
	// This is a no-op if peer is already closed
	Close()
}

// PeerOptions contains options for creating a new Peer.
type PeerOptions struct {
	IDGenerator func() string
	logger      *slog.Logger
}

// RPCHandler is a function that handles an incoming RPC request.
type RPCHandler func(ctx context.Context, params json.RawMessage) (result json.RawMessage, err error)

// TypedRPCHandler is a function that handles an incoming RPC request with typed parameters and result.
type TypedRPCHandler[Params, Result any] func(ctx context.Context, params Params) (Result, error)

// SubscriptionHandler is a function that handles an incoming subscription notification.
type SubscriptionHandler func(ctx context.Context, params json.RawMessage)

// TypedSubscriptionHandler is a function that handles an incoming subscription notification with typed parameters.
type TypedSubscriptionHandler[Params any] func(ctx context.Context, params Params)

// IDGenerator is a function that generates a unique ID for a message.
// The default IDGenerator generates a random xid.
type IDGenerator func() string

// PeerOption is a function that sets an option on a new Peer.
type PeerOption func(opts *PeerOptions)

// WithIDGenerator controls how IDs are generated for outgoing messages.
// By default, github.com/rs/xid is used.
func WithIDGenerator(gen IDGenerator) PeerOption {
	return func(opts *PeerOptions) {
		opts.IDGenerator = gen
	}
}

// WithLogger sets the logger for the peer.
func WithLogger(logger *slog.Logger) PeerOption {
	return func(opts *PeerOptions) {
		opts.logger = logger
	}
}
