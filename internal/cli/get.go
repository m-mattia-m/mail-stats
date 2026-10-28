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

// combinedName labels the row that adds up all selected contexts. The
// parentheses keep it apart from real context names in the table.
const combinedName = "(combined)"

func newGetCommand(d Deps, opts *globalOptions) *cobra.Command {
	get := &cobra.Command{
		Use:   "get",
		Short: "Display mailbox statistics or folders",
	}
	get.AddCommand(newGetStatsCommand(d, opts), newGetFoldersCommand(d, opts))
	return get
}

// statsOutput is the json/yaml shape of `get stats` for one context.
type statsOutput struct {
	Context string           `json:"context" yaml:"context"`
	Stats   stats.Report     `json:"stats" yaml:"stats"`
	Folders []mailbox.Folder `json:"folders" yaml:"folders"`
}

// multiStatsOutput is the json/yaml shape of `get stats` with --contexts or
// --all-contexts.
type multiStatsOutput struct {
	Contexts []statsOutput `json:"contexts" yaml:"contexts"`
	Combined stats.Report  `json:"combined" yaml:"combined"`
}

// foldersOutput is the json/yaml shape of one context in a multi-context
// `get folders`.
type foldersOutput struct {
	Context string           `json:"context" yaml:"context"`
	Folders []mailbox.Folder `json:"folders" yaml:"folders"`
}

func newGetStatsCommand(d Deps, opts *globalOptions) *cobra.Command {
	var since, until string
	sel := &selection{}
	cmd := &cobra.Command{
		Use:     "stats",
		Aliases: []string{"stat", "statistics"},
		Short:   "Show average sent and received mails per day, week and month",
		Long: `Show average sent and received mails per day, week and month.

With --contexts or --all-contexts several accounts are read in parallel. Each
account is averaged over its own history, and a combined row adds all of them
up over the window from the oldest mail of any account.`,
		Example: `  mail-stats get stats
  mail-stats get stats --context work --since 2024-01-01 -o json
  mail-stats get stats --contexts personal,work
  mail-stats get stats -A`,
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

			results, err := collectSelected(d, opts, sel)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if !sel.multi() {
				r := results[0]
				report := stats.Compute(r.col.Sent, r.col.Received, statOpts)
				if opts.output != outputTable {
					return printStructured(w, opts.output, statsOutput{Context: r.ctx.Name, Stats: report, Folders: r.col.Folders})
				}
				return printStatsTable(w, r.ctx, report)
			}

			out := multiStatsOutput{}
			var allSent, allReceived []time.Time
			for _, r := range results {
				report := stats.Compute(r.col.Sent, r.col.Received, statOpts)
				out.Contexts = append(out.Contexts, statsOutput{Context: r.ctx.Name, Stats: report, Folders: r.col.Folders})
				allSent = append(allSent, r.col.Sent...)
				allReceived = append(allReceived, r.col.Received...)
			}
			out.Combined = stats.Compute(allSent, allReceived, statOpts)
			if opts.output != outputTable {
				return printStructured(w, opts.output, out)
			}
			return printMultiStatsTable(w, results, out)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only count mails on or after this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "only count mails on or before this date (YYYY-MM-DD)")
	sel.register(cmd)
	return cmd
}

