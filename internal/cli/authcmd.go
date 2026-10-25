package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/oauth"
)

func newAuthCommand(d Deps, opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to OAuth2 accounts (Microsoft: Outlook.com, Hotmail, Microsoft 365)",
	}
	cmd.AddCommand(newAuthLoginCommand(d, opts), newAuthLogoutCommand(d, opts), newAuthStatusCommand(d, opts))
	return cmd
}

// oauthContext resolves the context named in args, or --context, or the
// current context, and checks that it uses OAuth2.
func oauthContext(opts *globalOptions, args []string) (*config.Context, string, error) {
	cfg, path, err := opts.loadConfig()
	if err != nil {
		return nil, "", err
	}
	name := opts.contextName
	if len(args) == 1 {
		name = args[0]
	}
	ctx, err := cfg.Resolve(name)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Validate(); err != nil {
		return nil, "", err
	}
	if !ctx.UsesOAuth2() {
		return nil, "", fmt.Errorf("context %q uses password login; set `--auth oauth2` with `mail-stats config set-context %s` first", ctx.Name, ctx.Name)
	}
	return ctx, path, nil
}

func newAuthLoginCommand(d Deps, opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "login [CONTEXT]",
		Short: "Sign in with the device-code flow and store the token",
		Long: `Sign in to an OAuth2 context (default: the current context).

mail-stats prints a URL and a code. Open the URL in any browser, enter the
code and sign in with the mail account. The refresh token is stored next to
the config file (tokens/<context>.json, mode 0600) and renewed automatically;
you only need to log in again if it is revoked or unused for a long time.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, path, err := oauthContext(opts, args)
			if err != nil {
				return err
			}
			m := oauthManager(d, path)
			if err := m.Login(cmd.Context(), ctx, cmd.OutOrStdout()); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to context %q. Token stored in %s.\n", ctx.Name, m.Path(ctx.Name))
			return err
		},
	}
}

func newAuthLogoutCommand(d Deps, opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout [CONTEXT]",
		Short: "Delete the stored token of a context",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, path, err := oauthContext(opts, args)
			if err != nil {
				return err
			}
			if err := oauthManager(d, path).Logout(ctx.Name); err != nil {
				if errors.Is(err, oauth.ErrNotLoggedIn) {
					return fmt.Errorf("context %q is not logged in", ctx.Name)
				}
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged out of context %q.\n", ctx.Name)
			return err
		},
	}
}

func newAuthStatusCommand(d Deps, opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the login state of every OAuth2 context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, path, err := opts.loadConfig()
			if err != nil {
				return err
			}
			m := oauthManager(d, path)
			tw := newTable(cmd.OutOrStdout())
			fmt.Fprintln(tw, "NAME\tUSERNAME\tTOKENS FROM\tSTATUS")
			for i := range cfg.Contexts {
				c := &cfg.Contexts[i]
				if !c.UsesOAuth2() {
					continue
				}
				source, status := "auth login", "logged in"
				switch _, err := m.Load(c.Name); {
				case c.OAuth2.TokenCommand != "":
					source, status = "tokenCommand", "-"
				case errors.Is(err, oauth.ErrNotLoggedIn):
					status = "not logged in"
				case err != nil:
					status = "error: " + err.Error()
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Name, c.Username, source, status)
			}
			return tw.Flush()
		},
	}
}
