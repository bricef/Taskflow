package http

import (
	"regexp"
	"strings"

	"github.com/bricef/taskflow/internal/model"
)

type routeBuilder struct{ r route }

func newRoute(method, path, summary string, defaultRole model.Role, defaultStatus int) *routeBuilder {
	return &routeBuilder{r: route{
		Method:  method,
		Path:    path,
		Summary: summary,
		MinRole: defaultRole,
		Status:  defaultStatus,
	}}
}

func Get(path, summary string) *routeBuilder {
	return newRoute("GET", path, summary, model.RoleReadOnly, 200)
}

func Post(path, summary string) *routeBuilder {
	return newRoute("POST", path, summary, model.RoleMember, 201)
}

func Patch(path, summary string) *routeBuilder {
	return newRoute("PATCH", path, summary, model.RoleMember, 200)
}

func Put(path, summary string) *routeBuilder {
	return newRoute("PUT", path, summary, model.RoleMember, 204)
}

func Delete(path, summary string) *routeBuilder {
	return newRoute("DELETE", path, summary, model.RoleMember, 204)
}

func (b *routeBuilder) Role(r model.Role) *routeBuilder { b.r.MinRole = r; return b }
func (b *routeBuilder) Status(s int) *routeBuilder      { b.r.Status = s; return b }
func (b *routeBuilder) Input(v any) *routeBuilder       { b.r.Input = v; return b }
func (b *routeBuilder) Output(v any) *routeBuilder      { b.r.Output = v; return b }

func (b *routeBuilder) QueryParams(params ...paramMeta) *routeBuilder {
	b.r.Params = append(b.r.Params, params...)
	return b
}

func Query(name, typ, desc string) paramMeta {
	return paramMeta{Name: name, In: "query", Type: typ, Desc: desc}
}

// Handle is the terminal method — sets the handler, infers path params, and returns the built route.
func (b *routeBuilder) Handle(h handler) route {
	b.r.Handler = h
	b.r.Params = append(inferPathParams(b.r.Path), b.r.Params...)
	return b.r
}

var pathParamRegex = regexp.MustCompile(`\{(\w+)\}`)

// Known path param types by convention.
var intParams = map[string]bool{"num": true, "id": true}

func inferPathParams(path string) []paramMeta {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	var params []paramMeta
	for _, m := range matches {
		name := m[1]
		typ := "string"
		if intParams[strings.ToLower(name)] {
			typ = "integer"
		}
		params = append(params, paramMeta{Name: name, In: "path", Type: typ, Required: true})
	}
	return params
}
