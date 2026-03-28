// Package cli generates Cobra commands from the domain operation definitions.
// Each operation becomes a CLI command that makes an HTTP call to the TaskFlow server.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	taskflowhttp "github.com/bricef/taskflow/internal/http"
	"github.com/bricef/taskflow/internal/model"
)

// Config holds the CLI configuration.
type Config struct {
	ServerURL string
	APIKey    string
}

// BuildCLI generates a Cobra command tree from model.Operations().
func BuildCLI(cfg Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "taskflow",
		Short: "TaskFlow CLI — human/AI collaborative task tracker",
	}

	groups := map[string]*cobra.Command{}
	getGroup := func(name string) *cobra.Command {
		if g, ok := groups[name]; ok {
			return g
		}
		g := &cobra.Command{Use: name, Short: "Manage " + name}
		groups[name] = g
		root.AddCommand(g)
		return g
	}

	for _, op := range model.Operations() {
		op := op
		group, verb := deriveCommandName(op.Action, op.Path)
		pathParams := op.PathParams()

		cmd := &cobra.Command{
			Use:   buildUsageLine(verb, pathParams),
			Short: op.Summary,
			RunE:  makeRunFunc(cfg, op),
		}

		if op.Input != nil {
			addFlags(cmd, op.Input)
		}
		for _, p := range op.Params {
			switch p.Type {
			case "boolean":
				cmd.Flags().Bool(p.Name, false, p.Desc)
			default:
				cmd.Flags().String(p.Name, "", p.Desc)
			}
		}
		cmd.Flags().Bool("json", false, "Output raw JSON")

		getGroup(group).AddCommand(cmd)
	}

	return root
}

func deriveCommandName(action model.Action, path string) (group, verb string) {
	segments := strings.Split(strings.Trim(path, "/"), "/")

	var names []string
	for _, s := range segments {
		if !strings.HasPrefix(s, "{") {
			names = append(names, s)
		}
	}

	customActions := map[model.Action]bool{
		model.ActionTransition: true,
		model.ActionReassign:   true,
		model.ActionHealth:     true,
	}

	if len(names) >= 2 && customActions[action] {
		group = singularize(names[len(names)-2])
		verb = names[len(names)-1]
	} else {
		group = singularize(names[len(names)-1])
		verb = string(action)
	}

	return group, verb
}

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	return strings.TrimSuffix(s, "s")
}

func buildUsageLine(verb string, pathParams []model.PathParam) string {
	parts := []string{verb}
	for _, p := range pathParams {
		parts = append(parts, "<"+p.Name+">")
	}
	return strings.Join(parts, " ")
}

func addFlags(cmd *cobra.Command, input any) {
	t := reflect.TypeOf(input)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		ft := f.Type

		if ft.Kind() == reflect.Struct && ft.NumField() == 2 && ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set" {
			ft = ft.Field(0).Type
		}

		switch ft.Kind() {
		case reflect.String:
			cmd.Flags().String(name, "", f.Name)
		case reflect.Int:
			cmd.Flags().Int(name, 0, f.Name)
		case reflect.Bool:
			cmd.Flags().Bool(name, false, f.Name)
		case reflect.Slice:
			cmd.Flags().StringSlice(name, nil, f.Name)
		}
	}
}

func makeRunFunc(cfg Config, op model.Operation) func(*cobra.Command, []string) error {
	pathParams := op.PathParams()

	return func(cmd *cobra.Command, args []string) error {
		if len(args) < len(pathParams) {
			return fmt.Errorf("expected %d argument(s): %s", len(pathParams), pathParamNames(pathParams))
		}

		url := cfg.ServerURL + op.Path
		for i, p := range pathParams {
			url = strings.Replace(url, "{"+p.Name+"}", args[i], 1)
		}

		var query []string
		for _, p := range op.Params {
			switch p.Type {
			case "boolean":
				if v, _ := cmd.Flags().GetBool(p.Name); v {
					query = append(query, p.Name+"=true")
				}
			default:
				if v, _ := cmd.Flags().GetString(p.Name); v != "" {
					query = append(query, p.Name+"="+v)
				}
			}
		}
		if len(query) > 0 {
			url += "?" + strings.Join(query, "&")
		}

		var bodyReader io.Reader
		if op.Input != nil {
			body := buildBodyFromFlags(cmd, op.Input)
			if len(body) > 0 {
				b, _ := json.Marshal(body)
				bodyReader = bytes.NewReader(b)
			}
		}

		req, err := http.NewRequest(taskflowhttp.MethodForAction(op.Action), url, bodyReader)
		if err != nil {
			return err
		}
		if bodyReader != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode >= 400 {
			var errResp map[string]any
			json.Unmarshal(respBody, &errResp)
			msg := "unknown error"
			if m, ok := errResp["message"].(string); ok {
				msg = m
			}
			return fmt.Errorf("error %d: %s", resp.StatusCode, msg)
		}

		if resp.StatusCode == 204 || len(respBody) == 0 {
			return nil
		}

		w := cmd.OutOrStdout()
		useJSON, _ := cmd.Flags().GetBool("json")
		if useJSON {
			fmt.Fprintln(w, string(respBody))
			return nil
		}

		return formatOutput(w, respBody, op.Output)
	}
}

func pathParamNames(params []model.PathParam) string {
	names := make([]string, len(params))
	for i, p := range params {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

func buildBodyFromFlags(cmd *cobra.Command, input any) map[string]any {
	body := map[string]any{}
	t := reflect.TypeOf(input)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return body
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if !cmd.Flags().Changed(name) {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Struct && ft.NumField() == 2 && ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set" {
			ft = ft.Field(0).Type
		}
		switch ft.Kind() {
		case reflect.String:
			v, _ := cmd.Flags().GetString(name)
			body[name] = v
		case reflect.Int:
			v, _ := cmd.Flags().GetInt(name)
			body[name] = v
		case reflect.Bool:
			v, _ := cmd.Flags().GetBool(name)
			body[name] = v
		case reflect.Slice:
			v, _ := cmd.Flags().GetStringSlice(name)
			body[name] = v
		}
	}
	return body
}

func formatOutput(w io.Writer, data []byte, outputType any) error {
	if outputType == nil {
		fmt.Fprintln(w, string(data))
		return nil
	}
	t := reflect.TypeOf(outputType)
	if t.Kind() == reflect.Slice {
		return formatTable(w, data)
	}
	return formatSingle(w, data)
}

func formatTable(out io.Writer, data []byte) error {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		fmt.Fprintln(out, string(data))
		return nil
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "(no results)")
		return nil
	}
	columns := orderedKeys(rows[0])
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(upperColumns(columns), "\t"))
	for _, row := range rows {
		vals := make([]string, len(columns))
		for i, col := range columns {
			vals[i] = formatValue(row[col])
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	return w.Flush()
}

func formatSingle(out io.Writer, data []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		fmt.Fprintln(out, string(data))
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, k := range orderedKeys(obj) {
		fmt.Fprintf(w, "%s:\t%s\n", k, formatValue(obj[k]))
	}
	return w.Flush()
}

func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int(val)) {
			return strconv.Itoa(int(val))
		}
		return fmt.Sprintf("%.2f", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ", ")
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func orderedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func upperColumns(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = strings.ToUpper(c)
	}
	return out
}
