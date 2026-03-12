// Package docker implements the Runtime interface using Docker.
package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
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

// Option is a functional option for configuring the Docker runtime.
type Option func(*Runtime)

// WithOCIRuntime sets the OCI runtime to use (e.g., "runsc" for gVisor).
// When set, containers are created with this runtime instead of the default.
func WithOCIRuntime(name string) Option {
	return func(r *Runtime) {
		r.ociRuntime = name
	}
}

// Runtime implements runtime.Runtime using Docker.
type Runtime struct {
	client     *client.Client
	ociRuntime string // optional OCI runtime override (e.g., "runsc")
}

// New creates a new Docker runtime with default settings.
func New() (*Runtime, error) {
	return NewWithOptions()
}

// NewWithOptions creates a new Docker runtime with the given functional options.
func NewWithOptions(opts ...Option) (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	r := &Runtime{client: cli}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func (r *Runtime) Name() string { return "docker" }

func (r *Runtime) Create(ctx context.Context, cfg *runtime.CreateConfig) (string, error) {
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

	// OCI runtime override (e.g., "runsc" for gVisor).
	if r.ociRuntime != "" {
		hostCfg.Runtime = r.ociRuntime
	}

	// Container security hardening.
	hostCfg.SecurityOpt = []string{"no-new-privileges"}
	hostCfg.CapDrop = []string{"ALL"}
	hostCfg.CapAdd = []string{"NET_BIND_SERVICE"}
	pidsLimit := int64(4096)
	hostCfg.Resources.PidsLimit = &pidsLimit

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

	// GPU passthrough.
	if cfg.GPU != nil && cfg.GPU.Count > 0 {
		driver := cfg.GPU.Driver
		if driver == "" {
			driver = "nvidia"
		}
		count := cfg.GPU.Count
		hostCfg.DeviceRequests = []container.DeviceRequest{
			{
				Driver:       driver,
				Count:        count,
				Capabilities: [][]string{{"gpu"}},
			},
		}
	}

	// Device passthrough (e.g., /dev/kvm, /dev/dri).
	for _, d := range cfg.Devices {
		cPath := d.ContainerPath
		if cPath == "" {
			cPath = d.HostPath
		}
		perms := d.Permissions
		if perms == "" {
			perms = "rwm"
		}
		hostCfg.Devices = append(hostCfg.Devices, container.DeviceMapping{
			PathOnHost:        d.HostPath,
			PathInContainer:   cPath,
			CgroupPermissions: perms,
		})
	}

	// Mounts.
	for _, m := range cfg.Mounts {
		hostCfg.Binds = append(hostCfg.Binds, formatBind(m))
	}

	// Network mode.
	networkingCfg := &network.NetworkingConfig{}
	switch cfg.Network.Mode {
	case "none":
		hostCfg.NetworkMode = container.NetworkMode("none")
	case "host":
		hostCfg.NetworkMode = container.NetworkMode("host")
	case "", "bridge":
		// Default Docker bridge — no special config needed.
	default:
		// Custom network name (e.g. an isolated matrix network).
		hostCfg.NetworkMode = container.NetworkMode(cfg.Network.Mode)
		networkingCfg.EndpointsConfig = map[string]*network.EndpointSettings{
			cfg.Network.Mode: {},
		}
	}

	// Custom DNS servers.
	if len(cfg.Network.DNS) > 0 {
		hostCfg.DNS = cfg.Network.DNS
	}

	resp, err := r.client.ContainerCreate(ctx, containerCfg, hostCfg, networkingCfg, nil, cfg.Name)
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

func (r *Runtime) Exec(ctx context.Context, id string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
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
		ID:     safePrefix(cj.ID, 12),
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
		// Populate port mappings from container inspection.
		for port, bindings := range cj.NetworkSettings.Ports {
			containerPort := port.Int()
			proto := port.Proto()
			for _, binding := range bindings {
				hostPort, _ := strconv.Atoi(binding.HostPort)
				info.Ports = append(info.Ports, runtime.PortMapping{
					ContainerPort: containerPort,
					HostPort:      hostPort,
					Protocol:      proto,
				})
			}
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
	for i := range containers {
		name := ""
		if len(containers[i].Names) > 0 {
			name = strings.TrimPrefix(containers[i].Names[0], "/")
		}
		infos = append(infos, runtime.Info{
			ID:     safePrefix(containers[i].ID, 12),
			Name:   name,
			Image:  containers[i].Image,
			State:  containers[i].State,
			Labels: containers[i].Labels,
		})
	}
	return infos, nil
}

const snapshotPrefix = "smx-snapshot/"

func (r *Runtime) Snapshot(ctx context.Context, id, tag string) (string, error) {
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

func (r *Runtime) Restore(ctx context.Context, snapshotID string, cfg *runtime.CreateConfig) (string, error) {
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
	for i := range images {
		for _, repoTag := range images[i].RepoTags {
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
				ID:        images[i].ID,
				Tag:       tag,
				SandboxID: id,
				CreatedAt: time.Unix(images[i].Created, 0),
				Size:      images[i].Size,
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

func (r *Runtime) CreateNetwork(ctx context.Context, name string, internal bool) error {
	_, err := r.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:   "bridge",
		Internal: internal,
		Labels: map[string]string{
			labelManaged: "true",
		},
	})
	if err != nil {
		return fmt.Errorf("create network %s: %w", name, err)
	}
	return nil
}

func (r *Runtime) DeleteNetwork(ctx context.Context, name string) error {
	if err := r.client.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("delete network %s: %w", name, err)
	}
	return nil
}

func (r *Runtime) CopyToContainer(ctx context.Context, id string, destPath string, content io.Reader) error {
	dir := filepath.Dir(destPath)
	name := filepath.Base(destPath)

	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write tar data: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}

	return r.client.CopyToContainer(ctx, id, dir, &buf, container.CopyToContainerOptions{})
}

func (r *Runtime) CopyFromContainer(ctx context.Context, id string, srcPath string) (io.ReadCloser, error) {
	rc, _, err := r.client.CopyFromContainer(ctx, id, srcPath)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}

	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		rc.Close()
		return nil, fmt.Errorf("read tar entry: %w", err)
	}

	return &tarEntryReadCloser{reader: tr, closer: rc}, nil
}

// tarEntryReadCloser wraps a tar.Reader entry and the underlying io.ReadCloser.
type tarEntryReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (t *tarEntryReadCloser) Read(p []byte) (int, error) {
	return t.reader.Read(p)
}

func (t *tarEntryReadCloser) Close() error {
	return t.closer.Close()
}

// CommitImage commits a container as an image with the given reference string.
func (r *Runtime) CommitImage(ctx context.Context, containerID, reference string) (string, error) {
	resp, err := r.client.ContainerCommit(ctx, containerID, container.CommitOptions{
		Reference: reference,
		Comment:   fmt.Sprintf("smx image build: %s", reference),
		Pause:     true,
	})
	if err != nil {
		return "", fmt.Errorf("commit container as image: %w", err)
	}
	return resp.ID, nil
}

// ListImages returns all Docker images matching the given reference prefix.
func (r *Runtime) ListImages(ctx context.Context, refPrefix string) ([]image.Summary, error) {
	f := filters.NewArgs()
	f.Add("reference", refPrefix+"*")
	return r.client.ImageList(ctx, image.ListOptions{Filters: f})
}

// InspectImage checks if an image exists locally and returns its ID.
func (r *Runtime) InspectImage(ctx context.Context, ref string) (string, error) {
	inspect, err := r.client.ImageInspect(ctx, ref)
	if err != nil {
		return "", err
	}
	return inspect.ID, nil
}

// RemoveImage removes a Docker image by reference.
func (r *Runtime) RemoveImage(ctx context.Context, ref string) error {
	_, err := r.client.ImageRemove(ctx, ref, image.RemoveOptions{
		Force:         true,
		PruneChildren: true,
	})
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}
	return nil
}

func (r *Runtime) ensureImage(ctx context.Context, ref string) error {
	_, err := r.client.ImageInspect(ctx, ref)
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

func (r *Runtime) buildLabels(cfg *runtime.CreateConfig) map[string]string {
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

// safePrefix returns at most the first n characters of s.
func safePrefix(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
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
