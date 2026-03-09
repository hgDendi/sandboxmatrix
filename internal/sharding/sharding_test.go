package sharding

import (
	"fmt"
	"testing"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

var testMembers = []v1alpha1.MatrixMember{
	{Name: "a", Blueprint: "bp-a"},
	{Name: "b", Blueprint: "bp-b"},
	{Name: "c", Blueprint: "bp-c"},
}

func makeTasks(n int) []Task {
	tasks := make([]Task, n)
	for i := range n {
		tasks[i] = Task{ID: fmt.Sprintf("t%d", i), Payload: "data"}
	}
	return tasks
}

func TestRoundRobin_Distribution(t *testing.T) {
	s := &RoundRobinStrategy{}
	tasks := makeTasks(9)
	assignments := s.Distribute(tasks, testMembers)

	if len(assignments) != 9 {
		t.Fatalf("expected 9 assignments, got %d", len(assignments))
	}

	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.MemberName]++
	}
	for _, m := range testMembers {
		if counts[m.Name] != 3 {
			t.Errorf("member %q got %d tasks, expected 3", m.Name, counts[m.Name])
		}
	}
}

func TestRoundRobin_UnevenDistribution(t *testing.T) {
	s := &RoundRobinStrategy{}
	tasks := makeTasks(10)
	assignments := s.Distribute(tasks, testMembers)

	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.MemberName]++
	}
	total := 0
	for _, c := range counts {
		total += c
		if c < 3 || c > 4 {
			t.Errorf("unexpected count %d (expected 3 or 4)", c)
		}
	}
	if total != 10 {
		t.Errorf("total assignments %d, expected 10", total)
	}
}

func TestHash_Deterministic(t *testing.T) {
	s := &HashStrategy{}
	tasks := []Task{
		{ID: "t1", Key: "user-123"},
		{ID: "t2", Key: "user-456"},
		{ID: "t3", Key: "user-123"}, // same key as t1
	}
	assignments := s.Distribute(tasks, testMembers)

	if assignments[0].MemberName != assignments[2].MemberName {
		t.Errorf("same key produced different members: %q vs %q",
			assignments[0].MemberName, assignments[2].MemberName)
	}
}

func TestHash_UsesIDWhenNoKey(t *testing.T) {
	s := &HashStrategy{}
	tasks := []Task{{ID: "task-abc"}}
	assignments := s.Distribute(tasks, testMembers)

	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].MemberName == "" {
		t.Error("member name should not be empty")
	}
}

func TestBalanced_EvenSpread(t *testing.T) {
	s := &BalancedStrategy{}
	tasks := makeTasks(12)
	assignments := s.Distribute(tasks, testMembers)

	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.MemberName]++
	}
	for _, m := range testMembers {
		if counts[m.Name] != 4 {
			t.Errorf("member %q got %d tasks, expected 4", m.Name, counts[m.Name])
		}
	}
}

func TestBalanced_MaxDiffIsOne(t *testing.T) {
	s := &BalancedStrategy{}
	tasks := makeTasks(7)
	assignments := s.Distribute(tasks, testMembers)

	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.MemberName]++
	}
	min, max := 999, 0
	for _, c := range counts {
		if c < min {
			min = c
		}
		if c > max {
			max = c
		}
	}
	if max-min > 1 {
		t.Errorf("imbalanced: max=%d min=%d (diff %d)", max, min, max-min)
	}
}

func TestEmptyTasks(t *testing.T) {
	for _, s := range []Strategy{&RoundRobinStrategy{}, &HashStrategy{}, &BalancedStrategy{}} {
		a := s.Distribute(nil, testMembers)
		if len(a) != 0 {
			t.Errorf("expected 0 assignments for nil tasks, got %d", len(a))
		}
	}
}

func TestEmptyMembers(t *testing.T) {
	for _, s := range []Strategy{&RoundRobinStrategy{}, &HashStrategy{}, &BalancedStrategy{}} {
		a := s.Distribute(makeTasks(5), nil)
		if a != nil {
			t.Errorf("expected nil for nil members, got %d", len(a))
		}
	}
}

func TestSingleMember(t *testing.T) {
	single := []v1alpha1.MatrixMember{{Name: "only"}}
	for _, s := range []Strategy{&RoundRobinStrategy{}, &HashStrategy{}, &BalancedStrategy{}} {
		assignments := s.Distribute(makeTasks(5), single)
		for _, a := range assignments {
			if a.MemberName != "only" {
				t.Errorf("expected member 'only', got %q", a.MemberName)
			}
		}
	}
}

func TestNewStrategy_Variants(t *testing.T) {
	tests := []struct {
		cfg  *v1alpha1.ShardingConfig
		want string
	}{
		{nil, "*sharding.RoundRobinStrategy"},
		{&v1alpha1.ShardingConfig{Strategy: "round-robin"}, "*sharding.RoundRobinStrategy"},
		{&v1alpha1.ShardingConfig{Strategy: "hash"}, "*sharding.HashStrategy"},
		{&v1alpha1.ShardingConfig{Strategy: "balanced"}, "*sharding.BalancedStrategy"},
		{&v1alpha1.ShardingConfig{Strategy: "unknown"}, "*sharding.RoundRobinStrategy"},
	}
	for _, tt := range tests {
		s := NewStrategy(tt.cfg)
		got := fmt.Sprintf("%T", s)
		if got != tt.want {
			t.Errorf("NewStrategy(%v) = %s, want %s", tt.cfg, got, tt.want)
		}
	}
}
