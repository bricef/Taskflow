package http

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// generateOpenAPISpec generates an OpenAPI 3.1 JSON document from the route list.
func generateOpenAPISpec(routes []Route) []byte {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "TaskFlow API",
			"version":     "1.0.0",
			"description": "A human/AI collaborative task tracker with kanban boards and workflow state machines.",
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
			"schemas": map[string]any{
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error":   map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
						"detail":  map[string]any{},
					},
				},
			},
		},
	}

	paths := map[string]any{}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)

	for _, rt := range routes {
		op := map[string]any{
			"summary":  rt.Summary,
			"security": []any{map[string]any{"bearerAuth": []any{}}},
			"tags":     []string{tagFromPath(rt.Path)},
		}

		// Path parameters (inferred from path).
		var params []any
		for _, p := range rt.PathParams() {
			params = append(params, map[string]any{
				"name": p.Name, "in": "path", "required": true,
				"schema": map[string]any{"type": p.Type},
			})
		}
		// Query parameters.
		for _, p := range rt.Params {
			param := map[string]any{
				"name": p.Name, "in": "query",
				"schema": map[string]any{"type": p.Type},
			}
			if p.Desc != "" {
				param["description"] = p.Desc
			}
			params = append(params, param)
		}
		if len(params) > 0 {
			op["parameters"] = params
		}

		// Request body.
		if rt.Input != nil {
			schemaName := typeName(rt.Input)
			schemas[schemaName] = typeToSchema(reflect.TypeOf(rt.Input))
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/" + schemaName},
					},
				},
			}
		}

		// Responses.
		responses := map[string]any{}
		statusStr := fmt.Sprintf("%d", statusForAction(rt.Action))
		if rt.Output != nil {
			outType := reflect.TypeOf(rt.Output)
			var responseSchema map[string]any
			if outType.Kind() == reflect.Slice {
				elemName := typeName(reflect.New(outType.Elem()).Elem().Interface())
				schemas[elemName] = typeToSchema(outType.Elem())
				responseSchema = map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/" + elemName},
				}
			} else {
				schemaName := typeName(rt.Output)
				schemas[schemaName] = typeToSchema(outType)
				responseSchema = map[string]any{"$ref": "#/components/schemas/" + schemaName}
			}
			responses[statusStr] = map[string]any{
				"description": "Success",
				"content": map[string]any{
					"application/json": map[string]any{"schema": responseSchema},
				},
			}
		} else {
			responses[statusStr] = map[string]any{"description": "Success"}
		}
		responses["4XX"] = map[string]any{
			"description": "Client error",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		}
		op["responses"] = responses

		method := strings.ToLower(MethodForAction(rt.Action))
		if _, ok := paths[rt.Path]; !ok {
			paths[rt.Path] = map[string]any{}
		}
		paths[rt.Path].(map[string]any)[method] = op
	}

	spec["paths"] = paths
	b, _ := json.MarshalIndent(spec, "", "  ")
	return b
}

// typeToSchema converts a Go struct type to a JSON Schema object.
func typeToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return map[string]any{"type": goTypeToJSONType(t)}
	}

	properties := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		omitempty := strings.Contains(tag, "omitempty")

		ft := f.Type
		isOptional := false

		if ft.Kind() == reflect.Struct && ft.NumField() == 2 && ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set" {
			ft = ft.Field(0).Type
			isOptional = true
		}

		prop := goTypeToProperty(ft)
		properties[name] = prop

		if !isOptional && !omitempty && ft.Kind() != reflect.Ptr {
			required = append(required, name)
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

func goTypeToProperty(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		inner := goTypeToProperty(t.Elem())
		inner["nullable"] = true
		return inner
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{}
		}
		return map[string]any{"type": "array", "items": goTypeToProperty(t.Elem())}
	default:
		if t == reflect.TypeOf(time.Time{}) {
			return map[string]any{"type": "string", "format": "date-time"}
		}
		if t == reflect.TypeOf(json.RawMessage{}) {
			return map[string]any{"type": "object", "description": "JSON object"}
		}
		return map[string]any{"type": "string"}
	}
}

func goTypeToJSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Bool:
		return "boolean"
	default:
		return "string"
	}
}

func typeName(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Slice {
		return t.Elem().Name()
	}
	return t.Name()
}

func tagFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return "other"
	}
	tag := parts[0]
	if tag == "boards" && len(parts) >= 4 {
		switch parts[2] {
		case "tasks":
			if len(parts) >= 5 {
				switch parts[4] {
				case "comments":
					return "comments"
				case "dependencies":
					return "dependencies"
				case "attachments":
					return "attachments"
				case "audit":
					return "audit"
				case "transition":
					return "tasks"
				}
			}
			return "tasks"
		case "workflow":
			return "workflows"
		case "audit":
			return "audit"
		case "reassign":
			return "boards"
		}
	}
	return tag
}
