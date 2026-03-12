// Package image provides image building and caching from blueprints.
// Pre-building a blueprint into a Docker image avoids repeating setup
// commands on every sandbox create, significantly reducing startup time.
package image

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/google/uuid"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
)

const imagePrefix = "smx-built/"

// BuildResult holds the result of a blueprint build.
type BuildResult struct {
	ImageID   string `json:"imageId"`
	ImageTag  string `json:"imageTag"`
	Blueprint string `json:"blueprint"`
	Cached    bool   `json:"cached"`
}

// Info holds metadata about a built image.
type Info struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	Blueprint string `json:"blueprint"`
	Size      int64  `json:"size"`
}

// Committer extends the runtime with image commit and query capabilities.
// The Docker runtime satisfies this interface.
type Committer interface {
	CommitImage(ctx context.Context, containerID, reference string) (string, error)
	ListImages(ctx context.Context, refPrefix string) ([]dockerimage.Summary, error)
	InspectImage(ctx context.Context, ref string) (string, error)
	RemoveImage(ctx context.Context, ref string) error
}

// Builder builds Docker images from blueprints.
type Builder struct {
	runtime   runtime.Runtime
	committer Committer
}

// NewBuilder creates a new Builder backed by the given runtime.
// The runtime must also implement Committer (e.g., Docker runtime).
func NewBuilder(rt runtime.Runtime) *Builder {
	b := &Builder{runtime: rt}
	if c, ok := rt.(Committer); ok {
		b.committer = c
	}
	return b
}

// Tag returns the expected image tag for a blueprint.
func Tag(bp *v1alpha1.Blueprint) string {
	version := bp.Metadata.Version
	if version == "" {
		version = "latest"
	}
	return imagePrefix + bp.Metadata.Name + ":" + version
}

// Prefix returns the prefix used for built images.
func Prefix() string {
	return imagePrefix
}

// Build creates a Docker image from a blueprint by:
// 1. Creating a temporary container from the base image
// 2. Running all setup commands
// 3. Committing the container as a new image
// 4. Destroying the temporary container
func (b *Builder) Build(ctx context.Context, blueprintPath string) (*BuildResult, error) {
	if b.committer == nil {
		return nil, fmt.Errorf("runtime does not support image building")
	}

	start := time.Now()

	// Parse and validate blueprint.
	bp, errs := blueprint.ValidateFile(blueprintPath)
	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid blueprint: %v", errs[0])
	}

	tag := Tag(bp)

	slog.Info("building image from blueprint",
		"blueprint", bp.Metadata.Name,
		"tag", tag,
		"steps", len(bp.Spec.Setup),
	)

	// Create temporary container.
	tmpName := "smx-build-" + bp.Metadata.Name + "-" + generateShortID()
	cfg := &runtime.CreateConfig{
		Name:  tmpName,
		Image: bp.Spec.Base,
		Labels: map[string]string{
			"sandboxmatrix/build":     "true",
			"sandboxmatrix/blueprint": bp.Metadata.Name,
		},
	}

	id, err := b.runtime.Create(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create build container: %w", err)
	}
	defer func() {
		// Always clean up the temporary build container.
		if destroyErr := b.runtime.Destroy(ctx, id); destroyErr != nil {
			slog.Warn("failed to destroy build container", "id", id, "error", destroyErr)
		}
	}()

	// Start container.
	if err := b.runtime.Start(ctx, id); err != nil {
		return nil, fmt.Errorf("start build container: %w", err)
	}

	// Run setup commands.
	for i, step := range bp.Spec.Setup {
		slog.Info("running setup step", "step", i+1, "command", step.Run)
		result, err := b.runtime.Exec(ctx, id, &runtime.ExecConfig{
			Cmd: []string{"sh", "-c", step.Run},
		})
		if err != nil {
			return nil, fmt.Errorf("setup step %d failed: %w", i+1, err)
		}
		if result.ExitCode != 0 {
			return nil, fmt.Errorf("setup step %d exited with code %d: %s", i+1, result.ExitCode, step.Run)
		}
	}

	// Commit container as image with the full tag reference.
	imageID, err := b.committer.CommitImage(ctx, id, tag)
	if err != nil {
		return nil, fmt.Errorf("commit build image: %w", err)
	}

	slog.Info("image built successfully",
		"blueprint", bp.Metadata.Name,
		"imageTag", tag,
		"duration", time.Since(start),
	)

	return &BuildResult{
		ImageID:   imageID,
		ImageTag:  tag,
		Blueprint: bp.Metadata.Name,
	}, nil
}

// IsCached checks if a pre-built image exists for the given blueprint.
// Returns the image tag and true if cached, or empty string and false if not.
func (b *Builder) IsCached(ctx context.Context, bp *v1alpha1.Blueprint) (string, bool) {
	if b.committer == nil {
		return "", false
	}

	tag := Tag(bp)
	imageID, err := b.committer.InspectImage(ctx, tag)
	if err != nil || imageID == "" {
		return "", false
	}
	return tag, true
}

// ListBuiltImages returns all images built by the image builder.
func (b *Builder) ListBuiltImages(ctx context.Context) ([]Info, error) {
	if b.committer == nil {
		return nil, fmt.Errorf("runtime does not support image listing")
	}

	images, err := b.committer.ListImages(ctx, imagePrefix)
	if err != nil {
		return nil, fmt.Errorf("list built images: %w", err)
	}

	var result []Info
	for i := range images {
		for _, repoTag := range images[i].RepoTags {
			if !strings.HasPrefix(repoTag, imagePrefix) {
				continue
			}
			// Parse blueprint name from "smx-built/<name>:<version>"
			withoutPrefix := strings.TrimPrefix(repoTag, imagePrefix)
			bpName := withoutPrefix
			if idx := strings.Index(withoutPrefix, ":"); idx >= 0 {
				bpName = withoutPrefix[:idx]
			}
			result = append(result, Info{
				ID:        images[i].ID,
				Tag:       repoTag,
				Blueprint: bpName,
				Size:      images[i].Size,
			})
		}
	}
	return result, nil
}

// Clean removes all images built by the image builder.
func (b *Builder) Clean(ctx context.Context) (int, error) {
	if b.committer == nil {
		return 0, fmt.Errorf("runtime does not support image removal")
	}

	images, err := b.ListBuiltImages(ctx)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, img := range images {
		if err := b.committer.RemoveImage(ctx, img.Tag); err != nil {
			slog.Warn("failed to remove built image", "tag", img.Tag, "error", err)
			continue
		}
		removed++
	}
	return removed, nil
}

// generateShortID returns a short random string for temporary naming.
func generateShortID() string {
	return uuid.New().String()[:8]
}
