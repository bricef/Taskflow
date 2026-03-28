package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/bricef/taskflow/internal/cli"
)

func main() {
	root := cli.BuildCLI(nil)
	root.SilenceErrors = true // we handle error printing in main

	// Global flags — bound to Viper so they override config file and env vars.
	root.PersistentFlags().String("url", "http://localhost:8374", "TaskFlow server URL")
	root.PersistentFlags().String("api-key", "", "API key for authentication")
	viper.BindPFlag("url", root.PersistentFlags().Lookup("url"))
	viper.BindPFlag("api_key", root.PersistentFlags().Lookup("api-key"))

	// Env vars: TASKFLOW_URL, TASKFLOW_API_KEY
	viper.SetEnvPrefix("TASKFLOW")
	viper.AutomaticEnv()

	// Config file: ~/.config/taskflow/config.yaml (or .json, .toml)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/taskflow")
	viper.AddConfigPath(".")
	viper.ReadInConfig() // ignore error — config file is optional

	// Resolve config before each command runs.
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cli.SetConfig(cli.Config{
			ServerURL: strings.TrimSpace(viper.GetString("url")),
			APIKey:    strings.TrimSpace(viper.GetString("api_key")),
		})
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
