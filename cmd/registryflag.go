package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// registryValue is a pflag.Value backed by a registry's Names() function.
// Set() validates input against the allowed name list at parse time; the
// stored value (*target) is only updated on a successful match.
type registryValue struct {
	target  *string
	allowed func() []string
}

func (v *registryValue) String() string { return *v.target }
func (v *registryValue) Type() string   { return "string" }

func (v *registryValue) Set(s string) error {
	for _, a := range v.allowed() {
		if a == s {
			*v.target = s
			return nil
		}
	}
	return fmt.Errorf("must be one of %v (got %q)", v.allowed(), s)
}

// addRegistryFlag declares an enum-style flag whose valid values come from
// allowed(). The flag description is augmented with the list of options so
// the user sees them in --help; shell completion suggests them too.
func addRegistryFlag(cmd *cobra.Command, target *string, name, def, baseHelp string, allowed func() []string) {
	*target = def
	help := fmt.Sprintf("%s (one of: %s)", baseHelp, strings.Join(allowed(), ", "))
	cmd.Flags().Var(&registryValue{target: target, allowed: allowed}, name, help)
	_ = cmd.RegisterFlagCompletionFunc(name, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return allowed(), cobra.ShellCompDirectiveNoFileComp
	})
}
