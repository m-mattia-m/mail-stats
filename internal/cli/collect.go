package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
	"github.com/m-mattia-m/mail-stats/internal/oauth"
)

// selection holds the flags that pick several contexts for one command.
type selection struct {
	contexts []string
	all      bool
}

func (s *selection) register(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&s.contexts, "contexts", nil, "contexts to read, comma-separated or repeated (instead of --context)")
	cmd.Flags().BoolVarP(&s.all, "all-contexts", "A", false, "read every context in the config file")
	cmd.MarkFlagsMutuallyExclusive("contexts", "all-contexts")
}

// multi reports whether more than the single --context/current context was
// asked for. It decides the output shape, even if only one context matches.
func (s *selection) multi() bool {
	return s.all || len(s.contexts) > 0
}

// resolve returns the selected contexts in config (for -A) or flag order,
// without duplicates.
func (s *selection) resolve(cfg *config.Config, opts *globalOptions) ([]*config.Context, error) {
	if !s.multi() {
		ctx, err := cfg.Resolve(opts.contextName)
		if err != nil {
			return nil, err
		}
		return []*config.Context{ctx}, nil
	}
	if opts.contextName != "" {
		return nil, errors.New("--context cannot be combined with --contexts or --all-contexts")
	}

	names := s.contexts
	if s.all {
		if len(cfg.Contexts) == 0 {
			return nil, errors.New("no contexts configured: create one with `mail-stats config set-context <name>`")
		}
		names = nil
		for _, c := range cfg.Contexts {
			names = append(names, c.Name)
		}
	}

	var ctxs []*config.Context
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		ctx, ok := cfg.Get(name)
		if !ok {
			return nil, fmt.Errorf("context %q not found", name)
		}
		ctxs = append(ctxs, ctx)
	}
	return ctxs, nil
}

// contextResult is one context's mailbox contents.
type contextResult struct {
	ctx *config.Context
	col *mailbox.Collection
}

// collectSelected resolves the selected contexts and reads all of them.
func collectSelected(d Deps, opts *globalOptions, sel *selection) ([]contextResult, error) {
	cfg, path, err := opts.loadConfig()
	if err != nil {
		return nil, err
	}
	ctxs, err := sel.resolve(cfg, opts)
	if err != nil {
		return nil, err
	}
	warnDuplicateAccounts(d, ctxs)
	return collectContexts(d, opts, ctxs, oauthManager(d, path))
}

// collectContexts reads every context. Passwords and access tokens are
// resolved one after the other first, so a password manager prompts at most
// once at a time; the mailboxes are then read in parallel. The first failing
// context (in order) fails the whole command.
func collectContexts(d Deps, opts *globalOptions, ctxs []*config.Context, tokens *oauth.Manager) ([]contextResult, error) {
	accounts := make([]mailbox.Account, len(ctxs))
	for i, ctx := range ctxs {
		if err := ctx.Validate(); err != nil {
			return nil, err
		}
		var password, accessToken string
		var err error
		if ctx.UsesOAuth2() {
			tokenCtx, cancel := context.WithTimeout(context.Background(), opts.timeout)
			accessToken, err = tokens.AccessToken(tokenCtx, ctx)
			cancel()
		} else {
			password, err = ctx.ResolvePassword(d.Getenv, d.RunCommand)
		}
		if err != nil {
			return nil, err
		}
		accounts[i] = mailbox.Account{
			Host:               ctx.Host,
			Port:               ctx.EffectivePort(),
			Username:           ctx.Username,
			Password:           password,
			AccessToken:        accessToken,
			Security:           ctx.Security,
			InsecureSkipVerify: ctx.InsecureSkipVerify,
			Timeout:            opts.timeout,
		}
	}

	results := make([]contextResult, len(ctxs))
	errs := make([]error, len(ctxs))
	var wg sync.WaitGroup
	for i := range ctxs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			col, err := collectAccount(d, accounts[i], ctxs[i])
			results[i], errs[i] = contextResult{ctx: ctxs[i], col: col}, err
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			if len(ctxs) > 1 {
				return nil, fmt.Errorf("context %q: %w", ctxs[i].Name, err)
			}
			return nil, err
		}
	}
	return results, nil
}

func collectAccount(d Deps, acc mailbox.Account, ctx *config.Context) (*mailbox.Collection, error) {
	sess, err := d.Dial(acc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.Close() }()
	return mailbox.Collect(sess, mailbox.Classifier{SentFolders: ctx.SentFolders, ExcludeFolders: ctx.ExcludeFolders})
}

// warnDuplicateAccounts tells the user when two selected contexts log in to
// the same mailbox, because its mails would be counted twice in the combined
// numbers.
func warnDuplicateAccounts(d Deps, ctxs []*config.Context) {
	first := map[string]string{}
	for _, ctx := range ctxs {
		key := strings.ToLower(ctx.Username) + "@" + strings.ToLower(ctx.Host) + fmt.Sprintf(":%d", ctx.EffectivePort())
		if other, ok := first[key]; ok {
			fmt.Fprintf(d.Err, "Warning: contexts %q and %q use the same account (%s @ %s); its mails are counted twice in the combined numbers.\n",
				other, ctx.Name, ctx.Username, ctx.Host)
			continue
		}
		first[key] = ctx.Name
	}
}
