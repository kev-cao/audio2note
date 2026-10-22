package cmd

import (
	"audio2note/internal/config"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// setting describes one config key: how to read/write it on a config.Config,
// the help text shown by `config help`, and (for restricted keys like the
// adapter selectors) the set of valid values. This is the "map from config keys
// to help messages" that drives `config get/set/help`.
type setting struct {
	key  string
	help string
	// options, when non-nil, restricts valid values (and `config help` lists
	// them). nil means free-form text.
	options func() []string
	get     func(config.Config) string
	// set applies a new value. nil means the key is not settable via
	// `config set` (e.g. secrets managed elsewhere); setHint explains the
	// alternative in the rejection message.
	set     func(*config.Config, string)
	setHint string
}

// maskedToken renders a secret for display: fixed-width asterisks when set (the
// real value is never printed), with a hint to change it via plaud auth setup.
func maskedToken(tok string) string {
	if tok == "" {
		return "(not set — run `audio2note plaud auth setup` to set it)"
	}
	return "******** (to change, run `audio2note plaud auth setup`)"
}

var settings = []setting{
	{
		key:     "defaultTranscriber",
		help:    "Transcriber adapter used when --transcriber is not passed.",
		options: transcriber.Names,
		get:     func(c config.Config) string { return c.DefaultTranscriber },
		set:     func(c *config.Config, v string) { c.DefaultTranscriber = v },
	},
	{
		key:     "defaultSummarizer",
		help:    "Summarizer adapter (the AI harness) used when --summarizer is not passed.",
		options: summarizer.Names,
		get:     func(c config.Config) string { return c.DefaultSummarizer },
		set:     func(c *config.Config, v string) { c.DefaultSummarizer = v },
	},
	{
		key: "basePrompt",
		help: "Base instruction the summarizer opens with, replacing the built-in default. " +
			"Use " + config.TranscriptPlaceholder + " where the transcript path should go " +
			"(if omitted, the path is appended automatically). The --append-prompt flag adds " +
			"text after this base prompt.",
		get: func(c config.Config) string { return c.BasePrompt },
		set: func(c *config.Config, v string) { c.BasePrompt = v },
	},
	{
		key: "obsidianVault",
		help: "Default Obsidian vault path to write notes into. Enables vault output on its own; " +
			"--obsidian-vault overrides it for a single run, and --obsidian forces vault output using this value.",
		get: func(c config.Config) string { return c.ObsidianVault },
		set: func(c *config.Config, v string) { c.ObsidianVault = v },
	},
	{
		key:     "plaudToken",
		help:    "Your Plaud bearer token, used by the `plaud` commands. Managed via `audio2note plaud auth setup`; shown masked.",
		get:     func(c config.Config) string { return maskedToken(c.PlaudToken) },
		set:     nil, // secret — set it via `plaud auth setup`
		setHint: "run `audio2note plaud auth setup`",
	},
}

func findSetting(key string) (setting, bool) {
	for _, s := range settings {
		if s.key == key {
			return s, true
		}
	}
	return setting{}, false
}

// settingKeys returns all known keys, for error messages.
func settingKeys() []string {
	keys := make([]string, len(settings))
	for i, s := range settings {
		keys[i] = s.key
	}
	return keys
}

func unknownKeyErr(key string) error {
	return fmt.Errorf("unknown config key %q (known keys: %s)", key, strings.Join(settingKeys(), ", "))
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage audio2note configuration (~/.audio2note/config.json)",
	}
	cmd.AddCommand(newConfigGetCmd(), newConfigSetCmd(), newConfigHelpCmd())
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [key]",
		Short: "Print the effective value of a config key (or all keys)",
		Long: "Print the effective configuration value — the on-disk setting if present, " +
			"otherwise the built-in default. With no key, prints every key.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Effective()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				for _, s := range settings {
					fmt.Printf("%s: %s\n", s.key, s.get(cfg))
				}
				return nil
			}
			s, ok := findSetting(args[0])
			if !ok {
				return unknownKeyErr(args[0])
			}
			fmt.Println(s.get(cfg))
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "set <key> <value>",
		Short:         "Set a config key and write it to ~/.audio2note/config.json",
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			s, ok := findSetting(key)
			if !ok {
				return unknownKeyErr(key)
			}
			if s.set == nil {
				return fmt.Errorf("%s can't be set with `config set` — %s", key, s.setHint)
			}
			if s.options != nil && !slices.Contains(s.options(), value) {
				return fmt.Errorf("invalid value %q for %s (one of: %s)", value, key, strings.Join(s.options(), ", "))
			}
			// Load the RAW file (not the merged defaults) so Save round-trips
			// only the keys the user has actually set.
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s.set(&cfg, value)
			if err := config.Save(cfg); err != nil {
				return err
			}
			path, _ := config.Path()
			fmt.Printf("✓ set %s in %s\n", key, path)
			return nil
		},
	}
}

func newConfigHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "help [key]",
		Short:         "Describe a config key (or list all keys with their help)",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				for _, s := range settings {
					printSettingHelp(s)
					fmt.Println()
				}
				return nil
			}
			s, ok := findSetting(args[0])
			if !ok {
				return unknownKeyErr(args[0])
			}
			printSettingHelp(s)
			return nil
		},
	}
}

func printSettingHelp(s setting) {
	fmt.Printf("%s\n  %s\n", s.key, s.help)
	if s.options != nil {
		fmt.Printf("  (one of: %s)\n", strings.Join(s.options(), ", "))
	}
}
