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
			fmt.Fprintln(tw, "CURRENT\tNAME\tHOST\tPORT\tUSERNAME\tSECURITY\tAUTH")
			for _, c := range cfg.Contexts {
				current := ""
				if c.Name == cfg.CurrentContext {
					current = "*"
				}
				security := c.Security
				if security == "" {
					security = "tls"
				}
				auth := c.Auth
				if auth == "" {
					auth = config.AuthPassword
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", current, c.Name, c.Host, c.EffectivePort(), c.Username, security, auth)
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
  mail-stats config set-context work --password-command "op read op://Work/imap/password"
  mail-stats config set-context hotmail --host outlook.office365.com --username me@hotmail.com --auth oauth2 --oauth2-client-id <app-id>`,
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
			if flags.Changed("auth") {
				ctx.Auth = c.Auth
			}
			if flags.Changed("oauth2-client-id") {
				ctx.OAuth2.ClientID = c.OAuth2.ClientID
			}
			if flags.Changed("oauth2-tenant") {
				ctx.OAuth2.Tenant = c.OAuth2.Tenant
			}
			if flags.Changed("oauth2-scope") {
				ctx.OAuth2.Scopes = c.OAuth2.Scopes
			}
			if flags.Changed("oauth2-token-command") {
				ctx.OAuth2.TokenCommand = c.OAuth2.TokenCommand
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
	f.StringVar(&c.Auth, "auth", "", "login method: password (default) or oauth2 (Microsoft: Outlook.com, Hotmail, Microsoft 365)")
	f.StringVar(&c.OAuth2.ClientID, "oauth2-client-id", "", "OAuth2: Application (client) ID of your Microsoft app registration")
	f.StringVar(&c.OAuth2.Tenant, "oauth2-tenant", "", "OAuth2: consumers (default, personal accounts), organizations, common or a tenant ID")
	f.StringSliceVar(&c.OAuth2.Scopes, "oauth2-scope", nil, "OAuth2: scopes to request (default IMAP.AccessAsUser.All + offline_access)")
	f.StringVar(&c.OAuth2.TokenCommand, "oauth2-token-command", "", "OAuth2: shell command that prints an access token (instead of `mail-stats auth login`)")
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
