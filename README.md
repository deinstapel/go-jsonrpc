# go-jsonrpc

A modern json-rpc 2.0 compatible library for go 1.22+.

The library is strictly conformant to the standard and supports:

- Incoming Calls, Typed with Generics
- Outgoing Calls, ~~Typed with Generics~~
- Incoming Notifications, Typed with Generics
- Outgoing Notificiations, ~~Typed with generics~~

## Transports

The actual json-rpc layer is independent from the underlying transport.
JSON-RPC should work on any reliable transport, ordering is not important.

Currently, we ship a binding for Websockets using gorilla/websocket and a
binding for a standard net.Conn.

Feel free to contribute more bindings
