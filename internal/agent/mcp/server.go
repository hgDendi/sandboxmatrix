// Package mcp implements an MCP (Model Context Protocol) server that exposes
// sandbox operations as tools, enabling AI agents to create and manage sandboxes.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/hg-dendi/sandboxmatrix/internal/aggregation"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	imagepkg "github.com/hg-dendi/sandboxmatrix/internal/image"
	"github.com/hg-dendi/sandboxmatrix/internal/interpreter"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/sharding"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mcpLimitedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *mcpLimitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}

// Server wraps an MCP server that exposes sandbox operations as tools.
type Server struct {
	mcpServer *server.MCPServer
	ctrl      *controller.Controller
	gateway   *a2a.Gateway
}

// NewServer creates a new MCP server backed by the given controller and A2A gateway.
func NewServer(ctrl *controller.Controller, gateway *a2a.Gateway) *Server {
	s := &Server{ctrl: ctrl, gateway: gateway}

	s.mcpServer = server.NewMCPServer(
		"sandboxmatrix",
		"0.1.0",
	)

	s.registerTools()
	return s
}

// registerTools registers all sandbox management tools on the MCP server.
func (s *Server) registerTools() {
	// sandbox_create
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_create",
			mcp.WithDescription("Create a new sandbox from a blueprint"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name for the new sandbox"),
			),
			mcp.WithString("blueprint",
				mcp.Required(),
				mcp.Description("Path to the blueprint YAML file"),
			),
			mcp.WithString("workspace",
				mcp.Description("Local directory to mount as workspace (optional)"),
			),
		),
		s.handleSandboxCreate,
	)

	// sandbox_list
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_list",
			mcp.WithDescription("List all sandboxes"),
		),
		s.handleSandboxList,
	)

	// sandbox_exec
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_exec",
			mcp.WithDescription("Execute a command in a running sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to execute the command in"),
			),
			mcp.WithString("command",
				mcp.Required(),
				mcp.Description("Command to execute (run via sh -c)"),
			),
		),
		s.handleSandboxExec,
	)

	// sandbox_stop
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_stop",
			mcp.WithDescription("Stop a running sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to stop"),
			),
		),
		s.handleSandboxStop,
	)

	// sandbox_start
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_start",
			mcp.WithDescription("Start a stopped sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to start"),
			),
		),
		s.handleSandboxStart,
	)

	// sandbox_destroy
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_destroy",
			mcp.WithDescription("Destroy a sandbox and clean up its resources"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to destroy"),
			),
		),
		s.handleSandboxDestroy,
	)

	// sandbox_stats
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_stats",
			mcp.WithDescription("Get resource usage statistics for a running sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to get stats for"),
			),
		),
		s.handleSandboxStats,
	)

	// a2a_send
	s.mcpServer.AddTool(
		mcp.NewTool("a2a_send",
			mcp.WithDescription("Send an agent-to-agent message from one sandbox to another"),
			mcp.WithString("from",
				mcp.Required(),
				mcp.Description("Sender sandbox name"),
			),
			mcp.WithString("to",
				mcp.Required(),
				mcp.Description("Recipient sandbox name"),
			),
			mcp.WithString("type",
				mcp.Required(),
				mcp.Description("Message type (e.g., request, response, event)"),
			),
			mcp.WithString("payload",
				mcp.Required(),
				mcp.Description("Message payload (JSON string)"),
			),
		),
		s.handleA2ASend,
	)

	// a2a_receive
	s.mcpServer.AddTool(
		mcp.NewTool("a2a_receive",
			mcp.WithDescription("Receive pending agent-to-agent messages for a sandbox (clears inbox)"),
			mcp.WithString("sandbox_name",
				mcp.Required(),
				mcp.Description("Sandbox name to receive messages for"),
			),
		),
		s.handleA2AReceive,
	)

	// a2a_broadcast
	s.mcpServer.AddTool(
		mcp.NewTool("a2a_broadcast",
			mcp.WithDescription("Broadcast a message to multiple sandboxes"),
			mcp.WithString("from",
				mcp.Required(),
				mcp.Description("Sender sandbox name"),
			),
			mcp.WithString("targets",
				mcp.Required(),
				mcp.Description("Comma-separated list of target sandbox names"),
			),
			mcp.WithString("type",
				mcp.Required(),
				mcp.Description("Message type (e.g., request, response, event)"),
			),
			mcp.WithString("payload",
				mcp.Required(),
				mcp.Description("Message payload (JSON string)"),
			),
		),
		s.handleA2ABroadcast,
	)

	// matrix_shard_task
	s.mcpServer.AddTool(
		mcp.NewTool("matrix_shard_task",
			mcp.WithDescription("Distribute tasks across matrix members using a sharding strategy"),
			mcp.WithString("matrix",
				mcp.Required(),
				mcp.Description("Name of the matrix to shard tasks across"),
			),
			mcp.WithString("tasks",
				mcp.Required(),
				mcp.Description("JSON array of tasks: [{\"id\":\"t1\",\"payload\":\"...\"}]"),
			),
			mcp.WithString("strategy",
				mcp.Description("Sharding strategy: round-robin (default), hash, balanced"),
			),
		),
		s.handleMatrixShardTask,
	)

	// matrix_collect_results
	s.mcpServer.AddTool(
		mcp.NewTool("matrix_collect_results",
			mcp.WithDescription("Collect task results from matrix members via A2A messages"),
			mcp.WithString("matrix",
				mcp.Required(),
				mcp.Description("Name of the matrix to collect results from"),
			),
			mcp.WithString("task_id",
				mcp.Required(),
				mcp.Description("Task ID to collect results for"),
			),
			mcp.WithString("timeout",
				mcp.Description("Timeout in seconds (default: 60)"),
			),
		),
		s.handleMatrixCollectResults,
	)

	// sandbox_write_file
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_write_file",
			mcp.WithDescription("Write content to a file in a sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Absolute path of the file inside the sandbox (e.g. /workspace/main.py)"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("File content to write"),
			),
		),
		s.handleSandboxWriteFile,
	)

	// sandbox_read_file
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_read_file",
			mcp.WithDescription("Read content from a file in a sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Absolute path of the file inside the sandbox (e.g. /workspace/main.py)"),
			),
		),
		s.handleSandboxReadFile,
	)

	// blueprint_build
	s.mcpServer.AddTool(
		mcp.NewTool("blueprint_build",
			mcp.WithDescription("Build a Docker image from a blueprint for faster sandbox creation"),
			mcp.WithString("blueprint",
				mcp.Required(),
				mcp.Description("Path to the blueprint YAML file"),
			),
		),
		s.handleBlueprintBuild,
	)

	// sandbox_ports
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_ports",
			mcp.WithDescription("List exposed ports and their host mappings for a sandbox"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to list ports for"),
			),
		),
		s.handleSandboxPorts,
	)

	// code_interpret
	s.mcpServer.AddTool(
		mcp.NewTool("code_interpret",
			mcp.WithDescription("Execute code in a sandbox and get structured output (stdout, stderr, exit code, output files)"),
			mcp.WithString("sandbox",
				mcp.Required(),
				mcp.Description("Name of the sandbox to execute code in"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("Programming language: python, javascript, bash, go, rust"),
			),
			mcp.WithString("code",
				mcp.Required(),
				mcp.Description("Source code to execute"),
			),
			mcp.WithString("timeout",
				mcp.Description("Execution timeout in seconds (default: 30)"),
			),
		),
		s.handleCodeInterpret,
	)

	// sandbox_ready_wait
	s.mcpServer.AddTool(
		mcp.NewTool("sandbox_ready_wait",
			mcp.WithDescription("Wait for a sandbox to pass its readiness probe"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the sandbox to wait for"),
			),
			mcp.WithString("timeout",
				mcp.Description("Timeout in seconds (default: 60)"),
			),
		),
		s.handleSandboxReadyWait,
	)
}

