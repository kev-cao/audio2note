package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"audio2note/internal/plaud"
)

const plaudSetupInstructions = `To get your Plaud bearer token:
  1. Open https://web.plaud.ai in your browser and sign in.
  2. Open the browser dev tools and inspect localStorage for the site.
  3. Copy the value of the key "pid_tokenstr".

`

func newPlaudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plaud",
		Short: "Plaud-related commands",
	}
	cmd.AddCommand(newPlaudAuthCmd())
	return cmd
}

func newPlaudAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Plaud authentication",
	}
	cmd.AddCommand(newPlaudAuthSetupCmd(), newPlaudAuthValidateCmd())
	return cmd
}

func newPlaudAuthSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "setup",
		Short:         "Prompt for a Plaud bearer token and save it to ~/.plaud/token",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Print(plaudSetupInstructions)
			fmt.Print("Bearer token: ")
			token, err := readLine(os.Stdin)
			if err != nil {
				return fmt.Errorf("read token: %w", err)
			}
			if err := plaud.SaveToken(token); err != nil {
				return err
			}
			path, _ := plaud.TokenPath()
			fmt.Printf("✓ saved token to %s\n", path)
			return nil
		},
	}
}

func newPlaudAuthValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "validate",
		Short:         "Check that the saved Plaud bearer token is still valid",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token, err := plaud.LoadToken()
			if err != nil {
				if errors.Is(err, plaud.ErrMissing) {
					return errors.New("missing bearer token, please run `audio2note plaud auth setup`")
				}
				return err
			}
			if err := plaud.Validate(token); err != nil {
				return errors.New("bearer token expired, please run `audio2note plaud auth setup`")
			}
			fmt.Println("token verified!")
			return nil
		},
	}
}

func readLine(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", errors.New("no input")
	}
	return strings.TrimSpace(sc.Text()), nil
}
