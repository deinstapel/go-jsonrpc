package jsonrpc

import (
	"context"
	"encoding/json"
)

func TypedRPC[Params, Result any](cb TypedRPCHandler[Params, Result]) RPCHandler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p Params
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams
		}
		res, err := cb(ctx, p)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(res)
		if err != nil {
			return nil, ErrInternal
		}
		return data, nil
	}
}

func TypedSubscription[Params any](cb TypedSubscriptionHandler[Params]) SubscriptionHandler {
	return func(ctx context.Context, params json.RawMessage) {
		var p Params
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}
		cb(ctx, p)
	}
}
