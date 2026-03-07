# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Phase 1 — MVP
- CLI framework with Cobra (`smx` root command)
- `smx version` command with JSON output support
- Configuration management with Viper
- Core API types (Sandbox, Blueprint, Workspace, Session, Matrix)
- Blueprint YAML parser and validator
- `smx blueprint validate/inspect` commands
- Built-in blueprints (python-dev, go-dev, node-dev, rust-dev)
- Runtime interface with pluggable plugin registry
- Docker runtime backend (create/start/stop/destroy/exec/list/info)
- File-based JSON state store with atomic writes
- Docker state reconciler (recovers sandboxes across CLI restarts)
- Sandbox lifecycle: `smx sandbox create/start/stop/destroy/list/exec/inspect`
- Workspace directory mounting support
- GitHub Actions CI/release/security workflows
- E2E test suite

#### Phase 2 — Core Platform
- Snapshot/restore via Docker commit (`smx sandbox snapshot/restore/snapshots`)
- MCP server with stdio transport (`smx mcp serve`)
- MCP tools: sandbox_create, sandbox_list, sandbox_exec, sandbox_stop, sandbox_start, sandbox_destroy, sandbox_stats
- Session management (`smx session start/end/list/exec`)
- Matrix multi-sandbox orchestration (`smx matrix create/list/inspect/stop/start/destroy`)
- Resource monitoring via Docker stats (`smx sandbox stats`)
- BoltDB persistent state store (alternative to JSON files)
- Matrix store with file-based persistence

#### Phase 3 — Advanced Platform
- Network policies (none/host/bridge/isolate) in blueprint spec
- Matrix sandboxes share isolated internal Docker network
- CreateNetwork/DeleteNetwork on Runtime interface
- gVisor runtime backend (Docker with --runtime=runsc)
- Firecracker runtime stub (Linux+KVM required)
- Configurable OCI runtime via Docker functional options
- A2A (Agent-to-Agent) messaging gateway
- A2A MCP tools: a2a_send, a2a_receive, a2a_broadcast
- A2A CLI commands for debugging (`smx a2a send/receive/broadcast`)
