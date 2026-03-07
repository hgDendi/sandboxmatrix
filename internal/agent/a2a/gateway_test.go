package a2a

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSendAndReceive(t *testing.T) {
	gw := New()
	ctx := context.Background()

	msg := &Message{
		From:    "sandbox-a",
		To:      "sandbox-b",
		Type:    "request",
		Payload: `{"action":"ping"}`,
	}

	if err := gw.Send(ctx, msg); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	msgs, err := gw.Receive(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Receive: unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "sandbox-a" {
		t.Errorf("expected From=sandbox-a, got %q", msgs[0].From)
	}
	if msgs[0].To != "sandbox-b" {
		t.Errorf("expected To=sandbox-b, got %q", msgs[0].To)
	}
	if msgs[0].Type != "request" {
		t.Errorf("expected Type=request, got %q", msgs[0].Type)
	}
	if msgs[0].Payload != `{"action":"ping"}` {
		t.Errorf("unexpected payload: %q", msgs[0].Payload)
	}
	if msgs[0].ID == "" {
		t.Error("expected non-empty ID")
	}
	if msgs[0].CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestReceiveClearsInbox(t *testing.T) {
	gw := New()
	ctx := context.Background()

	msg := &Message{
		From:    "sandbox-a",
		To:      "sandbox-b",
		Type:    "event",
		Payload: `{"data":"hello"}`,
	}
	if err := gw.Send(ctx, msg); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	// First receive should return the message.
	msgs, err := gw.Receive(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Receive: unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Second receive should return empty.
	msgs, err = gw.Receive(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Receive (second): unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(msgs))
	}
}

func TestPeekDoesNotClear(t *testing.T) {
	gw := New()
	ctx := context.Background()

	msg := &Message{
		From:    "sandbox-a",
		To:      "sandbox-b",
		Type:    "event",
		Payload: `{"data":"peek-test"}`,
	}
	if err := gw.Send(ctx, msg); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	// Peek should return the message.
	msgs, err := gw.Peek(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Peek: unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from Peek, got %d", len(msgs))
	}

	// Peek again should still return the message.
	msgs, err = gw.Peek(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Peek (second): unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from second Peek, got %d", len(msgs))
	}

	// Receive should also return the message (and then clear).
	msgs, err = gw.Receive(ctx, "sandbox-b")
	if err != nil {
		t.Fatalf("Receive: unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message from Receive after Peek, got %d", len(msgs))
	}
}

func TestBroadcast(t *testing.T) {
	gw := New()
	ctx := context.Background()

	targets := []string{"sandbox-b", "sandbox-c", "sandbox-d"}
	err := gw.Broadcast(ctx, "sandbox-a", targets, "event", `{"msg":"broadcast"}`)
	if err != nil {
		t.Fatalf("Broadcast: unexpected error: %v", err)
	}

	for _, target := range targets {
		msgs, err := gw.Receive(ctx, target)
		if err != nil {
			t.Fatalf("Receive for %q: unexpected error: %v", target, err)
		}
		if len(msgs) != 1 {
			t.Errorf("expected 1 message for %q, got %d", target, len(msgs))
			continue
		}
		if msgs[0].From != "sandbox-a" {
			t.Errorf("expected From=sandbox-a for %q, got %q", target, msgs[0].From)
		}
		if msgs[0].To != target {
			t.Errorf("expected To=%q, got %q", target, msgs[0].To)
		}
		if msgs[0].Type != "event" {
			t.Errorf("expected Type=event for %q, got %q", target, msgs[0].Type)
		}
		if msgs[0].Payload != `{"msg":"broadcast"}` {
			t.Errorf("unexpected payload for %q: %q", target, msgs[0].Payload)
		}
	}
}

func TestSubscribeHandler(t *testing.T) {
	gw := New()
	ctx := context.Background()

	var mu sync.Mutex
	var received []Message

	gw.Subscribe("sandbox-b", func(msg *Message) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, *msg)
	})

	msg := &Message{
		From:    "sandbox-a",
		To:      "sandbox-b",
		Type:    "request",
		Payload: `{"action":"test"}`,
	}
	if err := gw.Send(ctx, msg); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	// Handler is called synchronously, so no need to wait.
	// But give a small window just in case.
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected handler to receive 1 message, got %d", len(received))
	}
	if received[0].From != "sandbox-a" {
		t.Errorf("handler got From=%q, expected sandbox-a", received[0].From)
	}
}

func TestReceiveEmptyInbox(t *testing.T) {
	gw := New()
	ctx := context.Background()

	msgs, err := gw.Receive(ctx, "nonexistent-sandbox")
	if err != nil {
		t.Fatalf("Receive: unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty inbox, got %d", len(msgs))
	}
}
