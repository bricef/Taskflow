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

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/transport"
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

// friendlyRequestError returns a user-friendly error message for connection failures.
func friendlyRequestError(err error, serverURL string) error {
	return fmt.Errorf(`could not connect to TaskFlow server at %s

%w

Is the server running? To use a different server URL:
  --url <url>                          (command line flag)
  export TASKFLOW_URL=<url>            (environment variable)
  echo "url: <url>" >> ~/.config/taskflow/config.yaml  (config file)`, serverURL, err)
}

// cmdSpec is the CLI-internal representation of a command derived from a
// domain resource or operation. It captures everything needed to generate
// the Cobra command and its run function.
type cmdSpec struct {
	path    string
	summary string
	method  string // HTTP method
	input   any    // nil for resources / delete operations
	output  any
	params  []model.QueryParam
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
		spec := cmdSpec{
			path:    res.Path,
			summary: res.Summary,
			method:  "GET",
			output:  res.Output,
			params:  res.QueryParams(),
		}
		group, verb := splitName(res.Name)
		cmd := buildCommand(verb, res.PathParams(), spec)
		getGroup(group).AddCommand(cmd)
	}

	// Operations (mutations).
	for _, op := range model.Operations() {
		spec := cmdSpec{
			path:    op.Path,
			summary: op.Summary,
			method:  transport.MethodForAction(op.Action),
			input:   op.Input,
			output:  op.Output,
		}
		group, verb := splitName(op.Name)
		cmd := buildCommand(verb, op.PathParams(), spec)
		getGroup(group).AddCommand(cmd)
	}

	// Convenience commands (not derived from domain resources/operations).
	addConvenienceCommands(root, getGroup)

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

func addConvenienceCommands(root *cobra.Command, getGroup func(string) *cobra.Command) {
	// taskflow search --q <query>
	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "Search tasks across all boards",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig()
			q, _ := cmd.Flags().GetString("q")
			if q == "" {
				return fmt.Errorf("--q flag is required")
			}
			url := "/search?q=" + q
			if v, _ := cmd.Flags().GetString("state"); v != "" {
				url += "&state=" + v
			}
			if v, _ := cmd.Flags().GetString("assignee"); v != "" {
				url += "&assignee=" + v
			}
			if v, _ := cmd.Flags().GetString("priority"); v != "" {
				url += "&priority=" + v
			}
			return doGet(cmd, cfg, url)
		},
	}
	searchCmd.Flags().String("q", "", "Search query (required)")
	searchCmd.Flags().String("state", "", "Filter by state")
	searchCmd.Flags().String("assignee", "", "Filter by assignee (supports @me)")
	searchCmd.Flags().String("priority", "", "Filter by priority")
	searchCmd.Flags().Bool("json", false, "Output raw JSON")
	root.AddCommand(searchCmd)
}

// doGet is a helper for convenience commands that make a simple GET request.
func doGet(cmd *cobra.Command, cfg Config, path string) error {
	if err := checkConfig(cfg); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	cmd.SilenceUsage = true
	req, err := http.NewRequest("GET", cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return friendlyRequestError(err, cfg.ServerURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.Unmarshal(body, &errResp)
		msg := "unknown error"
		if m, ok := errResp["message"].(string); ok {
			msg = m
		}
		return fmt.Errorf("error %d: %s", resp.StatusCode, msg)
	}

	w := cmd.OutOrStdout()
	useJSON, _ := cmd.Flags().GetBool("json")
	if useJSON || resp.StatusCode == 204 {
		fmt.Fprintln(w, string(body))
		return nil
	}

	// Pretty-print JSON.
	var obj any
	if err := json.Unmarshal(body, &obj); err != nil {
		fmt.Fprintln(w, string(body))
		return nil
	}
	pretty, _ := json.MarshalIndent(obj, "", "  ")
	fmt.Fprintln(w, string(pretty))
	return nil
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

		url := cfg.ServerURL + spec.path
		for i, p := range pathParams {
			url = strings.Replace(url, "{"+p.Name+"}", args[i], 1)
		}

		var query []string
		for _, p := range spec.params {
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
		if spec.input != nil {
			body := buildBodyFromFlags(cmd, spec.input)
			if len(body) > 0 {
				b, _ := json.Marshal(body)
				bodyReader = bytes.NewReader(b)
			}
		}

		req, err := http.NewRequest(spec.method, url, bodyReader)
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
			return friendlyRequestError(err, cfg.ServerURL)
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

		return formatOutput(w, respBody, spec.output)
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
