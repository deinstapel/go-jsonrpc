package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockTransport struct {
	messages   chan []byte
	sendLock   sync.Mutex
	sentData   [][]byte
	onCloseFn  func()
	closeChan  chan struct{}
	closeOnce  sync.Once
	isClosed   bool
	sendDelay  time.Duration
	receiveMsg chan []byte
	t          *testing.T
}

func newMockTransport(t *testing.T) *mockTransport {
	return &mockTransport{
		messages:   make(chan []byte, 10),
		sentData:   make([][]byte, 0),
		closeChan:  make(chan struct{}),
		receiveMsg: make(chan []byte, 10),
		t:          t,
	}
}

func (m *mockTransport) Send(data []byte) error {
	m.t.Logf("Sending data: %s", data)
	m.sendLock.Lock()
	defer m.sendLock.Unlock()
	if m.isClosed {
		return ErrTransportClosed
	}
	m.sentData = append(m.sentData, data)
	if m.sendDelay > 0 {
		time.Sleep(m.sendDelay)
	}
	if m.receiveMsg != nil {
		m.receiveMsg <- data
	}
	return nil
}

func (m *mockTransport) Messages() <-chan []byte {
	return m.messages
}

func (m *mockTransport) Close() {
	m.closeOnce.Do(func() {
		m.sendLock.Lock()
		m.isClosed = true
		m.sendLock.Unlock()
		close(m.messages)
		close(m.closeChan)
		if m.onCloseFn != nil {
			m.onCloseFn()
		}
	})
}

func (m *mockTransport) OnClose(fn func()) {
	m.onCloseFn = fn
}

func (m *mockTransport) injectMessage(msg []byte) {
	if !m.isClosed {
		m.messages <- msg
	}
}

func (m *mockTransport) getSentData() [][]byte {
	m.sendLock.Lock()
	defer m.sendLock.Unlock()
	return m.sentData
}

func TestPeer_Call(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport, WithIDGenerator(func() string { return "test-id" }))

	// Start a goroutine to handle the incoming call request
	go func() {

		rawMessage, ok := <-transport.receiveMsg

		if !ok {
			t.Error("no message received")
			return
		}

		var msg message
		if err := json.Unmarshal(rawMessage, &msg); err != nil {
			t.Error("failed to unmarshal message:", err)
			return
		}

		t.Logf("Received message: %v, replying", msg)

		// Send a response back
		resp := message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(`"response data"`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.injectMessage(respBytes)
	}()

	// Make the call
	result, err := peer.Call(context.Background(), "test.method", "test data")
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var responseData string
	if err := json.Unmarshal(result, &responseData); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if responseData != "response data" {
		t.Errorf("Expected response 'response data', got '%s'", responseData)
	}
}

func TestPeer_CallWithContextCancel(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Make the call with a context that will be canceled
	_, err := peer.Call(ctx, "test.method", "test data")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled error, got: %v", err)
	}
}

func TestPeer_Notify(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport)

	err := peer.Notify("test.event", "notification data")
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	sentData := transport.getSentData()
	if len(sentData) != 1 {
		t.Fatalf("Expected 1 message sent, got %d", len(sentData))
	}

	var msg message
	if err := json.Unmarshal(sentData[0], &msg); err != nil {
		t.Fatalf("Failed to unmarshal notification: %v", err)
	}

	if msg.Method != "test.event" {
		t.Errorf("Expected method 'test.event', got '%s'", msg.Method)
	}

	var params string
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}

	if params != "notification data" {
		t.Errorf("Expected params 'notification data', got '%s'", params)
	}
}

func TestPeer_RegisterRPC(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport)

	// Register an RPC handler
	handler := RPCHandler(func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var input string
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		return json.Marshal("Echo: " + input)
	})

	err := peer.RegisterRPC("echo", handler)
	if err != nil {
		t.Fatalf("RegisterRPC failed: %v", err)
	}

	// Trying to register the same method again should fail
	err = peer.RegisterRPC("echo", handler)
	if err != ErrMethodAlreadyRegistered {
		t.Fatalf("Expected ErrMethodAlreadyRegistered, got: %v", err)
	}

	// Send a request to the registered method
	req := message{
		JSONRPC: "2.0",
		Method:  "echo",
		ID:      "req-1",
		Params:  json.RawMessage(`"hello"`),
	}
	reqBytes, _ := json.Marshal(req)
	transport.injectMessage(reqBytes)

	// Wait for the response
	time.Sleep(100 * time.Millisecond)

	sentData := transport.getSentData()
	if len(sentData) != 1 {
		t.Fatalf("Expected 1 message sent, got %d", len(sentData))
	}

	var resp message
	if err := json.Unmarshal(sentData[0], &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.ID != "req-1" {
		t.Errorf("Expected ID 'req-1', got '%s'", resp.ID)
	}

	var result string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	expectedResult := "Echo: hello"
	if result != expectedResult {
		t.Errorf("Expected result '%s', got '%s'", expectedResult, result)
	}
}

