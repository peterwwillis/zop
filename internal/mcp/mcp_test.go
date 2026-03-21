package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient_Invalid(t *testing.T) {
	ctx := context.Background()
	_, err := NewClient(ctx, "", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either url or command must be provided")
}

func TestNewClient_Stdio(t *testing.T) {
	ctx := context.Background()
	// Using 'echo' as a dummy MCP server. It won't actually respond to 
	// Initialize, so it might fail there, but we can check if it starts.
	// Actually, Initialize will fail because echo doesn't speak MCP.
	c, err := NewClient(ctx, "", "echo", "hello")
	if err == nil {
		defer c.Close()
	}
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initializing mcp client")
}
