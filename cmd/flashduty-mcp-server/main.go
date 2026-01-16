package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/flashduty"
	flashdutyPkg "github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
)

// These variables are set by the build process using ldflags.
var (
	version = "version"
	commit  = "commit"
	date    = "date"
)

var (
	rootCmd = &cobra.Command{
		Use:     "server",
		Short:   "Flashduty MCP Server",
		Long:    `A Flashduty MCP server that handles various incident management tools and resources.`,
		Version: fmt.Sprintf("Version: %s\nCommit: %s\nBuild Date: %s", version, commit, date),
	}

	stdioCmd = &cobra.Command{
		Use:   "stdio",
		Short: "Start stdio server",
		Long:  `Start a server that communicates via standard input/output streams using JSON-RPC messages.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			appKey := viper.GetString("app_key")
			if appKey == "" {
				return errors.New("app key not provided: use --app-key flag or set FLASHDUTY_APP_KEY environment variable")
			}

			// If you're wondering why we're not using viper.GetStringSlice("toolsets"),
			// it's because viper doesn't handle comma-separated values correctly for env
			// vars when using GetStringSlice.
			// https://github.com/spf13/viper/issues/380
			var enabledToolsets []string
			if viper.IsSet("toolsets") {
				toolsetsVal := viper.Get("toolsets")
				if s, ok := toolsetsVal.(string); ok {
					enabledToolsets = strings.Split(s, ",")
				} else if sl, ok := toolsetsVal.([]string); ok {
					enabledToolsets = sl
				} else {
					return fmt.Errorf("failed to parse 'toolsets': unexpected type %T", toolsetsVal)
				}
			}

			stdioServerConfig := flashduty.StdioServerConfig{
				Version:              version,
				BaseURL:              viper.GetString("base_url"),
				APPKey:               appKey,
				EnabledToolsets:      enabledToolsets,
				ReadOnly:             viper.GetBool("read-only"),
				OutputFormat:         viper.GetString("output-format"),
				ExportTranslations:   viper.GetBool("export-translations"),
				EnableCommandLogging: viper.GetBool("enable-command-logging"),
				LogFilePath:          viper.GetString("log-file"),
			}
			return flashduty.RunStdioServer(stdioServerConfig)
		},
	}

	httpCmd = &cobra.Command{
		Use:   "http",
		Short: "Start HTTP server",
		Long:  `Start a streamable HTTP server.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			httpServerConfig := flashduty.HTTPServerConfig{
				Version:     version,
				Commit:      commit,
				Date:        date,
				BaseURL:     viper.GetString("base_url"),
				Port:        viper.GetString("port"),
				LogFilePath: viper.GetString("log-file"),
			}
			return flashduty.RunHTTPServer(httpServerConfig)
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.SetGlobalNormalizationFunc(wordSepNormalizeFunc)
	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")

	// Add global flags that will be shared by all commands
	rootCmd.PersistentFlags().String("app-key", "", "Flashduty APP key (can also be set via FLASHDUTY_APP_KEY environment variable)")
	rootCmd.PersistentFlags().StringSlice("toolsets", flashdutyPkg.DefaultTools, "An optional comma separated list of groups of tools to allow, defaults to enabling all")
	rootCmd.PersistentFlags().Bool("read-only", false, "Restrict the server to read-only operations")
	rootCmd.PersistentFlags().String("output-format", "json", "Output format for tool results: json (default) or toon (Token-Oriented Object Notation for reduced token usage)")
	rootCmd.PersistentFlags().String("log-file", "", "Path to log file")
	rootCmd.PersistentFlags().Bool("enable-command-logging", false, "When enabled, the server will log all command requests and responses to the log file")
	rootCmd.PersistentFlags().Bool("export-translations", false, "Save translations to a JSON file")
	rootCmd.PersistentFlags().String("base-url", "https://api.flashcat.cloud", "Specify the Flashduty API base URL")

	// Add flags for http command
	httpCmd.Flags().String("port", "11310", "Port to listen on")

	// Bind flag to viper
	_ = viper.BindPFlag("app_key", rootCmd.PersistentFlags().Lookup("app-key"))
	_ = viper.BindPFlag("toolsets", rootCmd.PersistentFlags().Lookup("toolsets"))
	_ = viper.BindPFlag("read-only", rootCmd.PersistentFlags().Lookup("read-only"))
	_ = viper.BindPFlag("output-format", rootCmd.PersistentFlags().Lookup("output-format"))
	_ = viper.BindPFlag("log-file", rootCmd.PersistentFlags().Lookup("log-file"))
	_ = viper.BindPFlag("enable-command-logging", rootCmd.PersistentFlags().Lookup("enable-command-logging"))
	_ = viper.BindPFlag("export-translations", rootCmd.PersistentFlags().Lookup("export-translations"))
	_ = viper.BindPFlag("base_url", rootCmd.PersistentFlags().Lookup("base-url"))
	_ = viper.BindPFlag("port", httpCmd.Flags().Lookup("port"))

	// Add subcommands
	rootCmd.AddCommand(stdioCmd)
	rootCmd.AddCommand(httpCmd)
}

func initConfig() {
	// Initialize Viper configuration
	viper.SetEnvPrefix("flashduty")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func wordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	from := []string{"_"}
	to := "-"
	for _, sep := range from {
		name = strings.ReplaceAll(name, sep, to)
	}
	return pflag.NormalizedName(name)
}
