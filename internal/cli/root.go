// Package cli wires the kubectl-style command tree:
//
//	mail-stats get stats|folders
//	mail-stats config view|get-contexts|current-context|use-context|set-context|delete-context
//	mail-stats version
package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
)

// Session is an open mailbox connection.
type Session interface {
	mailbox.Source
	Close() error
}

// Deps are the side effects the commands depend on, injectable for tests.
type Deps struct {
	Out        io.Writer
	Err        io.Writer
	Getenv     config.Getenv
	RunCommand config.RunCommand
	Now        func() time.Time
	Dial       func(mailbox.Account) (Session, error)
	Version    string
}

// DefaultDeps returns the dependencies of the real binary.
func DefaultDeps(version string) Deps {
	return Deps{
		Out:        os.Stdout,
		Err:        os.Stderr,
		Getenv:     os.Getenv,
		RunCommand: config.ShellCommand,
		Now:        time.Now,
		Dial: func(acc mailbox.Account) (Session, error) {
			return mailbox.Dial(acc)
		},
		Version: version,
	}
}

// globalOptions are the persistent flags shared by all commands.
type globalOptions struct {
	configPath  string
	contextName string
	output      string
	timeout     time.Duration
}

// Execute runs the CLI with os.Args and returns the process exit code.
func Execute(version string) int {
	return Run(DefaultDeps(version), os.Args[1:])
}

// Run runs the CLI with args and returns the process exit code.
func Run(d Deps, args []string) int {
	cmd := NewRootCommand(d)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(d.Err, "Error:", err)
		return 1
	}
	return 0
}

// NewRootCommand builds the command tree.
func NewRootCommand(d Deps) *cobra.Command {
	opts := &globalOptions{}
	root := &cobra.Command{
		Use:           "mail-stats",
		Short:         "Show statistics about the mails in your IMAP mailbox",
		Long:          "mail-stats reads every folder of an IMAP account and shows how many mails you send and receive per day, week and month over the whole history.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(d.Out)
	root.SetErr(d.Err)

	pf := root.PersistentFlags()
	pf.StringVar(&opts.configPath, "config", "", "path to the config file (default $MAIL_STATS_CONFIG or ~/.mail-stats/config.yaml)")
	pf.StringVar(&opts.contextName, "context", "", "context to use instead of the current one")
	pf.StringVarP(&opts.output, "output", "o", "table", "output format: table, json or yaml")
	pf.DurationVar(&opts.timeout, "timeout", 30*time.Second, "IMAP connection and command timeout")

	root.AddCommand(newGetCommand(d, opts), newConfigCommand(opts), newVersionCommand(d))
	return root
}

func (o *globalOptions) resolveConfigPath() (string, error) {
	if o.configPath != "" {
		return o.configPath, nil
	}
	return config.DefaultPath()
}

func (o *globalOptions) loadConfig() (*config.Config, string, error) {
	path, err := o.resolveConfigPath()
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
}

func newVersionCommand(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), d.Version)
			return err
		},
	}
}
