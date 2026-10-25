// Package stats turns the timestamps of sent and received mails into
// per-day/week/month averages over the mailbox history.
package stats

import "time"

const (
	// DaysPerWeek is the length of a week in days.
	DaysPerWeek = 7.0
	// DaysPerMonth is the length of an average Gregorian month in days.
	DaysPerMonth = 365.2425 / 12
)

// Options limits the window the averages are computed over. Zero values mean
// "unbounded": Since defaults to the oldest mail, Until to Now.
type Options struct {
	Since time.Time
	Until time.Time
	Now   time.Time
}

// Summary holds the counts and averages for one direction (sent/received).
type Summary struct {
	Total    int        `json:"total" yaml:"total"`
	PerDay   float64    `json:"perDay" yaml:"perDay"`
	PerWeek  float64    `json:"perWeek" yaml:"perWeek"`
	PerMonth float64    `json:"perMonth" yaml:"perMonth"`
	First    *time.Time `json:"first,omitempty" yaml:"first,omitempty"`
	Last     *time.Time `json:"last,omitempty" yaml:"last,omitempty"`
}

// Report is the result of Compute. Sent and Received share the same window
// (From..To) so their averages are directly comparable.
type Report struct {
	From     time.Time `json:"from" yaml:"from"`
	To       time.Time `json:"to" yaml:"to"`
	Days     float64   `json:"days" yaml:"days"`
	Sent     Summary   `json:"sent" yaml:"sent"`
	Received Summary   `json:"received" yaml:"received"`
}

// Compute filters the timestamps to the window described by opts and
// averages them over it. The window starts at opts.Since, or at the oldest
// mail of either direction, and ends at opts.Until, or opts.Now. Each period
// count is clamped to at least one, so a history shorter than a month does not
// extrapolate a monthly average from a few days.
func Compute(sent, received []time.Time, opts Options) Report {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	to := now
	if !opts.Until.IsZero() {
		to = opts.Until
	}

	sent = filter(sent, opts.Since, to)
	received = filter(received, opts.Since, to)

	from := opts.Since
	if from.IsZero() {
		from = to
		for _, ts := range [][]time.Time{sent, received} {
			if first, _, ok := bounds(ts); ok && first.Before(from) {
				from = first
			}
		}
	}

	days := to.Sub(from).Hours() / 24
	if days < 0 {
		days = 0
	}

	return Report{
		From:     from,
		To:       to,
		Days:     days,
		Sent:     summarize(sent, days),
		Received: summarize(received, days),
	}
}

func summarize(ts []time.Time, days float64) Summary {
	s := Summary{Total: len(ts)}
	if first, last, ok := bounds(ts); ok {
		s.First, s.Last = &first, &last
	}
	total := float64(len(ts))
	s.PerDay = total / atLeastOne(days)
	s.PerWeek = total / atLeastOne(days/DaysPerWeek)
	s.PerMonth = total / atLeastOne(days/DaysPerMonth)
	return s
}

func atLeastOne(periods float64) float64 {
	if periods < 1 {
		return 1
	}
	return periods
}

// filter keeps the timestamps in [since, until]. A zero since is unbounded.
func filter(ts []time.Time, since, until time.Time) []time.Time {
	out := make([]time.Time, 0, len(ts))
	for _, t := range ts {
		if (!since.IsZero() && t.Before(since)) || t.After(until) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func bounds(ts []time.Time) (first, last time.Time, ok bool) {
	for i, t := range ts {
		if i == 0 || t.Before(first) {
			first = t
		}
		if i == 0 || t.After(last) {
			last = t
		}
	}
	return first, last, len(ts) > 0
}

// EndOfDay returns the last instant of t's calendar day, used to make an
// --until date inclusive.
func EndOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}
