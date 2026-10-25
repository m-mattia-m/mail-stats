package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/m-mattia-m/mail-stats/internal/config"
)

func newConfigCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the contexts (IMAP accounts) in the config file",
	}
	cmd.AddCommand(
		newConfigViewCommand(opts),
		newGetContextsCommand(opts),
		newCurrentContextCommand(opts),
		newUseContextCommand(opts),
		newSetContextCommand(opts),
		newDeleteContextCommand(opts),
	)
	return cmd
}

func newConfigViewCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Print the config file with plain-text passwords redacted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := opts.loadConfig()
			if err != nil {
				return err
			}
			format := opts.output
			if format == outputTable {
				format = outputYAML
			}
			if err := validateOutput(format); err != nil {
				return err
			}
			return printStructured(cmd.OutOrStdout(), format, cfg.Redacted())
		},
	}
}

func newGetContextsCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get-contexts",
		Short: "List all contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := opts.loadConfig()
			if err != nil {
				return err
			}
			tw := newTable(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CURRENT\tNAME\tHOST\tPORT\tUSERNAME\tSECURITY")
			for _, c := range cfg.Contexts {
				current := ""
				if c.Name == cfg.CurrentContext {
					current = "*"
				}
				security := c.Security
				if security == "" {
					security = "tls"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", current, c.Name, c.Host, c.EffectivePort(), c.Username, security)
			}
			return tw.Flush()
		},
	}
}

func newCurrentContextCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "current-context",
		Short: "Print the current context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := opts.loadConfig()
			if err != nil {
				return err
			}
			if cfg.CurrentContext == "" {
				return errors.New("current-context is not set")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), cfg.CurrentContext)
			return err
		},
	}
}

func newUseContextCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "use-context NAME",
		Short: "Set the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := opts.loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Get(args[0]); !ok {
				return fmt.Errorf("context %q not found", args[0])
			}
			cfg.CurrentContext = args[0]
			if err := cfg.Save(path); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q.\n", args[0])
			return err
		},
	}
}

func newSetContextCommand(opts *globalOptions) *cobra.Command {
	var c config.Context
	cmd := &cobra.Command{
		Use:   "set-context NAME",
		Short: "Create a context or update the given fields of an existing one",
		Example: `  mail-stats config set-context personal --host imap.example.com --username me@example.com --password-env MAIL_PASSWORD
  mail-stats config set-context work --password-command "op read op://Work/imap/password"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := opts.loadConfig()
			if err != nil {
				return err
			}

			ctx := config.Context{Name: args[0]}
			created := true
			if existing, ok := cfg.Get(args[0]); ok {
				ctx, created = *existing, false
			}
			flags := cmd.Flags()
			if flags.Changed("host") {
				ctx.Host = c.Host
			}
			if flags.Changed("port") {
				ctx.Port = c.Port
			}
			if flags.Changed("username") {
				ctx.Username = c.Username
			}
			if flags.Changed("security") {
				ctx.Security = c.Security
			}
			if flags.Changed("password-env") {
				ctx.PasswordEnv = c.PasswordEnv
			}
			if flags.Changed("password-command") {
				ctx.PasswordCommand = c.PasswordCommand
			}
			if flags.Changed("insecure-skip-verify") {
				ctx.InsecureSkipVerify = c.InsecureSkipVerify
			}
			if flags.Changed("sent-folder") {
				ctx.SentFolders = c.SentFolders
			}
			if flags.Changed("exclude-folder") {
				ctx.ExcludeFolders = c.ExcludeFolders
			}
			if err := ctx.Validate(); err != nil {
				return err
			}

			cfg.Upsert(ctx)
			if cfg.CurrentContext == "" {
				cfg.CurrentContext = ctx.Name
			}
			if err := cfg.Save(path); err != nil {
				return err
			}
			verb := "modified"
			if created {
				verb = "created"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Context %q %s.\n", ctx.Name, verb)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&c.Host, "host", "", "IMAP server host name")
	f.IntVar(&c.Port, "port", 0, "IMAP server port (default 993 for tls, 143 for starttls/none)")
	f.StringVar(&c.Username, "username", "", "IMAP login name")
	f.StringVar(&c.Security, "security", "", "connection security: tls, starttls or none (default tls)")
	f.StringVar(&c.PasswordEnv, "password-env", "", "environment variable that holds the password")
	f.StringVar(&c.PasswordCommand, "password-command", "", "shell command that prints the password")
	f.BoolVar(&c.InsecureSkipVerify, "insecure-skip-verify", false, "do not verify the server's TLS certificate")
	f.StringSliceVar(&c.SentFolders, "sent-folder", nil, "folder to count as sent, in addition to the detected ones (repeatable)")
	f.StringSliceVar(&c.ExcludeFolders, "exclude-folder", nil, "folder to leave out of the stats (repeatable)")
	return cmd
}

func newDeleteContextCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete-context NAME",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := opts.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Delete(args[0]); err != nil {
				return err
			}
			if err := cfg.Save(path); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted context %q.\n", args[0])
			return err
		},
	}
}
