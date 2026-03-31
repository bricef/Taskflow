package model

import (
	"fmt"
	"net/url"
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

// SubstitutePath replaces {param} placeholders in a path template with values
// from params. For example, SubstitutePath("/boards/{slug}/tasks/{num}",
// {"slug": "platform", "num": "1"}) returns "/boards/platform/tasks/1".
func SubstitutePath(path string, params map[string]string) string {
	for k, v := range params {
		path = strings.Replace(path, "{"+k+"}", v, 1)
	}
	return path
}

// BuildQueryString builds a URL query string (without leading "?") from v.
// v can be:
//   - a struct with `query` tags (non-zero fields are included)
//   - a map[string]string (all entries are included)
//   - nil (returns "")
func BuildQueryString(v any) string {
	if v == nil {
		return ""
	}

	// Handle map[string]string directly.
	if m, ok := v.(map[string]string); ok {
		vals := url.Values{}
		for k, v := range m {
			vals.Set(k, v)
		}
		return vals.Encode()
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	t := rv.Type()
	if t.Kind() != reflect.Struct {
		return ""
	}

	vals := url.Values{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("query")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := parseQueryTag(tag, f.Type)
		fv := rv.Field(i)

		switch fv.Kind() {
		case reflect.Ptr:
			if fv.IsNil() {
				continue
			}
			vals.Set(name, fmt.Sprint(fv.Elem().Interface()))
		case reflect.Bool:
			if fv.Bool() {
				vals.Set(name, "true")
			}
		case reflect.String:
			if s := fv.String(); s != "" {
				vals.Set(name, s)
			}
		case reflect.Int, reflect.Int64:
			if n := fv.Int(); n != 0 {
				vals.Set(name, fmt.Sprint(n))
			}
		}
	}
	return vals.Encode()
}
