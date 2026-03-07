// Package mcp implements an MCP (Model Context Protocol) server that exposes
// sandbox operations as tools, enabling AI agents to create and manage sandboxes.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
}

// ServeStdio starts the MCP server on stdio (stdin/stdout). It blocks until
// stdin is closed or the process receives SIGTERM/SIGINT.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

// --- Tool handlers ---

func (s *Server) handleSandboxCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func (s *Server) handleSandboxList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func (s *Server) handleSandboxExec(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("parameter 'name' is required"), nil
	}

	command, _ := args["command"].(string)
	if command == "" {
		return mcp.NewToolResultError("parameter 'command' is required"), nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result, err := s.ctrl.Exec(ctx, name, runtime.ExecConfig{
		Cmd:    []string{"sh", "-c", command},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("exec failed: %v", err)), nil
	}

	output := stdout.String()
	if errOut := stderr.String(); errOut != "" {
		output += "\n[stderr]\n" + errOut
	}
	if result.ExitCode != 0 {
		output += fmt.Sprintf("\n[exit code: %d]", result.ExitCode)
	}
	return mcp.NewToolResultText(output), nil
}

func (s *Server) handleSandboxStop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func (s *Server) handleSandboxStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func (s *Server) handleSandboxDestroy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func (s *Server) handleSandboxStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

// --- A2A tool handlers ---

func (s *Server) handleA2ASend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	msg := a2a.Message{
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

func (s *Server) handleA2AReceive(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	sandboxName, _ := args["sandbox_name"].(string)
	if sandboxName == "" {
		return mcp.NewToolResultError("parameter 'sandbox_name' is required"), nil
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

func (s *Server) handleA2ABroadcast(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	targets := strings.Split(targetsStr, ",")
	for i := range targets {
		targets[i] = strings.TrimSpace(targets[i])
	}

	if err := s.gateway.Broadcast(ctx, from, targets, msgType, payload); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to broadcast: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Broadcast from %q to %d targets (type: %s).", from, len(targets), msgType)), nil
}
