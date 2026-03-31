package model

import (
	"reflect"
	"strings"
)

// QueryParam describes a query string parameter derived from a filter or sort struct.
type QueryParam struct {
	Name string
	Type string // "string", "integer", "boolean"
	Desc string
}

// QueryParamsFrom derives query parameters from a struct's `query` tags.
// Tag format: `query:"name,description"`. Fields without a query tag are skipped.
// Returns nil if v is nil.
func QueryParamsFrom(v any) []QueryParam {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var params []QueryParam
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("query")
		if tag == "" || tag == "-" {
			continue
		}
		name, typ, desc := parseQueryTag(tag, f.Type)
		params = append(params, QueryParam{
			Name: name,
			Type: typ,
			Desc: desc,
		})
	}
	return params
}

// parseQueryTag parses a query tag into name, type, and description.
// Supported formats:
//
//	`query:"name,description"`               — type inferred from Go type
//	`query:"name,type_override,description"` — explicit type override
func parseQueryTag(tag string, goType reflect.Type) (name, typ, desc string) {
	parts := strings.SplitN(tag, ",", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], queryType(goType), parts[1]
	default:
		return parts[0], queryType(goType), ""
	}
}

// queryType maps a Go type to a query parameter type string.
func queryType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int64:
		return "integer"
	default:
		return "string"
	}
}
