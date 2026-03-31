package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestCLICommandTreeGolden(t *testing.T) {
	root := BuildCLI(nil)

	var commands []string
	walkCommands(root, "", &commands)
	sort.Strings(commands)

	got := strings.Join(commands, "\n") + "\n"

	goldenPath := "testdata/commands.golden.txt"

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Log("updated golden file")
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found — run 'go test -update' to create it: %v", err)
	}

	if got != string(expected) {
		t.Errorf("CLI command tree does not match golden file.\n\nRun 'go test ./internal/cli/ -update' to regenerate.\n\nTo see the diff:\n  go test ./internal/cli/ -run TestCLICommandTreeGolden -update && git diff testdata/commands.golden.txt")
	}
}

// walkCommands recursively walks the command tree and collects "group subcommand" usage lines.
func walkCommands(cmd *cobra.Command, prefix string, out *[]string) {
	for _, child := range cmd.Commands() {
		full := child.Name()
		if prefix != "" {
			full = prefix + " " + child.Name()
		}
		if child.HasSubCommands() {
			walkCommands(child, full, out)
		} else {
			// Include the full Use line for leaf commands (has args info).
			use := child.Use
			if prefix != "" {
				use = prefix + " " + use
			}
			line := fmt.Sprintf("%-50s %s", use, child.Short)
			*out = append(*out, line)
		}
	}
}
