package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/peterwwillis/zop/internal/provider"
)

// Authorizer is an interface for authorizing tool calls.
type Authorizer interface {
	IsAllowed(toolName string, args string) bool
}

// Client is a wrapper around an MCP client.
type Client struct {
	mcpClient  *client.Client
	serverInfo string
}

// NewClient creates a new MCP client that connects via stdio or SSE.
func NewClient(ctx context.Context, url string, command string, args ...string) (*Client, error) {
	var c *client.Client
	var err error
	var info string

	if url != "" {
		c, err = client.NewSSEMCPClient(url)
		if err != nil {
			return nil, fmt.Errorf("creating sse mcp client: %w", err)
		}
		info = url
	} else if command != "" {
		c, err = client.NewStdioMCPClient(command, os.Environ(), args...)
		if err != nil {
			return nil, fmt.Errorf("creating stdio mcp client: %w", err)
		}
		info = fmt.Sprintf("%s %v", command, args)
	} else {
		return nil, fmt.Errorf("either url or command must be provided for MCP client")
	}

	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting mcp client: %w", err)
	}

	// Initialize the client
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "zop",
				Version: "0.1.0",
			},
		},
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		return nil, fmt.Errorf("initializing mcp client: %w", err)
	}

	return &Client{
		mcpClient:  c,
		serverInfo: info,
	}, nil
}

// ListTools returns the list of tools available on the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]provider.Tool, error) {
	resp, err := c.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}

	tools := make([]provider.Tool, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		tools = append(tools, provider.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return tools, nil
}

// ExecuteTool calls a tool on the MCP server.
func (c *Client) ExecuteTool(ctx context.Context, name string, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("unmarshaling arguments: %w", err)
	}

	resp, err := c.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: params,
		},
	})
	if err != nil {
		return "", fmt.Errorf("calling tool: %w", err)
	}

	// Aggregate text content
	var output string
	for _, content := range resp.Content {
		if text, ok := content.(mcp.TextContent); ok {
			output += text.Text
		}
	}
	if resp.IsError {
		return output, fmt.Errorf("tool execution failed: %s", output)
	}
	return output, nil
}

// Close closes the connection to the MCP server.
func (c *Client) Close() error {
	return c.mcpClient.Close()
}

// ToolWrapper wraps an MCP tool into a tool.Definition.
type ToolWrapper struct {
	client     *Client
	name       string
	desc       string
	params     interface{}
	authorizer Authorizer
}

func (w *ToolWrapper) Name() string { return w.name }
func (w *ToolWrapper) Description() string { return w.desc }
func (w *ToolWrapper) Parameters() interface{} { return w.params }
func (w *ToolWrapper) Execute(ctx context.Context, args string) (string, error) {
	if w.authorizer != nil {
		if !w.authorizer.IsAllowed(w.name, args) {
			return "", fmt.Errorf("tool call %q is denied by tool policy", w.name)
		}
	}
	return w.client.ExecuteTool(ctx, w.name, args)
}

// WrapTools wraps all tools from an MCP client into tool definitions.
func WrapTools(ctx context.Context, c *Client, auth Authorizer) ([]*ToolWrapper, error) {
	resp, err := c.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	wrappers := make([]*ToolWrapper, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		wrappers = append(wrappers, &ToolWrapper{
			client:     c,
			name:       t.Name,
			desc:       t.Description,
			params:     t.InputSchema,
			authorizer: auth,
		})
	}
	return wrappers, nil
}
