// Package pool manages pre-warmed sandbox instances ready for immediate use.
package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
)

const (
	// LabelPool marks a container as belonging to the pre-warmed pool.
	LabelPool = "sandboxmatrix/pool"
	// LabelPoolBlueprint records which blueprint the pooled container was
	// created from.
	LabelPoolBlueprint = "sandboxmatrix/pool-blueprint"
)

// Config configures a pool for a blueprint.
type Config struct {
	BlueprintPath string
	MinReady      int // minimum warm instances to maintain
	MaxSize       int // maximum pool size
}

// Stats holds observable statistics for a single blueprint pool.
type Stats struct {
	BlueprintPath string
	MinReady      int
	MaxSize       int
	Ready         int
	InUse         int
	TotalCreated  int64
	AvgClaimTime  time.Duration
}

// BlueprintPool holds warm instances for a specific blueprint.
type BlueprintPool struct {
	Blueprint string
	MinReady  int
	MaxSize   int
	Ready     []string // container IDs ready to be claimed
	Creating  int      // number currently being created

	// Stats tracking.
	inUse        int
	totalCreated int64
	totalClaims  int64
	totalClaimNs int64 // cumulative nanoseconds spent on claims

	// The parsed blueprint image used when creating warm containers.
	image string
	// The original blueprint path (needed for on-demand creation).
	blueprintPath string

	// Per-blueprint notification channel for the warm loop.
	notify chan struct{}
}

