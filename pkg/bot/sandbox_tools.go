package bot

import (
	"context"
	"fmt"
	"log"
	"marinai/pkg/sandbox"
	"marinai/pkg/tools"
	"strings"
	"time"
)

var sandboxClient *sandbox.Client

func InitSandbox() error {
	var err error
	sandboxClient, err = sandbox.NewClient()
	if err != nil {
		return fmt.Errorf("failed to init sandbox client: %w", err)
	}
	log.Println("Sandbox client initialized")
	return nil
}

type BashTool struct{}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Execute a bash command in an isolated sandbox. The sandbox is ephemeral and resets periodically. Use for file operations, system commands, running scripts. Max 30 second timeout."
}

func (t *BashTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"command": {
				Type:        "string",
				Description: "The bash command to execute",
			},
			"timeout": {
				Type:        "integer",
				Description: "Timeout in seconds (max 30)",
			},
		},
		Required: []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if sandboxClient == nil {
		return tools.Result{Success: false, Error: "sandbox not initialized"}, nil
	}

	command, _ := params["command"].(string)
	timeout, _ := params["timeout"].(float64)
	timeoutInt := int(timeout)

	execCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	resp, err := sandboxClient.Exec(execCtx, command, timeoutInt)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	result := resp.Output
	if resp.TimedOut {
		result += "\n[Command timed out]"
	}
	if resp.ExitCode != 0 {
		result += fmt.Sprintf("\n[Exit code: %d]", resp.ExitCode)
	}
	if resp.Error != "" {
		result += fmt.Sprintf("\n[Error: %s]", resp.Error)
	}

	if result == "" {
		result = "[No output]"
	}

	return tools.Result{Success: true, Data: result}, nil
}

type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read a file from the sandbox filesystem. Returns the file contents."
}

func (t *ReadFileTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"path": {
				Type:        "string",
				Description: "Path to the file (relative to workspace)",
			},
		},
		Required: []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if sandboxClient == nil {
		return tools.Result{Success: false, Error: "sandbox not initialized"}, nil
	}

	path, _ := params["path"].(string)

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	content, err := sandboxClient.Read(execCtx, path)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var result string
	if len(content) > 5000 {
		result = string(content[:5000]) + "\n...[truncated]"
	} else {
		result = string(content)
	}

	return tools.Result{Success: true, Data: result}, nil
}

type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file in the sandbox filesystem. Creates the file if it doesn't exist. Max 1MB."
}

func (t *WriteFileTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"path": {
				Type:        "string",
				Description: "Path to the file (relative to workspace)",
			},
			"content": {
				Type:        "string",
				Description: "Content to write to the file",
			},
		},
		Required: []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if sandboxClient == nil {
		return tools.Result{Success: false, Error: "sandbox not initialized"}, nil
	}

	path, _ := params["path"].(string)
	content, _ := params["content"].(string)

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := sandboxClient.Write(execCtx, path, content); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	result := fmt.Sprintf("Written %d bytes to %s", len(content), path)
	return tools.Result{Success: true, Data: result}, nil
}

type ListDirTool struct{}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List files and directories in the sandbox filesystem."
}

func (t *ListDirTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"path": {
				Type:        "string",
				Description: "Directory path to list (default: root)",
			},
		},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if sandboxClient == nil {
		return tools.Result{Success: false, Error: "sandbox not initialized"}, nil
	}

	path, _ := params["path"].(string)
	if path == "" {
		path = "/"
	}

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	files, err := sandboxClient.List(execCtx, path)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	if len(files) == 0 {
		return tools.Result{Success: true, Data: "[Empty directory]"}, nil
	}

	var lines []string
	for _, f := range files {
		prefix := "📄 "
		if f.IsDir {
			prefix = "📁 "
		}
		lines = append(lines, fmt.Sprintf("%s%s (%d bytes)", prefix, f.Name, f.Size))
	}

	return tools.Result{Success: true, Data: strings.Join(lines, "\n")}, nil
}

func RegisterSandboxTools(registry *tools.Registry) {
	if sandboxClient == nil {
		return
	}
	_ = registry.Register(&BashTool{})
	_ = registry.Register(&ReadFileTool{})
	_ = registry.Register(&WriteFileTool{})
	_ = registry.Register(&ListDirTool{})
	log.Println("Sandbox tools registered: bash, read_file, write_file, list_dir")
}
