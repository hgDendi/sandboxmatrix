// Package interpreter provides a Code Interpreter that executes code in
// sandboxes and returns structured results, similar to E2B's code interpreter.
//
// It supports multiple languages (Python, JavaScript, Bash, Go, Rust) and
// captures stdout, stderr, exit code, execution duration, and output files.
package interpreter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// Language defines a supported programming language.
type Language string

const (
	// LangPython executes code with python3.
	LangPython Language = "python"
	// LangJavaScript executes code with node.
	LangJavaScript Language = "javascript"
	// LangBash executes code with bash.
	LangBash Language = "bash"
	// LangGo compiles and runs Go code.
	LangGo Language = "go"
	// LangRust compiles and runs Rust code.
	LangRust Language = "rust"
)

// DefaultTimeout is the default execution timeout in seconds.
const DefaultTimeout = 30

// maxExecOutput is the maximum captured output size (10 MB).
const maxExecOutput = 10 << 20

// maxFileContentSize is the maximum file content to inline in base64 (256 KB).
const maxFileContentSize = 256 << 10

// outputDir is the directory where code execution output files are collected.
const outputDir = "/tmp/smx_output"

// ExecuteRequest holds parameters for code execution.
type ExecuteRequest struct {
	Sandbox  string   `json:"sandbox"`
	Language Language `json:"language"`
	Code     string   `json:"code"`
	Timeout  int      `json:"timeout,omitempty"` // seconds, default 30
}

// ExecuteResult holds the result of code execution.
type ExecuteResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exitCode"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
	Files    []OutputFile  `json:"files,omitempty"`
}

// OutputFile represents a file generated during execution.
type OutputFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content string `json:"content,omitempty"` // base64 encoded for small files
}

// langConfig holds per-language execution parameters.
type langConfig struct {
	extension string
	// buildCmd returns the compile command (empty if interpreted).
	buildCmd func(src, bin string) []string
	// runCmd returns the execution command.
	runCmd func(src, bin string) []string
}

var languages = map[Language]langConfig{
	LangPython: {
		extension: ".py",
		runCmd: func(src, _ string) []string {
			return []string{"python3", src}
		},
	},
	LangJavaScript: {
		extension: ".js",
		runCmd: func(src, _ string) []string {
			return []string{"node", src}
		},
	},
	LangBash: {
		extension: ".sh",
		runCmd: func(src, _ string) []string {
			return []string{"bash", src}
		},
	},
	LangGo: {
		extension: ".go",
		runCmd: func(src, _ string) []string {
			dir := filepath.Dir(src)
			base := filepath.Base(src)
			return []string{"sh", "-c", fmt.Sprintf("cd %s && go run %s", dir, base)}
		},
	},
	LangRust: {
		extension: ".rs",
		buildCmd: func(src, bin string) []string {
			return []string{"rustc", src, "-o", bin}
		},
		runCmd: func(_, bin string) []string {
			return []string{bin}
		},
	},
}

// Interpreter executes code in sandboxes.
type Interpreter struct {
	ctrl *controller.Controller
}

// New creates a new Interpreter backed by the given controller.
func New(ctrl *controller.Controller) *Interpreter {
	return &Interpreter{ctrl: ctrl}
}

// Execute runs code in the specified language inside the sandbox.
//
// The method:
//  1. Writes the code to a temporary file in the sandbox.
//  2. Optionally compiles it (for Go/Rust).
//  3. Executes the code with a timeout.
//  4. Scans for output files in /tmp/smx_output.
//  5. Returns a structured result with stdout, stderr, exit code, and output files.
func (interp *Interpreter) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	start := time.Now()

	if req.Sandbox == "" {
		return nil, fmt.Errorf("sandbox name is required")
	}
	if req.Code == "" {
		return nil, fmt.Errorf("code is required")
	}

	lang, ok := languages[req.Language]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %q", req.Language)
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	// Derive context with timeout.
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// File paths inside the sandbox.
	srcFile := "/tmp/smx_code" + lang.extension
	binFile := "/tmp/smx_code_bin"

	// Step 1: Prepare the output directory and write code to the sandbox.
	if err := interp.writeCode(execCtx, req.Sandbox, srcFile, req.Code); err != nil {
		return &ExecuteResult{
			ExitCode: -1,
			Duration: time.Since(start),
			Error:    fmt.Sprintf("failed to write code: %v", err),
		}, nil
	}

	// Step 2: Compile if needed.
	if lang.buildCmd != nil {
		buildResult, err := interp.execInSandbox(execCtx, req.Sandbox, lang.buildCmd(srcFile, binFile))
		if err != nil {
			return &ExecuteResult{
				ExitCode: -1,
				Duration: time.Since(start),
				Error:    fmt.Sprintf("compilation failed: %v", err),
			}, nil
		}
		if buildResult.ExitCode != 0 {
			return &ExecuteResult{
				Stdout:   buildResult.Stdout,
				Stderr:   buildResult.Stderr,
				ExitCode: buildResult.ExitCode,
				Duration: time.Since(start),
				Error:    "compilation failed",
			}, nil
		}
	}

	// Step 3: Execute.
	runResult, err := interp.execInSandbox(execCtx, req.Sandbox, lang.runCmd(srcFile, binFile))
	if err != nil {
		return &ExecuteResult{
			ExitCode: -1,
			Duration: time.Since(start),
			Error:    fmt.Sprintf("execution failed: %v", err),
		}, nil
	}

	result := &ExecuteResult{
		Stdout:   runResult.Stdout,
		Stderr:   runResult.Stderr,
		ExitCode: runResult.ExitCode,
		Duration: time.Since(start),
	}

	// Step 4: Collect output files.
	files, err := interp.collectOutputFiles(ctx, req.Sandbox)
	if err == nil {
		result.Files = files
	}

	return result, nil
}

