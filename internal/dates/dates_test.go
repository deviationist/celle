package dates

import (
	"testing"
	"time"

	"github.com/deviationist/celle/internal/calendar"
)

func iso(t time.Time) string { return t.Format("2006-01-02") }

func TestParse(t *testing.T) {
	cases := map[string]string{
		"2026-12-24": "2026-12-24",
		"24.12.2026": "2026-12-24",
		"4.1.2027":   "2027-01-04",
		"2026/12/24": "2026-12-24",
		"46380":      "2026-12-24", // spreadsheet serial
		" 46380.75 ": "2026-12-24", // serial with a time part
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || iso(got) != want {
			t.Errorf("Parse(%q) = %s, %v; want %s", in, iso(got), err, want)
		}
	}
	for _, bad := range []string{"", "garbage", "2026-13-01", "31.02.2026", "0", "-5"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestToday(t *testing.T) {
	oslo, _ := time.LoadLocation("Europe/Oslo")
	// 23:30 UTC is already the next day in Oslo; Banko (UTC) got this wrong.
	now := time.Date(2026, 11, 17, 23, 30, 0, 0, time.UTC)
	if got := iso(Today(now, oslo)); got != "2026-11-18" {
		t.Errorf("Today = %s, want 2026-11-18", got)
	}
}

func TestDaysBetweenAcrossDST(t *testing.T) {
	if n := DaysBetween(Date(2026, 3, 1), Date(2026, 4, 1)); n != 31 {
		t.Errorf("got %d, want 31", n)
	}
	if n := DaysBetween(Date(2026, 11, 17), Date(2026, 10, 23)); n != -25 {
		t.Errorf("got %d, want -25 (Banko returned +25)", n)
	}
}

func TestBankingDaysBetween(t *testing.T) {
	no, _ := calendar.Get("NO")
	// Mon 2026-12-21 → Thu 2026-12-31: 22, 23, (24 julaften), (25, 26, 27), 28, 29, 30, 31.
	if n := BankingDaysBetween(no, Date(2026, 12, 21), Date(2026, 12, 31)); n != 6 {
		t.Errorf("got %d, want 6", n)
	}
	if n := BankingDaysBetween(no, Date(2026, 12, 31), Date(2026, 12, 21)); n != -6 {
		t.Errorf("reverse: got %d, want -6", n)
	}
	if n := BankingDaysBetween(no, Date(2026, 12, 21), Date(2026, 12, 21)); n != 0 {
		t.Errorf("same day: got %d, want 0", n)
	}
}

func TestToBankingDay(t *testing.T) {
	no, _ := calendar.Get("NO")
	cases := []struct {
		in   time.Time
		roll Roll
		want string
	}{
		{Date(2026, 12, 25), RollPrevious, "2026-12-23"}, // skips julaften
		{Date(2026, 12, 25), RollNext, "2026-12-28"},
		{Date(2026, 4, 5), RollPrevious, "2026-04-01"}, // Easter Sunday → Wednesday
		{Date(2026, 12, 31), RollPrevious, "2026-12-31"},
		{Date(2026, 11, 17), RollPrevious, "2026-11-17"},
	}
	for _, c := range cases {
		if got := iso(ToBankingDay(no, c.in, c.roll)); got != c.want {
			t.Errorf("ToBankingDay(%s, %s) = %s, want %s", iso(c.in), c.roll, got, c.want)
		}
	}
}

func TestPayday(t *testing.T) {
	no, _ := calendar.Get("NO")
	cases := []struct {
		y    int
		m    time.Month
		day  int
		cal  *calendar.Calendar
		want string
	}{
		{2027, 2, 31, nil, "2027-02-28"},      // clamps (Banko: 2027-03-03)
		{2028, 2, 30, nil, "2028-02-29"},      // leap year
		{2027, 2, LastDay, nil, "2027-02-28"}, // "last"
		{2026, 12, 25, no, "2026-12-23"},      // Banko: 2026-12-24
		{2026, 12, LastDay, no, "2026-12-31"}, // Banko: 2026-12-30
		{2026, 10, 25, no, "2026-10-23"},      // Sunday → Friday
		{2021, 2, 29, no, "2021-02-26"},       // clamp to Sun 28th, then roll
	}
	for _, c := range cases {
		if got := iso(Payday(c.y, c.m, c.day, c.cal, RollPrevious)); got != c.want {
			t.Errorf("Payday(%d-%d, %d) = %s, want %s", c.y, c.m, c.day, got, c.want)
		}
	}
}

func TestNextPayday(t *testing.T) {
	no, _ := calendar.Get("NO")
	cases := []struct {
		today time.Time
		day   int
		want  string
	}{
		{Date(2026, 11, 17), 25, "2026-11-25"},
		{Date(2026, 11, 25), 25, "2026-11-25"}, // today counts
		{Date(2026, 11, 26), 25, "2026-12-23"},
		{Date(2026, 12, 24), 25, "2027-01-25"}, // December's payday (23rd) has passed
		{Date(2026, 12, 31), LastDay, "2026-12-31"},
	}
	for _, c := range cases {
		if got := iso(NextPayday(c.today, c.day, no, RollPrevious)); got != c.want {
			t.Errorf("NextPayday(%s, %d) = %s, want %s", iso(c.today), c.day, got, c.want)
		}
	}
}

func TestParsePayday(t *testing.T) {
	for in, want := range map[string]int{"1": 1, "31": 31, "last": LastDay, "LAST": LastDay, "last-in-month": LastDay} {
		if got, err := ParsePayday(in); err != nil || got != want {
			t.Errorf("ParsePayday(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"0", "32", "x", ""} {
		if _, err := ParsePayday(bad); err == nil {
			t.Errorf("ParsePayday(%q) should fail", bad)
		}
	}
}

func TestFormat(t *testing.T) {
	day := Date(2027, 1, 4)
	for pattern, want := range map[string]string{
		"":           "2027-01-04",
		"DD.MM.YYYY": "04.01.2027",
		"D.M.YY":     "4.1.27",
		"YYYYMMDD":   "20270104",
	} {
		if got := Format(day, pattern); got != want {
			t.Errorf("Format(%q) = %s, want %s", pattern, got, want)
		}
	}
}
