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

func at(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, time.UTC)
}

func TestCompute_BusiestDays(t *testing.T) {
	sent := []time.Time{at(2024, 1, 2, 9), at(2024, 1, 3, 9), at(2024, 1, 3, 15), at(2024, 1, 5, 8)}
	received := []time.Time{
		at(2024, 1, 1, 8), at(2024, 1, 1, 9), at(2024, 1, 1, 10), // 3 received on the 1st
		at(2024, 1, 5, 8), at(2024, 1, 5, 9), at(2024, 1, 5, 10), // 3 received + 1 sent on the 5th
	}

	r := Compute(sent, received, Options{Now: at(2024, 2, 1, 0), Location: time.UTC})

	if d := r.Sent.BusiestDay; d == nil || *d != (Day{Date: "2024-01-03", Count: 2}) {
		t.Errorf("sent busiest = %+v", d)
	}
	// The 1st and 5th tie with 3 received; the earlier day wins.
	if d := r.Received.BusiestDay; d == nil || *d != (Day{Date: "2024-01-01", Count: 3}) {
		t.Errorf("received busiest = %+v", d)
	}
	// Overall the 5th wins: 1 sent + 3 received beats 3 received on the 1st.
	if d := r.BusiestDay; d == nil || *d != (BusiestDay{Date: "2024-01-05", Total: 4, Sent: 1, Received: 3}) {
		t.Errorf("overall busiest = %+v", d)
	}
}

func TestCompute_BusiestDayUsesLocation(t *testing.T) {
	// 23:30 UTC is already the next day in Zurich (UTC+1 in January).
	zurich := time.FixedZone("CET", 3600)
	received := []time.Time{time.Date(2024, 1, 1, 23, 30, 0, 0, time.UTC), at(2024, 1, 2, 10)}

	if d := Compute(nil, received, Options{Location: time.UTC}).Received.BusiestDay; d.Date != "2024-01-01" || d.Count != 1 {
		t.Errorf("UTC busiest = %+v", d)
	}
	if d := Compute(nil, received, Options{Location: zurich}).Received.BusiestDay; d.Date != "2024-01-02" || d.Count != 2 {
		t.Errorf("Zurich busiest = %+v", d)
	}
	// Without a location, time.Local is used; it must not panic.
	if d := Compute(nil, received, Options{}).BusiestDay; d == nil || d.Total < 1 {
		t.Errorf("local busiest = %+v", d)
	}
}

func TestCompute_BusiestDayRespectsWindowAndEmpty(t *testing.T) {
	received := []time.Time{at(2023, 6, 1, 9), at(2023, 6, 1, 10), at(2024, 1, 10, 9)}
	r := Compute(nil, received, Options{Since: at(2024, 1, 1, 0), Now: at(2024, 2, 1, 0), Location: time.UTC})
	if d := r.Received.BusiestDay; d == nil || d.Date != "2024-01-10" {
		t.Errorf("busiest outside window: %+v", d)
	}
	if r.Sent.BusiestDay != nil {
		t.Errorf("no sent mails: busiest = %+v", r.Sent.BusiestDay)
	}

	empty := Compute(nil, nil, Options{})
	if empty.BusiestDay != nil || empty.Sent.BusiestDay != nil || empty.Received.BusiestDay != nil {
		t.Errorf("empty report has busiest days: %+v", empty)
	}
}
