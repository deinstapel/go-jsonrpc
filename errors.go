package jsonrpc

import "errors"

var ErrTransportClosed = errors.New("transport closed")

// ErrInvalidParams is returned when the params of a request are invalid.
var ErrInvalidParams error = newRpcErr(nil, invalidParamsErr, "Invalid params")

// ErrInternal is returned when an internal error occurs in a handler or the
var ErrInternal error = newRpcErr(nil, internalErr, "Internal error")

// error factories for internal use
var errParseError = newRpcErr(nil, jsonParseErr, "Parse error")
var errInvalidRequest = newRpcErr(nil, invalidRequestErr, "Invalid Request")
var errMethodNotFound = func(id string) *RPCErr { return newRpcErr(&id, methodNotFoundErr, "Method not found") }
var errInvalidParams = func(id string) *RPCErr { return newRpcErr(&id, internalErr, "Invalid params") }
var errInternal = func(id string) *RPCErr { return newRpcErr(&id, internalErr, "Internal error") }

var ErrMethodAlreadyRegistered = errors.New("Method already registered")
var ErrAlreadySubscribed = errors.New("Already subscribed")

const jsonParseErr = -32700
const invalidRequestErr = -32600
const methodNotFoundErr = -32601
const invalidParamsErr = -32602
const internalErr = -32603

type RPCErr struct {
	code    int
	id      *string
	message string
}

func (e *RPCErr) Error() string {
	return e.message
}
func (e *RPCErr) Code() int {
	return e.code
}
func (e *RPCErr) ID() string {
	if e.id == nil {
		return ""
	}
	return *e.id
}

// NewRPCErr creates a new RPC error with the given code, and message.
func NewRPCErr(code int, message string) *RPCErr {
	return newRpcErr(nil, code, message)
}

func newRpcErr(id *string, code int, message string) *RPCErr {
	return &RPCErr{
		code:    code,
		message: message,
		id:      id,
	}
}