// ServeStdio starts the MCP server on stdio (stdin/stdout). It blocks until
// stdin is closed or the process receives SIGTERM/SIGINT.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

// --- Tool handlers ---

func (s *Server) handleSandboxCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	blueprintPath, _ := args["blueprint"].(string)
	if blueprintPath == "" {
		return mcp.NewToolResultError("parameter 'blueprint' is required"), nil
	}

	workspace, _ := args["workspace"].(string)

	sb, err := s.ctrl.Create(ctx, controller.CreateOptions{
		Name:          name,
		BlueprintPath: blueprintPath,
		WorkspaceDir:  workspace,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create sandbox: %v", err)), nil
	}

	result := map[string]string{
		"name":      sb.Metadata.Name,
		"state":     string(sb.Status.State),
		"runtimeID": sb.Status.RuntimeID,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSandboxList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	sandboxes, err := s.ctrl.List()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list sandboxes: %v", err)), nil
	}

	if len(sandboxes) == 0 {
		return mcp.NewToolResultText("No sandboxes found."), nil
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%-20s %-10s %-20s %-14s %s\n", "NAME", "STATE", "BLUEPRINT", "RUNTIME ID", "AGE"))
	for _, sb := range sandboxes {
		age := time.Since(sb.Metadata.CreatedAt).Truncate(time.Second)
		runtimeID := sb.Status.RuntimeID
		if len(runtimeID) > 12 {
			runtimeID = runtimeID[:12]
		}
		buf.WriteString(fmt.Sprintf("%-20s %-10s %-20s %-14s %s\n",
			sb.Metadata.Name,
			sb.Status.State,
			sb.Spec.BlueprintRef,
			runtimeID,
			age,
		))
	}
	return mcp.NewToolResultText(buf.String()), nil
}

func (s *Server) handleSandboxExec(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	command, _ := args["command"].(string)
	if command == "" {
		return mcp.NewToolResultError("parameter 'command' is required"), nil
	}

	stdout := &mcpLimitedWriter{limit: 10 << 20}
	stderr := &mcpLimitedWriter{limit: 10 << 20}

	result, err := s.ctrl.Exec(ctx, name, &runtime.ExecConfig{
		Cmd:    []string{"sh", "-c", command},
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("exec failed: %v", err)), nil
	}

	output := stdout.buf.String()
	if errOut := stderr.buf.String(); errOut != "" {
		output += "\n[stderr]\n" + errOut
	}
	if result.ExitCode != 0 {
		output += fmt.Sprintf("\n[exit code: %d]", result.ExitCode)
	}
	return mcp.NewToolResultText(output), nil
}

func (s *Server) handleSandboxStop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	if err := s.ctrl.Stop(ctx, name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to stop sandbox: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Sandbox %q stopped.", name)), nil
}

func (s *Server) handleSandboxStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	if err := s.ctrl.Start(ctx, name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to start sandbox: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Sandbox %q started.", name)), nil
}

func (s *Server) handleSandboxDestroy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	if err := s.ctrl.Destroy(ctx, name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to destroy sandbox: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Sandbox %q destroyed.", name)), nil
}

func (s *Server) handleSandboxStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	stats, err := s.ctrl.Stats(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get sandbox stats: %v", err)), nil
	}

	memUsageMiB := float64(stats.MemoryUsage) / (1024 * 1024)
	memLimitMiB := float64(stats.MemoryLimit) / (1024 * 1024)
	var memPercent float64
	if stats.MemoryLimit > 0 {
		memPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100.0
	}

	text := fmt.Sprintf("CPU:     %.1f%%\nMemory:  %.1f MiB / %.1f MiB (%.1f%%)", stats.CPUUsage, memUsageMiB, memLimitMiB, memPercent)
	return mcp.NewToolResultText(text), nil
}

