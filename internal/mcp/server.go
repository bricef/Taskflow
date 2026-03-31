// Package mcp provides an MCP server that exposes TaskFlow resources and
// operations as MCP resources and tools. Both are auto-derived from
// model.Resources() and model.Operations().
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

// NewServer creates an MCP server with all TaskFlow resources and tools registered.
func NewServer(client *httpclient.Client) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "taskflow",
		Version: "1.0.0",
	}, nil)

	registerResources(server, client)
	registerTools(server, client)

	return server
}

// --- Resources ---

func registerResources(server *gomcp.Server, client *httpclient.Client) {
	for _, res := range model.Resources() {
		res := res
		// Convert path template to RFC 6570 URI template.
		// /boards/{slug}/tasks → taskflow://boards/{slug}/tasks
		uri := "taskflow://" + strings.TrimPrefix(res.Path, "/")

		server.AddResourceTemplate(&gomcp.ResourceTemplate{
			URITemplate: uri,
			Name:        res.Name,
			Description: res.Summary,
			MIMEType:    "application/json",
		}, resourceHandler(client, res))
	}
}

func resourceHandler(client *httpclient.Client, res model.Resource) gomcp.ResourceHandler {
	return func(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
		params := extractParams(res.Path, req.Params.URI)

		raw, err := httpclient.GetOne[json.RawMessage](client.WithContext(ctx), res, params, nil)
		if err != nil {
			return nil, err
		}

		formatted, _ := json.MarshalIndent(json.RawMessage(raw), "", "  ")
		return &gomcp.ReadResourceResult{
			Contents: []*gomcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(formatted),
			}},
		}, nil
	}
}

// --- Tools ---

func registerTools(server *gomcp.Server, client *httpclient.Client) {
	for _, op := range model.Operations() {
		op := op
		server.AddTool(
			&gomcp.Tool{
				Name:        op.Name,
				Description: op.Summary,
				InputSchema: buildInputSchema(op),
			},
			toolHandler(client, op),
		)
	}
}

func toolHandler(client *httpclient.Client, op model.Operation) gomcp.ToolHandler {
	pathParams := op.PathParams()

	return func(ctx context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		// Unmarshal the raw input.
		var input map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			input = map[string]any{}
		}

		// Separate path params from body params.
		params := httpclient.PathParams{}
		for _, pp := range pathParams {
			if v, ok := input[pp.Name]; ok {
				params[pp.Name] = fmt.Sprint(v)
				delete(input, pp.Name)
			}
		}

		var body any
		if len(input) > 0 {
			body = input
		}

		raw, err := httpclient.Exec[json.RawMessage](client.WithContext(ctx), op, params, body)
		if err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					&gomcp.TextContent{Text: err.Error()},
				},
				IsError: true,
			}, nil
		}

		if len(raw) == 0 {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					&gomcp.TextContent{Text: "OK"},
				},
			}, nil
		}

		formatted, _ := json.MarshalIndent(json.RawMessage(raw), "", "  ")
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				&gomcp.TextContent{Text: string(formatted)},
			},
		}, nil
	}
}

// --- Schema generation ---

func buildInputSchema(op model.Operation) map[string]any {
	properties := map[string]any{}
	required := []string{}

	for _, pp := range op.PathParams() {
		properties[pp.Name] = map[string]any{
			"type":        pp.Type,
			"description": pp.Name + " path parameter",
		}
		required = append(required, pp.Name)
	}

	if op.Input != nil {
		t := reflect.TypeOf(op.Input)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() == reflect.Struct {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				tag := f.Tag.Get("json")
				if tag == "" || tag == "-" {
					continue
				}
				name := strings.Split(tag, ",")[0]
				omitempty := strings.Contains(tag, "omitempty")

				// Skip Optional[T] from required.
				ft := f.Type
				isOptional := ft.Kind() == reflect.Struct && ft.NumField() == 2 &&
					ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set"

				properties[name] = map[string]any{
					"type": jsonSchemaType(ft),
				}

				if !omitempty && !isOptional && ft.Kind() != reflect.Ptr {
					required = append(required, name)
				}
			}
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func jsonSchemaType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && t.NumField() == 2 &&
		t.Field(0).Name == "Value" && t.Field(1).Name == "Set" {
		return jsonSchemaType(t.Field(0).Type)
	}
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Slice:
		return "array"
	default:
		return "string"
	}
}

// --- URI param extraction ---

func extractParams(pathTemplate, uri string) httpclient.PathParams {
	uriPath := uri
	if idx := strings.Index(uri, "://"); idx >= 0 {
		uriPath = "/" + uri[idx+3:]
	}

	templateParts := strings.Split(strings.Trim(pathTemplate, "/"), "/")
	uriParts := strings.Split(strings.Trim(uriPath, "/"), "/")

	params := httpclient.PathParams{}
	for i, tp := range templateParts {
		if i >= len(uriParts) {
			break
		}
		if strings.HasPrefix(tp, "{") && strings.HasSuffix(tp, "}") {
			params[tp[1:len(tp)-1]] = uriParts[i]
		}
	}
	return params
}