// Pool manages pre-warmed sandbox instances ready for immediate use.
type Pool struct {
	mu      sync.Mutex
	runtime runtime.Runtime
	pools   map[string]*BlueprintPool // blueprint path -> pool
	store   state.Store

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new Pool manager.
func New(rt runtime.Runtime, store state.Store) *Pool {
	return &Pool{
		runtime: rt,
		pools:   make(map[string]*BlueprintPool),
		store:   store,
	}
}

// Configure sets up a pool for a blueprint.
func (p *Pool) Configure(cfg Config) error {
	if cfg.BlueprintPath == "" {
		return fmt.Errorf("blueprint path is required")
	}
	if cfg.MinReady < 0 {
		return fmt.Errorf("min-ready must be >= 0")
	}
	if cfg.MaxSize < 0 {
		return fmt.Errorf("max-size must be >= 0")
	}
	if cfg.MaxSize > 0 && cfg.MinReady > cfg.MaxSize {
		return fmt.Errorf("min-ready (%d) cannot exceed max-size (%d)", cfg.MinReady, cfg.MaxSize)
	}

	// Parse the blueprint to extract the image name.
	bp, errs := blueprint.ValidateFile(cfg.BlueprintPath)
	if len(errs) > 0 {
		return fmt.Errorf("invalid blueprint: %v", errs[0])
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.pools[cfg.BlueprintPath] = &BlueprintPool{
		Blueprint:     bp.Metadata.Name,
		MinReady:      cfg.MinReady,
		MaxSize:       cfg.MaxSize,
		Ready:         make([]string, 0),
		image:         bp.Spec.Base,
		blueprintPath: cfg.BlueprintPath,
		notify:        make(chan struct{}, 1),
	}

	return nil
}

// Claim takes a warm instance from the pool. Returns the container ID.
// If no warm instances are available, creates one on demand.
func (p *Pool) Claim(ctx context.Context, blueprintPath string) (string, error) {
	start := time.Now()

	p.mu.Lock()
	bp, ok := p.pools[blueprintPath]
	if !ok {
		p.mu.Unlock()
		return "", fmt.Errorf("no pool configured for blueprint %q", blueprintPath)
	}

	if len(bp.Ready) > 0 {
		// Pop the last ready container.
		id := bp.Ready[len(bp.Ready)-1]
		bp.Ready = bp.Ready[:len(bp.Ready)-1]
		bp.inUse++
		elapsed := time.Since(start)
		bp.totalClaims++
		bp.totalClaimNs += elapsed.Nanoseconds()
		notifyCh := bp.notify
		p.mu.Unlock()

		// Signal the warmer to create a replacement.
		select {
		case notifyCh <- struct{}{}:
		default:
		}

		return id, nil
	}

	// No warm instances available; create on demand.
	image := bp.image
	bpName := bp.Blueprint
	p.mu.Unlock()

	id, err := p.createWarmContainer(ctx, image, bpName, blueprintPath)
	if err != nil {
		return "", fmt.Errorf("create on-demand container: %w", err)
	}

	p.mu.Lock()
	bp.inUse++
	elapsed := time.Since(start)
	bp.totalClaims++
	bp.totalClaimNs += elapsed.Nanoseconds()
	p.mu.Unlock()

	return id, nil
}

// Release returns an instance to the pool (or destroys it if pool is full).
func (p *Pool) Release(ctx context.Context, containerID, blueprintPath string) error {
	p.mu.Lock()
	bp, ok := p.pools[blueprintPath]
	if !ok {
		p.mu.Unlock()
		// No pool configured; just destroy.
		return p.runtime.Destroy(ctx, containerID)
	}

	if bp.inUse > 0 {
		bp.inUse--
	}

	// Check if pool is at capacity.
	total := len(bp.Ready) + bp.inUse
	if bp.MaxSize > 0 && total >= bp.MaxSize {
		p.mu.Unlock()
		return p.runtime.Destroy(ctx, containerID)
	}

	bp.Ready = append(bp.Ready, containerID)
	p.mu.Unlock()
	return nil
}

// Warm starts background goroutines to maintain minimum pool levels.
func (p *Pool) Warm(ctx context.Context) error {
	// Cancel previous warm goroutines if any.
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	p.mu.Lock()
	blueprints := make([]string, 0, len(p.pools))
	for path := range p.pools {
		blueprints = append(blueprints, path)
	}
	p.mu.Unlock()

	// Start a warming goroutine for each configured blueprint.
	for _, bpPath := range blueprints {
		p.wg.Add(1)
		go p.warmLoop(ctx, bpPath)
	}

	return nil
}

// warmLoop maintains the minimum pool level for a single blueprint.
// Each blueprint has its own notification channel, eliminating cross-
// goroutine message bouncing.
func (p *Pool) warmLoop(ctx context.Context, blueprintPath string) {
	defer p.wg.Done()

	p.mu.Lock()
	bp, ok := p.pools[blueprintPath]
	if !ok {
		p.mu.Unlock()
		return
	}
	notifyCh := bp.notify
	p.mu.Unlock()

	// Do an initial fill.
	p.fillPool(ctx, blueprintPath)

	for {
		select {
		case <-ctx.Done():
			return
		case <-notifyCh:
			p.fillPool(ctx, blueprintPath)
		}
	}
}

// fillPool creates containers until the pool has at least MinReady instances.
func (p *Pool) fillPool(ctx context.Context, blueprintPath string) {
	for {
		if ctx.Err() != nil {
			return
		}

		p.mu.Lock()
		bp, ok := p.pools[blueprintPath]
		if !ok {
			p.mu.Unlock()
			return
		}

		needed := bp.MinReady - len(bp.Ready) - bp.Creating
		if needed <= 0 {
			p.mu.Unlock()
			return
		}

		// Check max size constraint.
		if bp.MaxSize > 0 {
			total := len(bp.Ready) + bp.inUse + bp.Creating
			if total >= bp.MaxSize {
				p.mu.Unlock()
				return
			}
		}

		bp.Creating++
		image := bp.image
		bpName := bp.Blueprint
		p.mu.Unlock()

		id, err := p.createWarmContainer(ctx, image, bpName, blueprintPath)

		p.mu.Lock()
		bp.Creating--
		if err == nil {
			bp.Ready = append(bp.Ready, id)
		}
		p.mu.Unlock()
	}
}

// createWarmContainer creates and starts a container suitable for the pool.
func (p *Pool) createWarmContainer(ctx context.Context, image, blueprintName, blueprintPath string) (string, error) {
	cfg := &runtime.CreateConfig{
		Name:  fmt.Sprintf("smx-pool-%s-%d", blueprintName, time.Now().UnixNano()),
		Image: image,
		Labels: map[string]string{
			LabelPool:               "true",
			LabelPoolBlueprint:      blueprintPath,
			"sandboxmatrix/managed": "true",
			"sandboxmatrix/name":    fmt.Sprintf("pool-%s", blueprintName),
		},
	}

	id, err := p.runtime.Create(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := p.runtime.Start(ctx, id); err != nil {
		// Best-effort cleanup.
		_ = p.runtime.Destroy(ctx, id)
		return "", fmt.Errorf("start container: %w", err)
	}

	p.mu.Lock()
	if bp, ok := p.pools[blueprintPath]; ok {
		bp.totalCreated++
	}
	p.mu.Unlock()

	return id, nil
}

// Stats returns pool statistics for all configured blueprints.
func (p *Pool) Stats() map[string]Stats {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make(map[string]Stats, len(p.pools))
	for path, bp := range p.pools {
		var avg time.Duration
		if bp.totalClaims > 0 {
			avg = time.Duration(bp.totalClaimNs / bp.totalClaims)
		}
		result[path] = Stats{
			BlueprintPath: path,
			MinReady:      bp.MinReady,
			MaxSize:       bp.MaxSize,
			Ready:         len(bp.Ready),
			InUse:         bp.inUse,
			TotalCreated:  bp.totalCreated,
			AvgClaimTime:  avg,
		}
	}
	return result
}

// Drain destroys all warm instances and stops background warming.
func (p *Pool) Drain(ctx context.Context) error {
	// Stop background warming goroutines.
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}

	p.mu.Lock()
	// Collect all ready container IDs.
	var ids []string
	for _, bp := range p.pools {
		ids = append(ids, bp.Ready...)
		bp.Ready = bp.Ready[:0]
	}
	p.mu.Unlock()

	// Destroy all collected containers.
	var firstErr error
	for _, id := range ids {
		if err := p.runtime.Destroy(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
