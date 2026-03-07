// Package docker implements the Runtime interface using Docker.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

const (
	labelPrefix  = "sandboxmatrix/"
	labelManaged = labelPrefix + "managed"
	labelName    = labelPrefix + "name"
)

// Runtime implements runtime.Runtime using Docker.
type Runtime struct {
	client *client.Client
}

// New creates a new Docker runtime.
func New() (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Runtime{client: cli}, nil
}

func (r *Runtime) Name() string { return "docker" }

func (r *Runtime) Create(ctx context.Context, cfg runtime.CreateConfig) (string, error) {
	// Ensure image exists locally; pull if not.
	if err := r.ensureImage(ctx, cfg.Image); err != nil {
		return "", err
	}

	// Build container config.
	containerCfg := &container.Config{
		Image:  cfg.Image,
		Labels: r.buildLabels(cfg),
		Tty:    true,
	}
	if len(cfg.Cmd) > 0 {
		containerCfg.Cmd = cfg.Cmd
	}
	if len(cfg.Env) > 0 {
		for k, v := range cfg.Env {
			containerCfg.Env = append(containerCfg.Env, k+"="+v)
		}
	}

	// Exposed ports.
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range cfg.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		containerPort := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		exposedPorts[containerPort] = struct{}{}
		if p.HostPort > 0 {
			portBindings[containerPort] = []nat.PortBinding{
				{HostPort: strconv.Itoa(p.HostPort)},
			}
		}
	}
	containerCfg.ExposedPorts = exposedPorts

	// Host config.
	hostCfg := &container.HostConfig{
		PortBindings: portBindings,
	}

	// Resource limits.
	if cfg.Memory != "" {
		mem, err := parseMemory(cfg.Memory)
		if err == nil {
			hostCfg.Resources.Memory = mem
		}
	}
	if cfg.CPU != "" {
		cpus, err := strconv.ParseFloat(cfg.CPU, 64)
		if err == nil {
			hostCfg.Resources.NanoCPUs = int64(cpus * 1e9)
		}
	}

	// Mounts.
	for _, m := range cfg.Mounts {
		hostCfg.Binds = append(hostCfg.Binds, formatBind(m))
	}

	resp, err := r.client.ContainerCreate(ctx, containerCfg, hostCfg, &network.NetworkingConfig{}, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

func (r *Runtime) Start(ctx context.Context, id string) error {
	return r.client.ContainerStart(ctx, id, container.StartOptions{})
}

func (r *Runtime) Stop(ctx context.Context, id string) error {
	return r.client.ContainerStop(ctx, id, container.StopOptions{})
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	return r.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

func (r *Runtime) Exec(ctx context.Context, id string, cfg runtime.ExecConfig) (runtime.ExecResult, error) {
	execCfg := container.ExecOptions{
		Cmd:          cfg.Cmd,
		AttachStdout: cfg.Stdout != nil,
		AttachStderr: cfg.Stderr != nil,
		AttachStdin:  cfg.Stdin != nil,
		Tty:          cfg.TTY,
	}
	if cfg.Dir != "" {
		execCfg.WorkingDir = cfg.Dir
	}
	for k, v := range cfg.Env {
		execCfg.Env = append(execCfg.Env, k+"="+v)
	}

	execID, err := r.client.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("exec create: %w", err)
	}

	resp, err := r.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{Tty: cfg.TTY})
	if err != nil {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()

	// Stream I/O.
	if cfg.Stdin != nil && cfg.TTY {
		go func() {
			_, _ = io.Copy(resp.Conn, cfg.Stdin)
		}()
	}

	outWriter := cfg.Stdout
	if outWriter == nil {
		outWriter = io.Discard
	}

	if cfg.TTY {
		_, _ = io.Copy(outWriter, resp.Reader)
	} else {
		errWriter := cfg.Stderr
		if errWriter == nil {
			errWriter = io.Discard
		}
		_, _ = stdcopy.StdCopy(outWriter, errWriter, resp.Reader)
	}

	// Get exit code.
	inspect, err := r.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("exec inspect: %w", err)
	}
	return runtime.ExecResult{ExitCode: inspect.ExitCode}, nil
}

func (r *Runtime) Info(ctx context.Context, id string) (runtime.Info, error) {
	cj, err := r.client.ContainerInspect(ctx, id)
	if err != nil {
		return runtime.Info{}, fmt.Errorf("inspect container: %w", err)
	}
	info := runtime.Info{
		ID:     cj.ID[:12],
		Name:   strings.TrimPrefix(cj.Name, "/"),
		Image:  cj.Config.Image,
		State:  cj.State.Status,
		Labels: cj.Config.Labels,
	}
	if cj.NetworkSettings != nil {
		for _, nw := range cj.NetworkSettings.Networks {
			info.IP = nw.IPAddress
			break
		}
	}
	return info, nil
}

