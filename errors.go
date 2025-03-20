package jsonrpc

import "errors"

var ErrTransportClosed = errors.New("transport closed")
var ErrParseError = newRpcErr(nil, jsonParseErr, "Parse error")
var ErrInvalidRequest = newRpcErr(nil, invalidRequestErr, "Invalid Request")
var ErrMethodNotFound = newRpcErr(nil, methodNotFoundErr, "Method not found")
var ErrInvalidParams = newRpcErr(nil, invalidParamsErr, "Invalid params")
var ErrInternal = newRpcErr(nil, internalErr, "Internal error")

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

func newRpcErr(id *string, code int, message string) *RPCErr {
	return &RPCErr{
		code:    code,
		message: message,
		id:      id,
	}
}
