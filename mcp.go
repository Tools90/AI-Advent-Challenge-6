package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPClient struct {
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

func NewMCPClient(ctx context.Context, serverName, nodePath, npxCLI, allowedDir string) (*MCPClient, error) {
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		return nil, fmt.Errorf("создать каталог MCP: %w", err)
	}
	node := nodePath
	if node == "" {
		node = findNode()
	}
	cli := npxCLI
	if cli == "" {
		cli = findNpxCLI()
	}
	cmd := shellCommand(node, cli, "-y", "@modelcontextprotocol/server-filesystem", allowedDir)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(node)+string(os.PathListSeparator)+os.Getenv("PATH"))
	client := mcp.NewClient(&mcp.Implementation{Name: "day16-agent", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("MCP connect: %w", err)
	}
	toolsResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("MCP tools/list: %w", err)
	}
	return &MCPClient{session: session, tools: toolsResult.Tools}, nil
}

func (m *MCPClient) Close() error {
	if m == nil || m.session == nil {
		return nil
	}
	return m.session.Close()
}

func (m *MCPClient) Tools() []*mcp.Tool {
	return append([]*mcp.Tool{}, m.tools...)
}

func (m *MCPClient) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("MCP tools/call %s: %w", name, err)
	}
	if result.StructuredContent != nil {
		return result.StructuredContent, nil
	}
	texts := []string{}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return map[string]any{"content": texts, "isError": result.IsError}, nil
}

func addMCPTools(tools map[string]Tool, client *MCPClient, serverName string) {
	if client == nil {
		return
	}
	for _, mcpTool := range client.Tools() {
		name := "mcp__" + serverName + "__" + mcpTool.Name
		schema := schemaMap(mcpTool.InputSchema)
		toolName := mcpTool.Name
		tools[name] = Tool{
			Schema: ToolSchema{
				Type: "function",
				Function: ToolFunction{
					Name:        name,
					Description: "[MCP " + serverName + "] " + mcpTool.Description,
					Parameters:  schema,
				},
			},
			Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				args := map[string]any{}
				if len(raw) > 0 {
					if err := json.Unmarshal(raw, &args); err != nil {
						return nil, fmt.Errorf("MCP arguments: %w", err)
					}
				}
				return client.Call(ctx, toolName, args)
			},
		}
	}
}

func schemaMap(value any) map[string]any {
	if schema, ok := value.(map[string]any); ok {
		return schema
	}
	data, err := json.Marshal(value)
	if err == nil {
		var schema map[string]any
		if json.Unmarshal(data, &schema) == nil && schema != nil {
			return schema
		}
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func shellCommand(command string, args ...string) *exec.Cmd {
	return exec.Command(command, args...)
}

func findNode() string {
	if path := os.Getenv("MCP_NODE"); path != "" {
		return path
	}
	if path, err := exec.LookPath("node.exe"); err == nil {
		return path
	}
	candidate := `C:\Program Files\nodejs\node.exe`
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "node.exe"
}

func findNpxCLI() string {
	if path := os.Getenv("MCP_NPX_CLI"); path != "" {
		return path
	}
	candidates := []string{
		`C:\Program Files\nodejs\node_modules\npm\bin\npx-cli.js`,
		`C:\Program Files\nodejs\node_modules\npm\bin\npx-cli.js`,
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "npx-cli.js"
}

func ensureMCPDemo(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "numbers.txt")
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte("12 35 7\n"), 0o644)
}

func mcpStatus(client *MCPClient, serverName string, err error) map[string]any {
	status := "connected"
	tools := []string{}
	if err != nil {
		status = "error"
	} else if client != nil {
		for _, tool := range client.Tools() {
			tools = append(tools, tool.Name)
		}
	}
	return map[string]any{
		"status":     status,
		"server":     serverName,
		"tools":      tools,
		"error":      errorString(err),
		"checked_at": time.Now().Format(time.RFC3339),
	}
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