func (r *Runtime) Stats(ctx context.Context, id string) (runtime.Stats, error) {
	resp, err := r.client.ContainerStats(ctx, id, false)
	if err != nil {
		return runtime.Stats{}, fmt.Errorf("container stats: %w", err)
	}
	defer resp.Body.Close()

	var statsResp container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&statsResp); err != nil {
		return runtime.Stats{}, fmt.Errorf("decode stats: %w", err)
	}

	// Calculate CPU percentage.
	cpuDelta := float64(statsResp.CPUStats.CPUUsage.TotalUsage - statsResp.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsResp.CPUStats.SystemUsage - statsResp.PreCPUStats.SystemUsage)
	numCPUs := float64(statsResp.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = 1.0
	}

	var cpuPercent float64
	if systemDelta > 0 && cpuDelta >= 0 {
		cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	return runtime.Stats{
		CPUUsage:    cpuPercent,
		MemoryUsage: statsResp.MemoryStats.Usage,
		MemoryLimit: statsResp.MemoryStats.Limit,
	}, nil
}

func (r *Runtime) List(ctx context.Context) ([]runtime.Info, error) {
	f := filters.NewArgs()
	f.Add("label", labelManaged+"=true")

	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	infos := make([]runtime.Info, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		infos = append(infos, runtime.Info{
			ID:     c.ID[:12],
			Name:   name,
			Image:  c.Image,
			State:  c.State,
			Labels: c.Labels,
		})
	}
	return infos, nil
}

const snapshotPrefix = "smx-snapshot/"

func (r *Runtime) Snapshot(ctx context.Context, id string, tag string) (string, error) {
	// Look up container to get the sandbox name from labels.
	cj, err := r.client.ContainerInspect(ctx, id)
	if err != nil {
		return "", fmt.Errorf("inspect container for snapshot: %w", err)
	}

	sandboxName := cj.Config.Labels[labelName]
	if sandboxName == "" {
		sandboxName = strings.TrimPrefix(cj.Name, "/")
	}

	ref := snapshotPrefix + sandboxName + ":" + tag

	resp, err := r.client.ContainerCommit(ctx, id, container.CommitOptions{
		Reference: ref,
		Comment:   fmt.Sprintf("smx snapshot of %s", sandboxName),
		Pause:     true,
	})
	if err != nil {
		return "", fmt.Errorf("commit container: %w", err)
	}

	return resp.ID, nil
}

func (r *Runtime) Restore(ctx context.Context, snapshotID string, cfg runtime.CreateConfig) (string, error) {
	// Use the snapshot image instead of the blueprint image.
	cfg.Image = snapshotID

	return r.Create(ctx, cfg)
}

func (r *Runtime) ListSnapshots(ctx context.Context, id string) ([]runtime.SnapshotInfo, error) {
	// Look up container to get the sandbox name.
	cj, err := r.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container for listing snapshots: %w", err)
	}

	sandboxName := cj.Config.Labels[labelName]
	if sandboxName == "" {
		sandboxName = strings.TrimPrefix(cj.Name, "/")
	}

	refPrefix := snapshotPrefix + sandboxName

	f := filters.NewArgs()
	f.Add("reference", refPrefix)

	images, err := r.client.ImageList(ctx, image.ListOptions{
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("list snapshot images: %w", err)
	}

	var snapshots []runtime.SnapshotInfo
	for _, img := range images {
		for _, repoTag := range img.RepoTags {
			if !strings.HasPrefix(repoTag, snapshotPrefix) {
				continue
			}
			// Parse tag from "smx-snapshot/<name>:<tag>"
			parts := strings.SplitN(repoTag, ":", 2)
			tag := ""
			if len(parts) == 2 {
				tag = parts[1]
			}
			snapshots = append(snapshots, runtime.SnapshotInfo{
				ID:        img.ID,
				Tag:       tag,
				SandboxID: id,
				CreatedAt: time.Unix(img.Created, 0),
				Size:      img.Size,
			})
		}
	}

	return snapshots, nil
}

func (r *Runtime) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	_, err := r.client.ImageRemove(ctx, snapshotID, image.RemoveOptions{
		Force:         true,
		PruneChildren: true,
	})
	if err != nil {
		return fmt.Errorf("remove snapshot image: %w", err)
	}
	return nil
}

func (r *Runtime) ensureImage(ctx context.Context, ref string) error {
	_, _, err := r.client.ImageInspectWithRaw(ctx, ref)
	if err == nil {
		return nil // already present
	}

	reader, err := r.client.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer reader.Close()
	// Drain to complete the pull.
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (r *Runtime) buildLabels(cfg runtime.CreateConfig) map[string]string {
	labels := make(map[string]string)
	for k, v := range cfg.Labels {
		labels[k] = v
	}
	labels[labelManaged] = "true"
	labels[labelName] = cfg.Name
	return labels
}

func formatBind(m runtime.Mount) string {
	bind := m.Source + ":" + m.Target
	if m.ReadOnly {
		bind += ":ro"
	}
	return bind
}

// parseMemory converts strings like "2Gi", "512Mi" to bytes.
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "Gi") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "Gi"), 64)
		if err != nil {
			return 0, err
		}
		return int64(v * 1024 * 1024 * 1024), nil
	}
	if strings.HasSuffix(s, "Mi") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "Mi"), 64)
		if err != nil {
			return 0, err
		}
		return int64(v * 1024 * 1024), nil
	}
	return strconv.ParseInt(s, 10, 64)
}
