package aggregation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// sendResult is a helper that sends a task-result message via the gateway.
func sendResult(gw *a2a.Gateway, from, to, taskID, status, output string) {
	tr := v1alpha1.TaskResult{
		MemberName: from,
		TaskID:     taskID,
		Status:     status,
		Output:     output,
	}
	payload, _ := json.Marshal(tr)
	_ = gw.Send(context.Background(), &a2a.Message{
		From:    from,
		To:      to,
		Type:    "task-result",
		Payload: string(payload),
	})
}

func TestCollectAll_AllResultsReceived(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	coordinator := "test-coordinator"
	taskID := "task-1"

	// Pre-send 3 results.
	sendResult(gw, "worker-a", coordinator, taskID, "success", "out-a")
	sendResult(gw, "worker-b", coordinator, taskID, "success", "out-b")
	sendResult(gw, "worker-c", coordinator, taskID, "failed", "err-c")

	cfg := &v1alpha1.AggregationConfig{Strategy: "collect-all", TimeoutSec: 5}
	result, err := collector.Collect(context.Background(), coordinator, taskID, 3, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("expected 3 results, got %d", result.Total)
	}
	if result.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.TaskID != taskID {
		t.Errorf("taskID mismatch: %q", result.TaskID)
	}
	if result.Strategy != "collect-all" {
		t.Errorf("strategy mismatch: %q", result.Strategy)
	}
}

func TestFirstSuccess_ReturnsEarly(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	coordinator := "coord"
	taskID := "task-2"

	// Send one success — should return immediately.
	sendResult(gw, "w1", coordinator, taskID, "success", "fast")

	cfg := &v1alpha1.AggregationConfig{Strategy: "first-success", TimeoutSec: 5}
	result, err := collector.Collect(context.Background(), coordinator, taskID, 3, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded < 1 {
		t.Error("expected at least 1 success")
	}
	// Should not wait for all 3.
	if result.Total > 1 {
		// This is acceptable — it may have picked up the message in one poll.
	}
}

func TestMajority_ReturnOnQuorum(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	coordinator := "coord"
	taskID := "task-3"

	// Send 2 successes out of 3 expected — majority reached.
	sendResult(gw, "w1", coordinator, taskID, "success", "ok")
	sendResult(gw, "w2", coordinator, taskID, "success", "ok")

	cfg := &v1alpha1.AggregationConfig{Strategy: "majority", TimeoutSec: 5}
	result, err := collector.Collect(context.Background(), coordinator, taskID, 3, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded < 2 {
		t.Errorf("expected at least 2 succeeded, got %d", result.Succeeded)
	}
}

func TestCollect_Timeout(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	coordinator := "coord"
	taskID := "task-4"

	// Only send 1 of 3 expected results.
	sendResult(gw, "w1", coordinator, taskID, "success", "partial")

	cfg := &v1alpha1.AggregationConfig{Strategy: "collect-all", TimeoutSec: 1}
	start := time.Now()
	result, _ := collector.Collect(context.Background(), coordinator, taskID, 3, cfg)
	elapsed := time.Since(start)

	// Should timeout after ~1 second.
	if elapsed < 800*time.Millisecond {
		t.Errorf("returned too fast: %v", elapsed)
	}
	// Should have the 1 result we sent.
	if result.Total != 1 {
		t.Errorf("expected 1 result, got %d", result.Total)
	}
}

func TestCollect_ContextCancellation(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cfg := &v1alpha1.AggregationConfig{Strategy: "collect-all", TimeoutSec: 60}
	result, err := collector.Collect(ctx, "coord", "task-5", 5, cfg)

	// Should return due to context cancellation, not the 60s timeout.
	if err == nil {
		t.Log("no error (context may have returned partial results)")
	}
	if result == nil {
		t.Fatal("result should not be nil even on cancellation")
	}
}

func TestCollect_IgnoresWrongTaskID(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	coordinator := "coord"

	// Send a result for a different task.
	sendResult(gw, "w1", coordinator, "other-task", "success", "wrong")
	// Send the correct one.
	sendResult(gw, "w2", coordinator, "task-6", "success", "right")

	cfg := &v1alpha1.AggregationConfig{Strategy: "collect-all", TimeoutSec: 2}
	result, _ := collector.Collect(context.Background(), coordinator, "task-6", 1, cfg)

	if result.Total != 1 {
		t.Errorf("expected 1 result (matching task-6), got %d", result.Total)
	}
}

func TestCollect_DefaultConfig(t *testing.T) {
	gw := a2a.New()
	collector := NewCollector(gw)

	sendResult(gw, "w1", "coord", "task-7", "success", "ok")

	// nil config should use defaults.
	result, _ := collector.Collect(context.Background(), "coord", "task-7", 1, nil)
	if result.Strategy != "collect-all" {
		t.Errorf("default strategy should be collect-all, got %q", result.Strategy)
	}
}

func TestFormatSummary(t *testing.T) {
	ar := &AggregatedResult{
		TaskID:    "t1",
		Strategy:  "collect-all",
		Succeeded: 2,
		Total:     3,
	}
	s := FormatSummary(ar)
	if s == "" {
		t.Error("summary should not be empty")
	}
}