func newGetFoldersCommand(d Deps, opts *globalOptions) *cobra.Command {
	sel := &selection{}
	cmd := &cobra.Command{
		Use:     "folders",
		Aliases: []string{"folder", "mailboxes"},
		Short:   "List all folders, how they are counted and how many mails they hold",
		Example: `  mail-stats get folders
  mail-stats get folders --contexts personal,work
  mail-stats get folders -A -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(opts.output); err != nil {
				return err
			}
			results, err := collectSelected(d, opts, sel)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if opts.output != outputTable {
				if !sel.multi() {
					return printStructured(w, opts.output, results[0].col.Folders)
				}
				var out []foldersOutput
				for _, r := range results {
					out = append(out, foldersOutput{Context: r.ctx.Name, Folders: r.col.Folders})
				}
				return printStructured(w, opts.output, out)
			}

			tw := newTable(w)
			if sel.multi() {
				fmt.Fprint(tw, "CONTEXT\t")
			}
			fmt.Fprintln(tw, "NAME\tROLE\tMESSAGES")
			for _, r := range results {
				for _, f := range r.col.Folders {
					if sel.multi() {
						fmt.Fprintf(tw, "%s\t", r.ctx.Name)
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Name, f.Role, folderMessages(f))
				}
			}
			return tw.Flush()
		},
	}
	sel.register(cmd)
	return cmd
}

func folderMessages(f mailbox.Folder) string {
	if f.Role == mailbox.RoleExcluded {
		return "-"
	}
	return fmt.Sprint(f.Messages)
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
	fmt.Fprintf(tw, "CONTEXT:\t%s (%s)\n", ctx.Name, accountLabel(ctx))
	fmt.Fprintf(tw, "PERIOD:\t%s\n", periodLabel(r))
	fmt.Fprintf(tw, "BUSIEST DAY:\t%s\n\n", busiestDayLabel(r.BusiestDay))
	if err := tw.Flush(); err != nil {
		return err
	}

	tw = newTable(w)
	fmt.Fprintln(tw, "DIRECTION\tTOTAL\tPER DAY\tPER WEEK\tPER MONTH\tFIRST\tLAST\tBUSIEST DAY")
	writeSummaryRows(tw, "", r)
	return tw.Flush()
}

func printMultiStatsTable(w io.Writer, results []contextResult, out multiStatsOutput) error {
	tw := newTable(w)
	fmt.Fprintln(tw, "CONTEXT\tACCOUNT\tPERIOD\tBUSIEST DAY")
	for i, r := range results {
		report := out.Contexts[i].Stats
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ctx.Name, accountLabel(r.ctx), periodLabel(report), busiestDayLabel(report.BusiestDay))
	}
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n\n", combinedName, fmt.Sprintf("%d accounts", len(results)), periodLabel(out.Combined), busiestDayLabel(out.Combined.BusiestDay))
	if err := tw.Flush(); err != nil {
		return err
	}

	tw = newTable(w)
	fmt.Fprintln(tw, "CONTEXT\tDIRECTION\tTOTAL\tPER DAY\tPER WEEK\tPER MONTH\tFIRST\tLAST\tBUSIEST DAY")
	for _, c := range out.Contexts {
		writeSummaryRows(tw, c.Context+"\t", c.Stats)
	}
	writeSummaryRows(tw, combinedName+"\t", out.Combined)
	return tw.Flush()
}

// writeSummaryRows writes the sent and received rows, each prefixed with
// prefix (a leading column, or "").
func writeSummaryRows(w io.Writer, prefix string, r stats.Report) {
	for _, row := range []struct {
		name string
		s    stats.Summary
	}{{"sent", r.Sent}, {"received", r.Received}} {
		fmt.Fprintf(w, "%s%s\t%d\t%.2f\t%.2f\t%.2f\t%s\t%s\t%s\n",
			prefix, row.name, row.s.Total, row.s.PerDay, row.s.PerWeek, row.s.PerMonth, formatDate(row.s.First), formatDate(row.s.Last), dayLabel(row.s.BusiestDay))
	}
}

func accountLabel(ctx *config.Context) string {
	return ctx.Username + " @ " + ctx.Host
}

func periodLabel(r stats.Report) string {
	return fmt.Sprintf("%s – %s (%.0f days)", r.From.Format(dateLayout), r.To.Format(dateLayout), r.Days)
}

// busiestDayLabel renders e.g. "2024-01-15 (14 mails: 3 sent, 11 received)".
func busiestDayLabel(d *stats.BusiestDay) string {
	if d == nil {
		return "-"
	}
	return fmt.Sprintf("%s (%d mails: %d sent, %d received)", d.Date, d.Total, d.Sent, d.Received)
}

// dayLabel renders e.g. "2024-01-15 (11)".
func dayLabel(d *stats.Day) string {
	if d == nil {
		return "-"
	}
	return fmt.Sprintf("%s (%d)", d.Date, d.Count)
}

func formatDate(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(dateLayout)
}