func (s *Server) handleCodeInterpret(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	sandbox, _ := args["sandbox"].(string)
	if sandbox == "" {
		return mcp.NewToolResultError("parameter 'sandbox' is required"), nil
	}
	language, _ := args["language"].(string)
	if language == "" {
		return mcp.NewToolResultError("parameter 'language' is required"), nil
	}
	code, _ := args["code"].(string)
	if code == "" {
		return mcp.NewToolResultError("parameter 'code' is required"), nil
	}

	timeoutSec := 30
	if t, ok := args["timeout"].(string); ok && t != "" {
		_, _ = fmt.Sscanf(t, "%d", &timeoutSec)
	}

	interp := interpreter.New(s.ctrl)
	result, err := interp.Execute(ctx, &interpreter.ExecuteRequest{
		Sandbox:  sandbox,
		Language: interpreter.Language(language),
		Code:     code,
		Timeout:  timeoutSec,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("code execution failed: %v", err)), nil
	}

	// Format output for the AI agent.
	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("[stderr]\n")
		output.WriteString(result.Stderr)
	}
	if result.Error != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("[error] ")
		output.WriteString(result.Error)
	}
	if result.ExitCode != 0 {
		output.WriteString(fmt.Sprintf("\n[exit code: %d]", result.ExitCode))
	}
	output.WriteString(fmt.Sprintf("\n[duration: %s]", result.Duration))

	if len(result.Files) > 0 {
		output.WriteString(fmt.Sprintf("\n[output files: %d]", len(result.Files)))
		for _, f := range result.Files {
			output.WriteString(fmt.Sprintf("\n  - %s (%d bytes)", f.Name, f.Size))
		}
	}

	return mcp.NewToolResultText(output.String()), nil
}

