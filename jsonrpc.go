package jsonrpc

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"

	"github.com/rs/xid"
)

type jsonRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	ID      string          `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRpcError   `json:"error,omitempty"`
}

type serializationError struct {
	JSONRPC string       `json:"jsonrpc"`
	Error   jsonRpcError `json:"error"`
	ID      *string      `json:"id"`
}

type peer struct {
	transport Transport

	callLock sync.Mutex
	calls    map[string]onCallResponse

	registrationLock sync.Mutex
	registrations    map[string]RPCHandler

	subscriptionLock sync.Mutex
	subscriptions    map[string]SubscriptionHandler

	isClosed atomic.Bool

	onClose func()
	opts    *PeerOptions
}

type onCallResponse func(data json.RawMessage, err error)

func NewPeer(ctx context.Context, t Transport, options ...PeerOption) Peer {
	opts := &PeerOptions{
		IDGenerator: func() string { return xid.New().String() },
	}

	for _, option := range options {
		option(opts)
	}

	p := &peer{
		transport:     t,
		calls:         make(map[string]onCallResponse),
		registrations: make(map[string]RPCHandler),
		subscriptions: make(map[string]SubscriptionHandler),
		opts:          opts,
	}

	t.OnClose(func() {
		p.callLock.Lock()
		for _, ch := range p.calls {
			ch(nil, ErrTransportClosed)
		}
		p.calls = nil
		p.callLock.Unlock()

		p.registrationLock.Lock()
		p.registrations = nil
		p.registrationLock.Unlock()

		p.subscriptionLock.Lock()
		p.subscriptions = nil
		p.subscriptionLock.Unlock()

		p.isClosed.Store(true)

		if p.onClose != nil {
			p.onClose()
		}
	})

	go p.run(ctx)
	return p
}

func (p *peer) run(ctx context.Context) {
	msgs := p.transport.Messages()
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case raw, ok := <-msgs:
			if !ok {
				// channel closed -> transport closed
				return
			}
			if raw[0] == '[' {
				var msgs []message
				if err := json.Unmarshal(raw, &msgs); err != nil {
					p.sendError(errParseError)
					continue
				}
				if len(msgs) == 0 {
					p.sendError(errInvalidRequest)
					continue
				}
				for _, msg := range msgs {
					p.handleIncomingMessage(callCtx, msg)
				}
			} else {
				var msg message
				if err := json.Unmarshal(raw, &msg); err != nil {
					p.sendError(errParseError)
					continue
				}
				p.handleIncomingMessage(callCtx, msg)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *peer) handleIncomingMessage(ctx context.Context, msg message) {
	if msg.JSONRPC != "2.0" {
		p.sendError(errInvalidRequest)
		return
	}

	if len(msg.Method) > 0 {
		// incoming message is call
		if len(msg.ID) == 0 {
			// incoming message is notify
			go func() {
				p.subscriptionLock.Lock()
				handler, ok := p.subscriptions[msg.Method]
				p.subscriptionLock.Unlock()

				if ok {
					handler(ctx, msg.Params)
				}
			}()
		} else {
			// incoming message is call
			go func() {
				p.registrationLock.Lock()
				handler, ok := p.registrations[msg.Method]
				p.registrationLock.Unlock()

				if ok {
					result, err := handler(ctx, msg.Params)
					if err != nil {
						if rpcErr, ok := err.(*RPCErr); ok {
							// if the user returned an rpcErr, clone it and set the id
							p.sendError(newRpcErr(&msg.ID, rpcErr.code, rpcErr.message))
						} else {
							p.sendError(errInternal(msg.ID))
						}
						return
					}
					resultBytes, err := json.Marshal(result)
					if err != nil {
						p.sendError(errInternal(msg.ID))
						return
					}
					p.sendJson(message{
						JSONRPC: "2.0",
						Result:  resultBytes,
						ID:      msg.ID,
					})
				} else {
					p.sendError(errMethodNotFound(msg.ID))
				}
			}()
		}
	} else {
		// incoming message is response
		p.callLock.Lock()
		cb, ok := p.calls[msg.ID]
		p.callLock.Unlock()

		if ok {
			var err error

			// if error is set, it overwrites result
			if msg.Error != nil {
				err = newRpcErr(&msg.ID, msg.Error.Code, msg.Error.Message)
				msg.Result = nil
			}

			cb(msg.Result, err)
		}
	}
}

func (p *peer) sendError(err *RPCErr) {
	if err.Code() == jsonParseErr || err.Code() == invalidRequestErr {
		// important to use a serialization error here to
		// ensure id:nil is sent.
		p.sendJson(serializationError{
			JSONRPC: "2.0",
			Error: jsonRpcError{
				Code:    err.Code(),
				Message: err.Error(),
			},
			ID: nil,
		})
		return
	}
	p.sendJson(message{
		JSONRPC: "2.0",
		Error: &jsonRpcError{
			Code:    err.Code(),
			Message: err.Error(),
		},
		ID: err.ID(),
	})
}

func (p *peer) sendJson(data any) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("error marshalling message: %v", err)
		return
	}
	p.transport.Send(dataBytes)
}

func (p *peer) Call(ctx context.Context, method string, params any) (result json.RawMessage, resultErr error) {
	if p.isClosed.Load() {
		return nil, ErrTransportClosed
	}
	id := p.opts.IDGenerator()
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	resultWait := make(chan struct{})
	cb := func(data json.RawMessage, err error) {
		result = data
		resultErr = err
		close(resultWait)
	}

	p.callLock.Lock()
	p.calls[id] = cb
	p.callLock.Unlock()

	p.sendJson(message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
		ID:      id,
	})

	canceled := false
	select {
	case <-ctx.Done():
		resultErr = ctx.Err()
		result = nil
		canceled = true
	case <-resultWait:
	}

	p.callLock.Lock()
	delete(p.calls, id)
	if canceled {
		close(resultWait)
	}
	p.callLock.Unlock()
	return
}

func (p *peer) Notify(method string, params any) error {
	if p.isClosed.Load() {
		return ErrTransportClosed
	}
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	p.sendJson(message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	})
	return nil
}

func (p *peer) RegisterRPC(method string, handler RPCHandler) error {
	if p.isClosed.Load() {
		return ErrTransportClosed
	}
	p.registrationLock.Lock()
	defer p.registrationLock.Unlock()
	if _, ok := p.registrations[method]; ok {
		return ErrMethodAlreadyRegistered
	}
	p.registrations[method] = handler
	return nil
}

func (p *peer) Subscribe(method string, handler SubscriptionHandler) error {
	if p.isClosed.Load() {
		return ErrTransportClosed
	}
	p.subscriptionLock.Lock()
	defer p.subscriptionLock.Unlock()
	if _, ok := p.subscriptions[method]; ok {
		return ErrAlreadySubscribed
	}
	p.subscriptions[method] = handler
	return nil
}

func (p *peer) OnClose(fn func()) {
	p.onClose = fn
}

func (p *peer) Close() {
	p.transport.Close()
}
