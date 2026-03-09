// Package sharding implements task distribution strategies across matrix members.
package sharding

import (
	"fmt"
	"hash/fnv"
	"sync/atomic"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// Task represents a unit of work to be distributed.
type Task struct {
	ID      string            `json:"id"`
	Payload string            `json:"payload"`
	Key     string            `json:"key,omitempty"` // used by hash strategy
	Meta    map[string]string `json:"meta,omitempty"`
}

// Assignment maps a task to a matrix member.
type Assignment struct {
	MemberName string `json:"memberName"`
	Task       Task   `json:"task"`
}

// Strategy defines the interface for task distribution algorithms.
type Strategy interface {
	Distribute(tasks []Task, members []v1alpha1.MatrixMember) []Assignment
}

// NewStrategy creates a sharding strategy from a config.
func NewStrategy(cfg *v1alpha1.ShardingConfig) Strategy {
	if cfg == nil {
		return &RoundRobinStrategy{}
	}
	switch cfg.Strategy {
	case "hash":
		return &HashStrategy{}
	case "balanced":
		return &BalancedStrategy{}
	default:
		return &RoundRobinStrategy{}
	}
}

// RoundRobinStrategy distributes tasks evenly across members in order.
type RoundRobinStrategy struct {
	counter atomic.Uint64
}

func (s *RoundRobinStrategy) Distribute(tasks []Task, members []v1alpha1.MatrixMember) []Assignment {
	if len(members) == 0 {
		return nil
	}
	assignments := make([]Assignment, 0, len(tasks))
	for _, t := range tasks {
		idx := s.counter.Add(1) - 1
		member := members[idx%uint64(len(members))]
		assignments = append(assignments, Assignment{
			MemberName: member.Name,
			Task:       t,
		})
	}
	return assignments
}

// HashStrategy distributes tasks based on a hash of the task key.
type HashStrategy struct{}

func (s *HashStrategy) Distribute(tasks []Task, members []v1alpha1.MatrixMember) []Assignment {
	if len(members) == 0 {
		return nil
	}
	assignments := make([]Assignment, 0, len(tasks))
	for _, t := range tasks {
		key := t.Key
		if key == "" {
			key = t.ID
		}
		h := fnv.New32a()
		fmt.Fprint(h, key)
		idx := h.Sum32() % uint32(len(members))
		assignments = append(assignments, Assignment{
			MemberName: members[idx].Name,
			Task:       t,
		})
	}
	return assignments
}

// BalancedStrategy tries to distribute tasks evenly, assigning to the member
// with the fewest assigned tasks so far.
type BalancedStrategy struct{}

func (s *BalancedStrategy) Distribute(tasks []Task, members []v1alpha1.MatrixMember) []Assignment {
	if len(members) == 0 {
		return nil
	}
	counts := make(map[string]int, len(members))
	for _, m := range members {
		counts[m.Name] = 0
	}

	assignments := make([]Assignment, 0, len(tasks))
	for _, t := range tasks {
		// Find member with fewest assignments.
		minName := members[0].Name
		minCount := counts[minName]
		for _, m := range members[1:] {
			if counts[m.Name] < minCount {
				minName = m.Name
				minCount = counts[m.Name]
			}
		}
		counts[minName]++
		assignments = append(assignments, Assignment{
			MemberName: minName,
			Task:       t,
		})
	}
	return assignments
}