func (s *Server) handleSandboxPorts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	sb, err := s.ctrl.Get(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get sandbox: %v", err)), nil
	}

	if sb.Status.RuntimeID == "" {
		return mcp.NewToolResultText("No ports mapped (sandbox has no runtime ID)."), nil
	}

	info, err := s.ctrl.Runtime().Info(ctx, sb.Status.RuntimeID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to inspect container: %v", err)), nil
	}

	if len(info.Ports) == 0 {
		return mcp.NewToolResultText("No ports mapped."), nil
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%-15s %-10s %-10s\n", "CONTAINER", "HOST", "PROTO"))
	for _, p := range info.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		hostStr := "none"
		if p.HostPort > 0 {
			hostStr = fmt.Sprintf("%d", p.HostPort)
		}
		buf.WriteString(fmt.Sprintf("%-15d %-10s %-10s\n", p.ContainerPort, hostStr, proto))
	}
	return mcp.NewToolResultText(buf.String()), nil
}

// --- File tool handlers ---

func (s *Server) handleSandboxWriteFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return mcp.NewToolResultError("parameter 'path' is required"), nil
	}

	content, _ := args["content"].(string)

	if err := s.ctrl.WriteFile(ctx, name, path, strings.NewReader(content)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("File written to %s in sandbox %q.", path, name)), nil
}

func (s *Server) handleSandboxReadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return mcp.NewToolResultError("parameter 'path' is required"), nil
	}

	rc, err := s.ctrl.ReadFile(ctx, name, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}
	defer rc.Close()

	buf := &mcpLimitedWriter{limit: 10 << 20}
	if _, err := io.Copy(buf, rc); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file content: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.buf.String()), nil
}

// --- Image build handler ---

