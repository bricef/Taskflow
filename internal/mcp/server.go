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
	"github.com/bricef/taskflow/internal/version"
)

// NewServer creates an MCP server with all TaskFlow resources and tools registered.
// If notifier is non-nil, tool responses include pending notifications from other actors.
func NewServer(client *httpclient.Client, notifier *Notifier) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "taskflow",
		Version: version.Version,
	}, nil)

	registerResources(server, client)
	registerTools(server, client, notifier)
	registerWhoAmI(server, client, notifier)

	return server
}

// descriptionOf returns the longer description if set, otherwise the summary.
func descriptionOf(summary, description string) string {
	if description != "" {
		return description
	}
	return summary
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
			Description: descriptionOf(res.Summary, res.Description),
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

func registerTools(server *gomcp.Server, client *httpclient.Client, notifier *Notifier) {
	for _, op := range model.Operations() {
		op := op
		server.AddTool(
			&gomcp.Tool{
				Name:        op.Name,
				Description: descriptionOf(op.Summary, op.Description),
				InputSchema: buildInputSchema(op),
			},
			withNotifications(notifier, toolHandler(client, op)),
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

		// Support task_ref shorthand: "platform/3" → slug=platform, num=3
		if ref, ok := input["task_ref"].(string); ok {
			if slash := strings.LastIndex(ref, "/"); slash >= 0 {
				input["slug"] = ref[:slash]
				input["num"] = ref[slash+1:]
			}
			delete(input, "task_ref")
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

// --- WhoAmI ---

func registerWhoAmI(server *gomcp.Server, client *httpclient.Client, notifier *Notifier) {
	server.AddTool(
		&gomcp.Tool{
			Name:        "whoami",
			Description: "Returns your actor identity (name, role, type) as determined by your API key.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		withNotifications(notifier, func(ctx context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			actor, err := client.WithContext(ctx).WhoAmI()
			if err != nil {
				return &gomcp.CallToolResult{
					Content: []gomcp.Content{&gomcp.TextContent{Text: err.Error()}},
					IsError: true,
				}, nil
			}
			data, _ := json.MarshalIndent(actor, "", "  ")
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
			}, nil
		}),
	)
}

// --- Notification piggyback ---

// withNotifications wraps a tool handler to append pending notifications
// to the response. If notifier is nil, the handler is returned unchanged.
func withNotifications(notifier *Notifier, handler gomcp.ToolHandler) gomcp.ToolHandler {
	if notifier == nil {
		return handler
	}
	return func(ctx context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		result, err := handler(ctx, req)
		if err != nil || result == nil {
			return result, err
		}

		notifications := notifier.Drain()
		if len(notifications) == 0 {
			return result, nil
		}

		// Append notifications as a separate text block.
		var summary strings.Builder
		summary.WriteString(fmt.Sprintf("\n--- %d notification(s) from other actors ---\n", len(notifications)))
		for _, n := range notifications {
			summary.WriteString(fmt.Sprintf("  [%s] %s\n", n.Timestamp, n.Summary))
		}

		result.Content = append(result.Content, &gomcp.TextContent{Text: summary.String()})
		return result, nil
	}
}

// --- Schema generation ---

func buildInputSchema(op model.Operation) map[string]any {
	properties := map[string]any{}
	required := []string{}

	pps := op.PathParams()
	hasSlugAndNum := false
	for _, pp := range pps {
		properties[pp.Name] = map[string]any{
			"type":        pp.Type,
			"description": pp.Name + " path parameter",
		}
		required = append(required, pp.Name)
		if pp.Name == "num" {
			hasSlugAndNum = true
		}
	}
	// Add task_ref as a convenience alternative to slug+num.
	if hasSlugAndNum {
		properties["task_ref"] = map[string]any{
			"type":        "string",
			"description": "Task reference as board/num (e.g. 'platform/3'). Alternative to providing slug and num separately.",
		}
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
