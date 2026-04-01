// Package cli generates Cobra commands from the domain operation definitions.
// Each operation becomes a CLI command that makes an HTTP call to the TaskFlow server.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

// Config holds the CLI configuration.
type Config struct {
	ServerURL string
	APIKey    string
}

var activeConfig *Config

// SetConfig sets the active CLI configuration. Called by the root command's
// PersistentPreRun after Viper resolves flags, env vars, and config file.
func SetConfig(cfg Config) {
	activeConfig = &cfg
}

func getConfig() Config {
	if activeConfig != nil {
		return *activeConfig
	}
	return Config{ServerURL: "http://localhost:8374"}
}

// checkConfig validates the config and returns a helpful error if something is missing.
func checkConfig(cfg Config) error {
	if cfg.APIKey == "" {
		return fmt.Errorf(`no API key configured

Set your API key using one of:
  --api-key <key>                      (command line flag)
  export TASKFLOW_API_KEY=<key>        (environment variable)
  echo "api_key: <key>" >> ~/.config/taskflow/config.yaml  (config file)

The seed admin key is written to seed-admin-key.txt on first server start.`)
	}
	return nil
}

// cmdSpec is the CLI-internal representation of a command derived from a
// domain resource or operation.
type cmdSpec struct {
	summary string
	input   any // nil for resources / delete operations
	output  any
	params  []model.QueryParam
	// Exactly one of these is set:
	resource  *model.Resource
	operation *model.Operation
}

// BuildCLI generates a Cobra command tree from model.Resources() and model.Operations().
// Config is resolved lazily via SetConfig before commands run.
func BuildCLI(cfg *Config) *cobra.Command {
	if cfg != nil {
		SetConfig(*cfg)
	}
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

	// Resources (read-only, GET).
	for _, res := range model.Resources() {
		res := res
		spec := cmdSpec{
			summary:  res.Summary,
			output:   res.Output,
			params:   res.QueryParams(),
			resource: &res,
		}
		group, verb := splitName(res.Name)
		cmd := buildCommand(verb, res.PathParams(), spec)
		getGroup(group).AddCommand(cmd)
	}

	// Operations (mutations).
	for _, op := range model.Operations() {
		op := op
		spec := cmdSpec{
			summary:   op.Summary,
			input:     op.Input,
			output:    op.Output,
			operation: &op,
		}
		group, verb := splitName(op.Name)
		cmd := buildCommand(verb, op.PathParams(), spec)
		getGroup(group).AddCommand(cmd)
	}

	return root
}

// splitName splits a "resource_action" name into (resource, action).
func splitName(name string) (group, verb string) {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return name, name
}

// buildCommand creates a Cobra command from a cmdSpec.
func buildCommand(verb string, pathParams []model.PathParam, spec cmdSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   buildUsageLine(verb, pathParams),
		Short: spec.summary,
		RunE:  makeRunFunc(spec, pathParams),
	}

	if spec.input != nil {
		addFlags(cmd, spec.input)
	}
	for _, p := range spec.params {
		switch p.Type {
		case "boolean":
			cmd.Flags().Bool(p.Name, false, p.Desc)
		default:
			cmd.Flags().String(p.Name, "", p.Desc)
		}
	}
	cmd.Flags().Bool("json", false, "Output raw JSON")

	return cmd
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

		desc := flagDescription(name, ft)

		switch ft.Kind() {
		case reflect.String:
			cmd.Flags().String(name, "", desc)
		case reflect.Int:
			cmd.Flags().Int(name, 0, desc)
		case reflect.Bool:
			cmd.Flags().Bool(name, false, desc)
		case reflect.Slice:
			cmd.Flags().StringSlice(name, nil, desc)
		}
	}
}

// validateRequiredFlags checks that all required input fields have corresponding flags set.
// Required means: exported, has a json tag (not "-"), not a pointer, not Optional, not omitempty.
func validateRequiredFlags(cmd *cobra.Command, input any) error {
	if input == nil {
		return nil
	}
	t := reflect.TypeOf(input)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var missing []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if strings.Contains(tag, "omitempty") {
			continue
		}

		ft := f.Type
		// Optional[T] fields are never required.
		if ft.Kind() == reflect.Struct && ft.NumField() == 2 && ft.Field(0).Name == "Value" && ft.Field(1).Name == "Set" {
			continue
		}
		// Pointer fields are nullable, not required.
		if ft.Kind() == reflect.Ptr {
			continue
		}
		// Slice fields are required only if they can't be empty.
		if ft.Kind() == reflect.Slice {
			if !cmd.Flags().Changed(name) {
				missing = append(missing, "--"+name)
			}
			continue
		}

		if !cmd.Flags().Changed(name) {
			missing = append(missing, "--"+name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// flagDescription returns a help description for a flag, including valid values for known enums.
var enumDescriptions = map[string]string{
	"priority": "Priority (critical, high, medium, low, none)",
	"role":     "Role (admin, member, read_only)",
	"type":     "Actor type (human, ai_agent)",
	"ref_type": "Reference type (url, file, git_commit, git_branch, git_pr)",
	"dep_type": "Dependency type (depends_on, relates_to)",
}

func flagDescription(name string, _ reflect.Type) string {
	if desc, ok := enumDescriptions[name]; ok {
		return desc
	}
	// Use the flag name as-is for unknown fields.
	return strings.ReplaceAll(name, "_", " ")
}

func makeRunFunc(spec cmdSpec, pathParams []model.PathParam) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg := getConfig()
		if err := checkConfig(cfg); err != nil {
			cmd.SilenceUsage = true
			return err
		}
		if len(args) < len(pathParams) {
			return fmt.Errorf("expected %d argument(s): %s", len(pathParams), pathParamNames(pathParams))
		}
		if err := validateRequiredFlags(cmd, spec.input); err != nil {
			return err
		}
		cmd.SilenceUsage = true

		// Build path params from positional args.
		params := httpclient.PathParams{}
		for i, p := range pathParams {
			params[p.Name] = args[i]
		}

		client := httpclient.New(cfg.ServerURL, cfg.APIKey)
		var raw json.RawMessage

		var err error
		if spec.resource != nil {
			filter := buildFilterFromFlags(cmd, spec.params)
			raw, err = httpclient.GetOne[json.RawMessage](client, *spec.resource, params, filter)
		} else {
			var body any
			if spec.input != nil {
				b := buildBodyFromFlags(cmd, spec.input)
				if len(b) > 0 {
					body = b
				}
			}
			raw, err = httpclient.Exec[json.RawMessage](client, *spec.operation, params, body)
		}
		if err != nil {
			return err
		}

		if len(raw) == 0 {
			return nil
		}

		w := cmd.OutOrStdout()
		useJSON, _ := cmd.Flags().GetBool("json")
		if useJSON {
			fmt.Fprintln(w, string(raw))
			return nil
		}

		return formatOutput(w, raw, spec.output)
	}
}

// buildFilterFromFlags builds a map of query parameter values from CLI flags.
// Returns nil if no flags were set.
func buildFilterFromFlags(cmd *cobra.Command, params []model.QueryParam) map[string]string {
	if len(params) == 0 {
		return nil
	}
	vals := map[string]string{}
	for _, p := range params {
		switch p.Type {
		case "boolean":
			if v, _ := cmd.Flags().GetBool(p.Name); v {
				vals[p.Name] = "true"
			}
		default:
			if v, _ := cmd.Flags().GetString(p.Name); v != "" {
				vals[p.Name] = v
			}
		}
	}
	if len(vals) == 0 {
		return nil
	}
	return vals
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
