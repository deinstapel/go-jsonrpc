package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypedRPC(t *testing.T) {
	t.Run("successful handling", func(t *testing.T) {
		handler := TypedRPC(func(ctx context.Context, params string) (int, error) {
			if params == "test" {
				return 42, nil
			}
			return 0, errors.New("invalid input")
		})

		params, _ := json.Marshal("test")
		result, err := handler(context.Background(), params)

		assert.NoError(t, err)
		var value int
		err = json.Unmarshal(result, &value)
		assert.NoError(t, err)
		assert.Equal(t, 42, value)
	})

	t.Run("invalid params", func(t *testing.T) {
		handler := TypedRPC(func(ctx context.Context, params int) (string, error) {
			return "result", nil
		})

		// Invalid JSON for an int
		result, err := handler(context.Background(), json.RawMessage(`"not an int"`))

		assert.Error(t, err)
		assert.Equal(t, ErrInvalidParams, err)
		assert.Nil(t, result)
	})

	t.Run("handler error", func(t *testing.T) {
		expectedErr := errors.New("handler error")
		handler := TypedRPC(func(ctx context.Context, params string) (int, error) {
			return 0, expectedErr
		})

		params, _ := json.Marshal("test")
		result, err := handler(context.Background(), params)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})
}

func TestTypedSubscription(t *testing.T) {
	t.Run("successful subscription", func(t *testing.T) {
		called := false
		handler := TypedSubscription(func(ctx context.Context, params string) {
			called = true
			assert.Equal(t, "test", params)
		})

		params, _ := json.Marshal("test")
		handler(context.Background(), params)

		assert.True(t, called)
	})

	t.Run("invalid params", func(t *testing.T) {
		called := false
		handler := TypedSubscription(func(ctx context.Context, params int) {
			called = true
		})

		// Invalid JSON for an int
		handler(context.Background(), json.RawMessage(`"not an int"`))

		assert.False(t, called, "Handler should not be called with invalid params")
	})
}
