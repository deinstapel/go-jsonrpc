package transports

import (
	"context"
	"sync/atomic"

	"github.com/deinstapel/go-jsonrpc"
	"github.com/gorilla/websocket"
)

type conn struct {
	c         *websocket.Conn
	onClose   func()
	writeChan chan []byte
	closeFlag atomic.Bool
}

// Returns a channel of incoming messages
// This channel is closed when the connection is closed
func (c *conn) Messages() <-chan []byte {
	incoming := make(chan []byte)
	go func() {
		defer close(incoming)
		for {
			t, msg, err := c.c.ReadMessage()
			if err != nil {
				c.Close()
				return
			}
			if t != websocket.TextMessage {
				continue
			}

			incoming <- msg
		}
	}()
	return incoming
}

// Close closes the connection
// This is a no-op if connection is already closed
func (c *conn) Close() {
	if c.closeFlag.CompareAndSwap(false, true) {
		c.c.Close()
		close(c.writeChan)
		c.onClose()
	}
}

// OnClose sets a function to be called when the connection is closed
// Is guaranteed to be called exactly once per connection
// If invoked multiple times, overwrites the previous function
func (c *conn) OnClose(fn func()) {
	c.onClose = fn
}

// runWrite runs the write loop
func (c *conn) runWrite(ctx context.Context) {
	for {
		select {
		case m := <-c.writeChan:
			if err := c.c.WriteMessage(websocket.TextMessage, m); err != nil {
				c.Close()
				return
			}
		case <-ctx.Done():
			c.Close()
			return
		}
	}
}

// Send sends a preserialized message to the client
// This is a no-op if connection is already closed
func (c *conn) Send(msg []byte) error {
	if c.closeFlag.Load() {
		return nil
	}

	c.writeChan <- msg
	return nil
}

// NewWebsocketTransport creates a new websocket transport
// based on the provided connection
func NewWebsocketTransport(ctx context.Context, wsConn *websocket.Conn) jsonrpc.Transport {
	c := &conn{
		c:         wsConn,
		writeChan: make(chan []byte, 5),
		closeFlag: atomic.Bool{},
	}

	go c.runWrite(ctx)

	return c
}
