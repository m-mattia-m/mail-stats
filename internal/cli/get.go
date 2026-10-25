package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
	"github.com/m-mattia-m/mail-stats/internal/stats"
)

const dateLayout = "2006-01-02"

func newGetCommand(d Deps, opts *globalOptions) *cobra.Command {
	get := &cobra.Command{
		Use:   "get",
		Short: "Display mailbox statistics or folders",
	}
	get.AddCommand(newGetStatsCommand(d, opts), newGetFoldersCommand(d, opts))
	return get
}

// statsOutput is the json/yaml shape of `get stats`.
type statsOutput struct {
	Context string           `json:"context" yaml:"context"`
	Stats   stats.Report     `json:"stats" yaml:"stats"`
	Folders []mailbox.Folder `json:"folders" yaml:"folders"`
}

func newGetStatsCommand(d Deps, opts *globalOptions) *cobra.Command {
	var since, until string
	cmd := &cobra.Command{
		Use:     "stats",
		Aliases: []string{"stat", "statistics"},
		Short:   "Show average sent and received mails per day, week and month",
		Example: `  mail-stats get stats
  mail-stats get stats --context work --since 2024-01-01 -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(opts.output); err != nil {
				return err
			}
			statOpts := stats.Options{Now: d.Now()}
			var err error
			if statOpts.Since, err = parseDate("since", since); err != nil {
				return err
			}
			if statOpts.Until, err = parseDate("until", until); err != nil {
				return err
			}
			if !statOpts.Until.IsZero() {
				statOpts.Until = stats.EndOfDay(statOpts.Until)
			}

			ctx, col, err := collect(d, opts)
			if err != nil {
				return err
			}
			report := stats.Compute(col.Sent, col.Received, statOpts)

			w := cmd.OutOrStdout()
			if opts.output != outputTable {
				return printStructured(w, opts.output, statsOutput{Context: ctx.Name, Stats: report, Folders: col.Folders})
			}
			return printStatsTable(w, ctx, report)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only count mails on or after this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "only count mails on or before this date (YYYY-MM-DD)")
	return cmd
}

func newGetFoldersCommand(d Deps, opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "folders",
		Aliases: []string{"folder", "mailboxes"},
		Short:   "List all folders, how they are counted and how many mails they hold",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(opts.output); err != nil {
				return err
			}
			_, col, err := collect(d, opts)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if opts.output != outputTable {
				return printStructured(w, opts.output, col.Folders)
			}
			tw := newTable(w)
			fmt.Fprintln(tw, "NAME\tROLE\tMESSAGES")
			for _, f := range col.Folders {
				messages := fmt.Sprint(f.Messages)
				if f.Role == mailbox.RoleExcluded {
					messages = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Name, f.Role, messages)
			}
			return tw.Flush()
		},
	}
}

// collect resolves the context, connects and reads every folder.
func collect(d Deps, opts *globalOptions) (*config.Context, *mailbox.Collection, error) {
	cfg, _, err := opts.loadConfig()
	if err != nil {
		return nil, nil, err
	}
	ctx, err := cfg.Resolve(opts.contextName)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Validate(); err != nil {
		return nil, nil, err
	}
	password, err := ctx.ResolvePassword(d.Getenv, d.RunCommand)
	if err != nil {
		return nil, nil, err
	}

	sess, err := d.Dial(mailbox.Account{
		Host:               ctx.Host,
		Port:               ctx.EffectivePort(),
		Username:           ctx.Username,
		Password:           password,
		Security:           ctx.Security,
		InsecureSkipVerify: ctx.InsecureSkipVerify,
		Timeout:            opts.timeout,
	})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = sess.Close() }()

	col, err := mailbox.Collect(sess, mailbox.Classifier{SentFolders: ctx.SentFolders, ExcludeFolders: ctx.ExcludeFolders})
	if err != nil {
		return nil, nil, err
	}
	return ctx, col, nil
}

func parseDate(flag, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation(dateLayout, value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --%s %q: want YYYY-MM-DD", flag, value)
	}
	return t, nil
}

func printStatsTable(w io.Writer, ctx *config.Context, r stats.Report) error {
	tw := newTable(w)
	fmt.Fprintf(tw, "CONTEXT:\t%s (%s @ %s)\n", ctx.Name, ctx.Username, ctx.Host)
	fmt.Fprintf(tw, "PERIOD:\t%s – %s (%.0f days)\n\n", r.From.Format(dateLayout), r.To.Format(dateLayout), r.Days)
	if err := tw.Flush(); err != nil {
		return err
	}

	tw = newTable(w)
	fmt.Fprintln(tw, "DIRECTION\tTOTAL\tPER DAY\tPER WEEK\tPER MONTH\tFIRST\tLAST")
	for _, row := range []struct {
		name string
		s    stats.Summary
	}{{"sent", r.Sent}, {"received", r.Received}} {
		fmt.Fprintf(tw, "%s\t%d\t%.2f\t%.2f\t%.2f\t%s\t%s\n",
			row.name, row.s.Total, row.s.PerDay, row.s.PerWeek, row.s.PerMonth, formatDate(row.s.First), formatDate(row.s.Last))
	}
	return tw.Flush()
}

func formatDate(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(dateLayout)
}
