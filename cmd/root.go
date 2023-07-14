package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/cli/v2/pkg/findsh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	cobracompletefig "github.com/withfig/autocomplete-tools/integrations/cobra"

	"github.com/pomdtr/sunbeam/internal"
	"github.com/pomdtr/sunbeam/types"
	"github.com/pomdtr/sunbeam/utils"
)

const (
	coreGroupID   = "core"
	customGroupID = "custom"
)

var (
	Version = "dev"
	Date    = "unknown"
)

var (
	options    internal.SunbeamOptions
	cwd        string
	cwdCommand *Command
)

func init() {
	options = internal.SunbeamOptions{
		MaxHeight:  utils.LookupIntEnv("SUNBEAM_HEIGHT", 0),
		MaxWidth:   utils.LookupIntEnv("SUNBEAM_WIDTH", 0),
		FullScreen: utils.LookupBoolEnv("SUNBEAM_FULLSCREEN", true),
		Border:     utils.LookupBoolEnv("SUNBEAM_BORDER", false),
		Margin:     utils.LookupIntEnv("SUNBEAM_MARGIN", 0),
	}

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cwd = wd

	c, err := ParseCommand(cwd, "")
	if err != nil {
		return
	}

	cwdCommand = &c
}

func NewRootCmd() *cobra.Command {
	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:          "sunbeam",
		Short:        "Command Line Launcher",
		Version:      fmt.Sprintf("%s (%s)", Version, Date),
		SilenceUsage: true,
		Long: `Sunbeam is a command line launcher for your terminal, inspired by fzf and raycast.

See https://pomdtr.github.io/sunbeam for more information.`,
	}

	rootCmd.AddGroup(
		&cobra.Group{ID: coreGroupID, Title: "Core Commands"},
		&cobra.Group{ID: customGroupID, Title: "Custom Commands"},
	)
	rootCmd.AddCommand(NewQueryCmd())
	rootCmd.AddCommand(NewCommandCmd())
	rootCmd.AddCommand(NewFetchCmd())
	rootCmd.AddCommand(NewListCmd())
	rootCmd.AddCommand(NewReadCmd())
	rootCmd.AddCommand(NewTriggerCmd())
	rootCmd.AddCommand(NewValidateCmd())
	rootCmd.AddCommand(NewDetailCmd())
	rootCmd.AddCommand(NewRunCmd())
	rootCmd.AddCommand(NewExecCmd())
	rootCmd.AddCommand(NewEvalCmd())

	rootCmd.AddCommand(cobracompletefig.CreateCompletionSpecCommand())
	docCmd := &cobra.Command{
		Use:    "docs",
		Short:  "Generate documentation for sunbeam",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := buildDoc(rootCmd)
			if err != nil {
				return err
			}

			fmt.Println(doc)
			return nil
		},
	}
	rootCmd.AddCommand(docCmd)

	for name, command := range commands {
		rootCmd.AddCommand(NewCustomCmd(name, command))
	}

	manCmd := &cobra.Command{
		Use:    "generate-man-pages [path]",
		Short:  "Generate Man Pages for sunbeam",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			header := &doc.GenManHeader{
				Title:   "MINE",
				Section: "3",
			}
			err := doc.GenManTree(rootCmd, header, args[0])
			if err != nil {
				return err
			}

			return nil
		},
	}
	rootCmd.AddCommand(manCmd)

	return rootCmd

}

func runCommand(commandBin string, args []string, input string) error {
	var command types.Command
	if runtime.GOOS != "windows" {
		if err := os.Chmod(commandBin, 0755); err != nil {
			return err
		}

		command = types.Command{
			Name: commandBin,
			Args: args,
		}
		return Run(internal.NewCommandGenerator(&command))
	}

	shExe, err := findsh.Find()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("the `sh.exe` interpreter is required. Please install Git for Windows and try again")
		}
		return err
	}
	forwardArgs := append([]string{"-c", `command "$@"`, "--", commandBin}, args...)

	command = types.Command{
		Name: shExe,
		Args: forwardArgs,
	}

	return Run(internal.NewCommandGenerator(&command))
}

func Run(generator internal.PageGenerator) error {
	if !isatty.IsTerminal(os.Stderr.Fd()) {
		output, err := generator()
		if err != nil {
			return fmt.Errorf("could not generate page: %s", err)
		}

		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
			return fmt.Errorf("could not encode page: %s", err)
		}

		return nil
	}

	runner := internal.NewRunner(generator)
	err := internal.Draw(runner, options)
	if errors.Is(err, internal.ErrInterrupted) && isatty.IsTerminal(os.Stdout.Fd()) {
		return nil
	}

	return err
}

func buildDoc(command *cobra.Command) (string, error) {
	if command.GroupID == customGroupID {
		return "", nil
	}

	var page strings.Builder
	err := doc.GenMarkdown(command, &page)
	if err != nil {
		return "", err
	}

	out := strings.Builder{}
	for _, line := range strings.Split(page.String(), "\n") {
		if strings.Contains(line, "SEE ALSO") {
			break
		}

		out.WriteString(line + "\n")
	}

	for _, child := range command.Commands() {
		childPage, err := buildDoc(child)
		if err != nil {
			return "", err
		}
		out.WriteString(childPage)
	}

	return out.String(), nil
}

func NewCustomCmd(commandName string, manifest Manifest) *cobra.Command {
	cmd := &cobra.Command{
		Use:                commandName,
		Short:              manifest.Title,
		Long:               manifest.Description,
		DisableFlagParsing: true,
		GroupID:            customGroupID,
	}

	if len(manifest.SubCommands) == 0 {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] == "--help" {
				return cmd.Help()
			}
			var input string
			if !isatty.IsTerminal(os.Stdin.Fd()) {
				inputBytes, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}

				input = string(inputBytes)
			}

			return runCommand(filepath.Join(commandRoot, commandName, manifest.Entrypoint), args, input)
		}

		return cmd
	}

	for name, subCommand := range manifest.SubCommands {
		subCommand := subCommand
		cmd.AddCommand(&cobra.Command{
			Use:                name,
			Short:              subCommand.Title,
			Long:               subCommand.Description,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 1 && args[0] == "--help" {
					return cmd.Help()
				}
				var input string
				if !isatty.IsTerminal(os.Stdin.Fd()) {
					inputBytes, err := io.ReadAll(os.Stdin)
					if err != nil {
						return err
					}

					input = string(inputBytes)
				}

				return runCommand(filepath.Join(commandRoot, commandName, subCommand.Entrypoint), args, input)

			},
		})
	}

	return cmd
}