// writeCode writes source code to a file in the sandbox. It uses a here-document
// approach via base64 to avoid shell escaping issues with arbitrary code.
func (interp *Interpreter) writeCode(ctx context.Context, sandbox, path, code string) error {
	// Encode the code as base64 to avoid any shell escaping issues.
	encoded := base64.StdEncoding.EncodeToString([]byte(code))

	// Create output directory and write the code file.
	cmd := []string{
		"sh", "-c",
		fmt.Sprintf("mkdir -p %s && echo '%s' | base64 -d > %s",
			outputDir, encoded, path),
	}

	result, err := interp.execInSandbox(ctx, sandbox, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("write failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return nil
}

// execResult holds the captured output of a command execution.
type execResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// execInSandbox executes a command in the sandbox and captures output.
func (interp *Interpreter) execInSandbox(ctx context.Context, sandbox string, cmd []string) (*execResult, error) {
	stdout := &limitedWriter{limit: maxExecOutput}
	stderr := &limitedWriter{limit: maxExecOutput}

	result, err := interp.ctrl.Exec(ctx, sandbox, &runtime.ExecConfig{
		Cmd:    cmd,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return nil, err
	}

	return &execResult{
		ExitCode: result.ExitCode,
		Stdout:   stdout.buf.String(),
		Stderr:   stderr.buf.String(),
	}, nil
}

// collectOutputFiles scans the output directory for files generated during
// execution and returns them, inlining small files as base64.
func (interp *Interpreter) collectOutputFiles(ctx context.Context, sandbox string) ([]OutputFile, error) {
	// List files in output directory using JSON for reliable parsing.
	cmd := []string{
		"sh", "-c",
		fmt.Sprintf(`python3 -c "
import os, json
d = '%s'
if not os.path.isdir(d):
    print('[]')
else:
    files = []
    for name in os.listdir(d):
        full = os.path.join(d, name)
        if os.path.isfile(full):
            files.append({'name': name, 'path': full, 'size': os.path.getsize(full)})
    print(json.dumps(files))
" 2>/dev/null || echo '[]'`, outputDir),
	}

	result, err := interp.execInSandbox(ctx, sandbox, cmd)
	if err != nil {
		return nil, err
	}

	stdout := strings.TrimSpace(result.Stdout)
	if stdout == "" || stdout == "[]" {
		return nil, nil
	}

	var rawFiles []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(stdout), &rawFiles); err != nil {
		return nil, fmt.Errorf("parse output files: %w", err)
	}

	var files []OutputFile
	for _, rf := range rawFiles {
		of := OutputFile{
			Name: rf.Name,
			Path: rf.Path,
			Size: rf.Size,
		}

		// Inline small files as base64.
		if rf.Size > 0 && rf.Size <= maxFileContentSize {
			content, err := interp.readFileBase64(ctx, sandbox, rf.Path)
			if err == nil {
				of.Content = content
			}
		}

		files = append(files, of)
	}

	return files, nil
}

// readFileBase64 reads a file from the sandbox and returns its content as base64.
func (interp *Interpreter) readFileBase64(ctx context.Context, sandbox, path string) (string, error) {
	result, err := interp.execInSandbox(ctx, sandbox, []string{
		"base64", path,
	})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("base64 read failed: %s", result.Stderr)
	}
	return strings.TrimSpace(result.Stdout), nil
}

// limitedWriter wraps a bytes.Buffer with a maximum size to prevent
// unbounded memory growth from large command outputs.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // discard silently
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}