func TestPeer_Subscribe(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport)

	notificationReceived := make(chan string, 1)

	// Subscribe to a notification
	handler := func(ctx context.Context, params json.RawMessage) {
		var data string
		if err := json.Unmarshal(params, &data); err != nil {
			t.Errorf("Failed to unmarshal notification params: %v", err)
			return
		}
		notificationReceived <- data
	}

	err := peer.Subscribe("test.event", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Trying to subscribe to the same method again should fail
	err = peer.Subscribe("test.event", handler)
	if err != ErrAlreadySubscribed {
		t.Fatalf("Expected ErrAlreadySubscribed, got: %v", err)
	}

	// Send a notification to the subscribed method
	notification := message{
		JSONRPC: "2.0",
		Method:  "test.event",
		Params:  json.RawMessage(`"event data"`),
	}
	notifyBytes, _ := json.Marshal(notification)
	transport.injectMessage(notifyBytes)

	// Wait for the notification to be processed
	select {
	case data := <-notificationReceived:
		expectedData := "event data"
		if data != expectedData {
			t.Errorf("Expected notification data '%s', got '%s'", expectedData, data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Notification not received within timeout")
	}
}

func TestPeer_SubscribeBatch(t *testing.T) {
	transport := newMockTransport(t)
	peer := NewPeer(context.Background(), transport)

	notificationReceived := make(chan string, 2)

	// Subscribe to a notification
	handler := func(ctx context.Context, params json.RawMessage) {
		var data string
		if err := json.Unmarshal(params, &data); err != nil {
			t.Errorf("Failed to unmarshal notification params: %v", err)
			return
		}
		notificationReceived <- data
	}

	err := peer.Subscribe("test.event", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Trying to subscribe to the same method again should fail
	err = peer.Subscribe("test.event", handler)
	if err != ErrAlreadySubscribed {
		t.Fatalf("Expected ErrAlreadySubscribed, got: %v", err)
	}

	// Send a notification to the subscribed method
	notification := message{
		JSONRPC: "2.0",
		Method:  "test.event",
		Params:  json.RawMessage(`"event data"`),
	}
	notifyBytes, _ := json.Marshal([]message{notification, notification})
	t.Logf("notifyBytes: %s", notifyBytes)
	transport.injectMessage(notifyBytes)

	// Wait for the first notification to be processed
	select {
	case data := <-notificationReceived:
		expectedData := "event data"
		if data != expectedData {
			t.Errorf("Expected notification data '%s', got '%s'", expectedData, data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Notification not received within timeout")
	}

	// Wait for the second notification to be processed
	select {
	case data := <-notificationReceived:
		expectedData := "event data"
		if data != expectedData {
			t.Errorf("Expected notification data '%s', got '%s'", expectedData, data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Notification not received within timeout")
	}
}

func TestPeer_Close(t *testing.T) {
	transport := newMockTransport(t)

	closeHandlerCalled := make(chan struct{})

	peer := NewPeer(context.Background(), transport)
	peer.OnClose(func() {
		close(closeHandlerCalled)
	})

	// Register a few things to make sure cleanup happens
	peer.RegisterRPC("test.method", func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	})

	peer.Subscribe("test.event", func(ctx context.Context, params json.RawMessage) {})

	// Make a call that won't be answered
	callDone := make(chan struct{})
	go func() {
		_, err := peer.Call(context.Background(), "remote.method", "data")
		if err != ErrTransportClosed {
			t.Errorf("Expected ErrTransportClosed, got: %v", err)
		}
		close(callDone)
	}()

	// Close the peer
	peer.Close()

	// Wait for the close handler to be called
	select {
	case <-closeHandlerCalled:
		// Good, this is expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close handler not called within timeout")
	}

	// Wait for the call to finish with an error
	select {
	case <-callDone:
		// Good, this is expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Call didn't finish within timeout")
	}

	// Further operations should fail
	err := peer.RegisterRPC("another.method", func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	})
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed from RegisterRPC, got: %v", err)
	}

	err = peer.Subscribe("another.event", func(ctx context.Context, params json.RawMessage) {})
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed from Subscribe, got: %v", err)
	}

	_, err = peer.Call(context.Background(), "another.method", "data")
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed from Call, got: %v", err)
	}

	err = peer.Notify("another.event", "data")
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed from Notify, got: %v", err)
	}
}
