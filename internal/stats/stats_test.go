package stats

import (
	"math"
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestCompute_WindowFromOldestMailToNow(t *testing.T) {
	now := date(2024, 1, 31)
	sent := []time.Time{date(2024, 1, 10), date(2024, 1, 20)}
	received := []time.Time{date(2023, 12, 2), date(2024, 1, 5), date(2024, 1, 30), date(2024, 1, 1)}

	r := Compute(sent, received, Options{Now: now})

	if !r.From.Equal(date(2023, 12, 2)) || !r.To.Equal(now) {
		t.Fatalf("window = %v..%v", r.From, r.To)
	}
	approx(t, "days", r.Days, 60)
	if r.Sent.Total != 2 || r.Received.Total != 4 {
		t.Fatalf("totals = %d/%d", r.Sent.Total, r.Received.Total)
	}
	approx(t, "sent per day", r.Sent.PerDay, 2.0/60)
	approx(t, "sent per week", r.Sent.PerWeek, 2.0/(60/DaysPerWeek))
	approx(t, "received per month", r.Received.PerMonth, 4.0/(60/DaysPerMonth))
	if !r.Sent.First.Equal(date(2024, 1, 10)) || !r.Sent.Last.Equal(date(2024, 1, 20)) {
		t.Errorf("sent bounds = %v..%v", r.Sent.First, r.Sent.Last)
	}
	if !r.Received.First.Equal(date(2023, 12, 2)) || !r.Received.Last.Equal(date(2024, 1, 30)) {
		t.Errorf("received bounds = %v..%v", r.Received.First, r.Received.Last)
	}
}

func TestCompute_ShortHistoryDoesNotExtrapolate(t *testing.T) {
	now := date(2024, 1, 3)
	r := Compute([]time.Time{date(2024, 1, 1), date(2024, 1, 2)}, nil, Options{Now: now})

	approx(t, "days", r.Days, 2)
	approx(t, "per day", r.Sent.PerDay, 1)
	// Two days are less than a week and a month: the period counts as one.
	approx(t, "per week", r.Sent.PerWeek, 2)
	approx(t, "per month", r.Sent.PerMonth, 2)
}

func TestCompute_SinceUntilFilter(t *testing.T) {
	received := []time.Time{date(2023, 1, 1), date(2024, 1, 10), date(2024, 2, 10), date(2025, 1, 1)}
	opts := Options{Since: date(2024, 1, 1), Until: date(2024, 3, 1), Now: date(2026, 1, 1)}

	r := Compute(nil, received, opts)

	if r.Received.Total != 2 {
		t.Fatalf("total = %d, want 2", r.Received.Total)
	}
	if !r.From.Equal(opts.Since) || !r.To.Equal(opts.Until) {
		t.Fatalf("window = %v..%v", r.From, r.To)
	}
	approx(t, "days", r.Days, 60)
	if r.Sent.First != nil || r.Sent.Last != nil || r.Sent.Total != 0 {
		t.Errorf("empty sent summary = %+v", r.Sent)
	}
}

func TestCompute_Empty(t *testing.T) {
	r := Compute(nil, nil, Options{})
	if r.Days != 0 || r.Sent.Total != 0 || r.Received.PerDay != 0 {
		t.Fatalf("empty report = %+v", r)
	}
	if r.To.IsZero() || !r.From.Equal(r.To) {
		t.Fatalf("empty window = %v..%v", r.From, r.To)
	}
}

func TestCompute_SinceAfterUntil(t *testing.T) {
	r := Compute(nil, nil, Options{Since: date(2024, 2, 1), Until: date(2024, 1, 1)})
	if r.Days != 0 {
		t.Fatalf("days = %v, want 0", r.Days)
	}
}

func TestEndOfDay(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	got := EndOfDay(time.Date(2024, 5, 6, 0, 0, 0, 0, loc))
	want := time.Date(2024, 5, 6, 23, 59, 59, 999999999, loc)
	if !got.Equal(want) {
		t.Fatalf("EndOfDay = %v, want %v", got, want)
	}
}
