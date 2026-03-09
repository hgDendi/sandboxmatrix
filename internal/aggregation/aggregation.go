// Package aggregation implements result collection strategies for distributed tasks.
package aggregation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// AggregatedResult holds the combined results from all matrix members.
type AggregatedResult struct {
	TaskID    string                `json:"taskID"`
	Strategy  string                `json:"strategy"`
	Results   []v1alpha1.TaskResult `json:"results"`
	Total     int                   `json:"total"`
	Succeeded int                   `json:"succeeded"`
	Failed    int                   `json:"failed"`
}

// Collector gathers results from matrix members via A2A messages.
type Collector struct {
	gateway *a2a.Gateway
}

// NewCollector creates a new result collector.
func NewCollector(gateway *a2a.Gateway) *Collector {
	return &Collector{gateway: gateway}
}

// Collect waits for results from the specified members, polling the coordinator's
// A2A inbox for "task-result" messages. It returns when all expected results are
// received or the timeout expires.
func (c *Collector) Collect(ctx context.Context, coordinatorName string, taskID string, expectedCount int, cfg *v1alpha1.AggregationConfig) (*AggregatedResult, error) {
	strategy := "collect-all"
	timeoutSec := 60
	if cfg != nil {
		if cfg.Strategy != "" {
			strategy = cfg.Strategy
		}
		if cfg.TimeoutSec > 0 {
			timeoutSec = cfg.TimeoutSec
		}
	}

	deadline := time.After(time.Duration(timeoutSec) * time.Second)
	var results []v1alpha1.TaskResult

	for {
		select {
		case <-ctx.Done():
			return c.buildResult(taskID, strategy, results), ctx.Err()
		case <-deadline:
			return c.buildResult(taskID, strategy, results), nil
		default:
		}

		// Poll for task-result messages only, preserving other message types.
		msgs, err := c.gateway.ReceiveByType(ctx, coordinatorName, "task-result")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		for _, msg := range msgs {
			var tr v1alpha1.TaskResult
			if err := json.Unmarshal([]byte(msg.Payload), &tr); err != nil {
				continue
			}
			if tr.TaskID != taskID {
				continue
			}
			results = append(results, tr)
		}

		// Check completion based on strategy.
		switch strategy {
		case "first-success":
			for _, r := range results {
				if r.Status == "success" {
					return c.buildResult(taskID, strategy, results), nil
				}
			}
		case "majority":
			successCount := 0
			for _, r := range results {
				if r.Status == "success" {
					successCount++
				}
			}
			if successCount > expectedCount/2 {
				return c.buildResult(taskID, strategy, results), nil
			}
		default: // "collect-all"
			if len(results) >= expectedCount {
				return c.buildResult(taskID, strategy, results), nil
			}
		}

		if len(msgs) == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (c *Collector) buildResult(taskID string, strategy string, results []v1alpha1.TaskResult) *AggregatedResult {
	succeeded := 0
	failed := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			succeeded++
		default:
			failed++
		}
	}
	return &AggregatedResult{
		TaskID:    taskID,
		Strategy:  strategy,
		Results:   results,
		Total:     len(results),
		Succeeded: succeeded,
		Failed:    failed,
	}
}

// FormatSummary returns a human-readable summary of the aggregated result.
func FormatSummary(ar *AggregatedResult) string {
	return fmt.Sprintf("Task %s: %d/%d succeeded (strategy: %s)",
		ar.TaskID, ar.Succeeded, ar.Total, ar.Strategy)
}
