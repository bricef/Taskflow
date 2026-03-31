// taskflow-mcp is an MCP server that exposes TaskFlow operations as tools
// and resources over stdio transport. One instance per AI agent, each
// configured with that agent's API key.
//
// Configuration via environment variables:
//
//	TASKFLOW_URL      — server URL (default: http://localhost:8374)
//	TASKFLOW_API_KEY  — bearer token for authentication (required)
package main

import (
	"context"
	"fmt"
	"os"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bricef/taskflow/internal/httpclient"
	tfmcp "github.com/bricef/taskflow/internal/mcp"
)

func main() {
	serverURL := envOr("TASKFLOW_URL", "http://localhost:8374")
	apiKey := os.Getenv("TASKFLOW_API_KEY")

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "TASKFLOW_API_KEY is required")
		os.Exit(1)
	}

	client := httpclient.New(serverURL, apiKey)
	server := tfmcp.NewServer(client)

	if err := server.Run(context.Background(), &gomcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