func (s *Server) handleBlueprintBuild(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	blueprintPath, _ := args["blueprint"].(string)
	if blueprintPath == "" {
		return mcp.NewToolResultError("parameter 'blueprint' is required"), nil
	}

	builder := imagepkg.NewBuilder(s.ctrl.Runtime())
	result, err := builder.Build(ctx, blueprintPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to build image: %v", err)), nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// --- A2A tool handlers ---

func (s *Server) handleA2ASend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	from, _ := args["from"].(string)
	if from == "" {
		return mcp.NewToolResultError("parameter 'from' is required"), nil
	}
	to, _ := args["to"].(string)
	if to == "" {
		return mcp.NewToolResultError("parameter 'to' is required"), nil
	}
	msgType, _ := args["type"].(string)
	if msgType == "" {
		return mcp.NewToolResultError("parameter 'type' is required"), nil
	}
	payload, _ := args["payload"].(string)

	if s.gateway == nil {
		return mcp.NewToolResultError("A2A gateway not configured"), nil
	}

	msg := &a2a.Message{
		From:    from,
		To:      to,
		Type:    msgType,
		Payload: payload,
	}
	if err := s.gateway.Send(ctx, msg); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send message: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Message sent from %q to %q (type: %s).", from, to, msgType)), nil
}

func (s *Server) handleA2AReceive(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	sandboxName, _ := args["sandbox_name"].(string)
	if sandboxName == "" {
		return mcp.NewToolResultError("parameter 'sandbox_name' is required"), nil
	}

	if s.gateway == nil {
		return mcp.NewToolResultError("A2A gateway not configured"), nil
	}

	msgs, err := s.gateway.Receive(ctx, sandboxName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to receive messages: %v", err)), nil
	}

	if len(msgs) == 0 {
		return mcp.NewToolResultText("No pending messages."), nil
	}

	data, err := json.Marshal(msgs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal messages: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleA2ABroadcast(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam: external library type with value receivers
	args := request.GetArguments()

	from, _ := args["from"].(string)
	if from == "" {
		return mcp.NewToolResultError("parameter 'from' is required"), nil
	}
	targetsStr, _ := args["targets"].(string)
	if targetsStr == "" {
		return mcp.NewToolResultError("parameter 'targets' is required"), nil
	}
	msgType, _ := args["type"].(string)
	if msgType == "" {
		return mcp.NewToolResultError("parameter 'type' is required"), nil
	}
	payload, _ := args["payload"].(string)

	if s.gateway == nil {
		return mcp.NewToolResultError("A2A gateway not configured"), nil
	}

	targets := strings.Split(targetsStr, ",")
	for i := range targets {
		targets[i] = strings.TrimSpace(targets[i])
	}

	if err := s.gateway.Broadcast(ctx, from, targets, msgType, payload); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to broadcast: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Broadcast from %q to %d targets (type: %s).", from, len(targets), msgType)), nil
}

// --- Matrix orchestration tool handlers ---

func (s *Server) handleMatrixShardTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam
	args := request.GetArguments()

	matrixName, _ := args["matrix"].(string)
	if matrixName == "" {
		return mcp.NewToolResultError("parameter 'matrix' is required"), nil
	}

	tasksJSON, _ := args["tasks"].(string)
	if tasksJSON == "" {
		return mcp.NewToolResultError("parameter 'tasks' is required"), nil
	}

	strategyName, _ := args["strategy"].(string)

	// Get matrix to find members.
	mx, err := s.ctrl.GetMatrix(matrixName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get matrix: %v", err)), nil
	}

	// Parse tasks.
	var tasks []sharding.Task
	if err := json.Unmarshal([]byte(tasksJSON), &tasks); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse tasks JSON: %v", err)), nil
	}

	// Create strategy and distribute.
	cfg := mx.Sharding
	if strategyName != "" {
		cfg = &v1alpha1.ShardingConfig{Strategy: strategyName}
	}
	strategy := sharding.NewStrategy(cfg)
	assignments := strategy.Distribute(tasks, mx.Members)

	if s.gateway == nil {
		return mcp.NewToolResultError("A2A gateway not configured"), nil
	}

	// Send each assignment via A2A.
	for _, a := range assignments {
		sandboxName := matrixName + "-" + a.MemberName
		payload, _ := json.Marshal(a.Task)
		msg := &a2a.Message{
			From:    matrixName + "-coordinator",
			To:      sandboxName,
			Type:    "task-assignment",
			Payload: string(payload),
		}
		_ = s.gateway.Send(ctx, msg)
	}

	result := map[string]interface{}{
		"totalTasks":   len(tasks),
		"totalMembers": len(mx.Members),
		"assignments":  len(assignments),
		"strategy":     strategyName,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleMatrixCollectResults(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam
	args := request.GetArguments()

	matrixName, _ := args["matrix"].(string)
	if matrixName == "" {
		return mcp.NewToolResultError("parameter 'matrix' is required"), nil
	}

	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return mcp.NewToolResultError("parameter 'task_id' is required"), nil
	}

	// Get matrix to determine expected member count.
	mx, err := s.ctrl.GetMatrix(matrixName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get matrix: %v", err)), nil
	}

	// Parse optional timeout.
	timeoutSec := 60
	if t, ok := args["timeout"].(string); ok && t != "" {
		_, _ = fmt.Sscanf(t, "%d", &timeoutSec)
	}

	// Copy config to avoid mutating the stored matrix.
	aggCfg := &v1alpha1.AggregationConfig{Strategy: "collect-all", TimeoutSec: timeoutSec}
	if mx.Aggregation != nil {
		*aggCfg = *mx.Aggregation
	}
	if aggCfg.TimeoutSec == 0 {
		aggCfg.TimeoutSec = timeoutSec
	}

	if s.gateway == nil {
		return mcp.NewToolResultError("A2A gateway not configured"), nil
	}

	collector := aggregation.NewCollector(s.gateway)
	coordinatorName := matrixName + "-coordinator"
	result, err := collector.Collect(ctx, coordinatorName, taskID, len(mx.Members), aggCfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("collection error: %v", err)), nil
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSandboxReadyWait(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // hugeParam
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	timeoutSec := 60
	if t, ok := args["timeout"].(string); ok && t != "" {
		_, _ = fmt.Sscanf(t, "%d", &timeoutSec)
	}

	// Poll sandbox state until Ready or timeout using a ticker.
	deadline := time.After(time.Duration(timeoutSec) * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return mcp.NewToolResultError("context canceled"), nil
		case <-deadline:
			return mcp.NewToolResultError(fmt.Sprintf("sandbox %q did not become ready within %ds", name, timeoutSec)), nil
		case <-ticker.C:
			sb, err := s.ctrl.Get(name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get sandbox: %v", err)), nil
			}

			if sb.Status.State == v1alpha1.SandboxStateReady {
				return mcp.NewToolResultText(fmt.Sprintf("Sandbox %q is ready.", name)), nil
			}
			if sb.Status.State == v1alpha1.SandboxStateError {
				msg := sb.Status.Message
				if sb.Status.ProbeError != "" {
					msg = sb.Status.ProbeError
				}
				return mcp.NewToolResultError(fmt.Sprintf("sandbox %q is in error state: %s", name, msg)), nil
			}
		}
	}
}
