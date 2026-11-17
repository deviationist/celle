// Package dates holds the date arithmetic behind the API: parsing the many
// shapes a spreadsheet sends a date in, day counts, rolling to a banking day
// and paydays.
//
// All dates are calendar dates: midnight UTC, no time of day. "Today" depends
// on a time zone and is passed in by the caller.
package dates

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/deviationist/celle/internal/calendar"
)

// spreadsheetEpoch is day 0 of the serial dates used by Google Sheets and
// Excel (1900 date system, including Excel's 1900 leap-year quirk).
var spreadsheetEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)

// Date builds a calendar date.
func Date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Today returns today's date in the time zone.
func Today(now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	return Date(n.Year(), n.Month(), n.Day())
}

var layouts = []string{"2006-01-02", "02.01.2006", "2.1.2006", "2006/01/02"}

// Parse accepts an ISO date (2026-12-24), a Norwegian date (24.12.2026) or a
// spreadsheet serial number (46380), which is what a date cell becomes when it
// is concatenated into a URL without TEXT().
func Parse(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("date is empty")
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if n < 1 || n > 2958465 { // 9999-12-31
			return time.Time{}, fmt.Errorf("serial date %q is out of range", s)
		}
		return spreadsheetEpoch.AddDate(0, 0, int(n)), nil
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %q as a date (use YYYY-MM-DD, DD.MM.YYYY or a serial number)", s)
}

// DaysBetween counts calendar days from a to b: positive when b is later.
func DaysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// BankingDaysBetween counts the banking days after a, up to and including b.
// It is negative when b is before a (the banking days after b up to and
// including a).
func BankingDaysBetween(c *calendar.Calendar, a, b time.Time) int {
	sign := 1
	if b.Before(a) {
		a, b, sign = b, a, -1
	}
	n := 0
	for d := a.AddDate(0, 0, 1); !d.After(b); d = d.AddDate(0, 0, 1) {
		if c.IsBankingDay(d) {
			n++
		}
	}
	return sign * n
}

// Roll says which way to move a date that is not a banking day.
type Roll string

const (
	RollPrevious Roll = "previous"
	RollNext     Roll = "next"
)

// ToBankingDay returns d if it is a banking day, otherwise the nearest banking
// day before it (RollPrevious) or after it (RollNext).
func ToBankingDay(c *calendar.Calendar, d time.Time, roll Roll) time.Time {
	step := -1
	if roll == RollNext {
		step = 1
	}
	for !c.IsBankingDay(d) {
		d = d.AddDate(0, 0, step)
	}
	return d
}

// LastDay is the payday value meaning the last day of the month.
const LastDay = 0

// ParsePayday reads a payday: a day of the month (1–31) or "last".
func ParsePayday(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "last" || s == "last-in-month" {
		return LastDay, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 31 {
		return 0, fmt.Errorf("payday %q must be 1–31 or \"last\"", s)
	}
	return n, nil
}

// Payday returns the payday in a month. A day past the end of the month (31 in
// February) means the last day. With a calendar, a payday that is not a banking
// day rolls to one; without, it is returned as is.
func Payday(year int, month time.Month, day int, c *calendar.Calendar, roll Roll) time.Time {
	last := Date(year, month+1, 0).Day()
	if day == LastDay || day > last {
		day = last
	}
	d := Date(year, month, day)
	if c != nil {
		d = ToBankingDay(c, d, roll)
	}
	return d
}

// NextPayday returns the first payday on or after today.
func NextPayday(today time.Time, day int, c *calendar.Calendar, roll Roll) time.Time {
	y, m := today.Year(), today.Month()
	for {
		p := Payday(y, m, day, c, roll)
		if !p.Before(today) {
			return p
		}
		m++
		if m > time.December {
			m, y = time.January, y+1
		}
	}
}

// Format renders a date with a spreadsheet-style pattern: YYYY, YY, MM, M, DD
// and D are replaced; anything else is kept. An empty pattern gives ISO
// (YYYY-MM-DD).
func Format(d time.Time, pattern string) string {
	if pattern == "" {
		return d.Format("2006-01-02")
	}
	r := strings.NewReplacer(
		"YYYY", fmt.Sprintf("%04d", d.Year()),
		"YY", fmt.Sprintf("%02d", d.Year()%100),
		"MM", fmt.Sprintf("%02d", int(d.Month())),
		"M", strconv.Itoa(int(d.Month())),
		"DD", fmt.Sprintf("%02d", d.Day()),
		"D", strconv.Itoa(d.Day()),
	)
	return r.Replace(pattern)
}
