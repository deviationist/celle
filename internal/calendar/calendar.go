// Package calendar decides which days are banking days in a country.
//
// Holiday rules come from github.com/rickar/cal/v2. Each supported country
// adds what that library leaves out for banking purposes (for Norway:
// julaften, a bank-closing day that is not a public holiday).
package calendar

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rickar/cal/v2"
	"github.com/rickar/cal/v2/no"
)

// Holiday is one non-banking day other than a weekend.
type Holiday struct {
	Date time.Time
	Name string
	// Kind is "public" for a public holiday and "bank" for a day that is only
	// closed for banks.
	Kind string
}

// Calendar answers banking-day questions for one country.
type Calendar struct {
	Code string // ISO 3166-1 alpha-2, upper case
	Name string
	bc   *cal.BusinessCalendar
}

type country struct {
	name     string
	holidays []*cal.Holiday
}

var (
	// Norges Bank's settlement system (NBO) is closed on julaften although it
	// is not a public holiday. Nyttårsaften is NOT closed in Norway (only in
	// Sweden and Denmark); Banko had it wrong. Sources: norges-bank.no
	// "Oppgjørsdager" and Bits' NICS calendar.
	// (Not built with Holiday.Clone: Clone ignores Month/Day overrides.)
	noJulaften = &cal.Holiday{
		Name: "Julaften", Type: cal.ObservanceBank,
		Month: time.December, Day: 24, Func: cal.CalcDayOfMonth,
	}
)

var countries = map[string]country{
	"NO": {
		name:     "Norway",
		holidays: append(append([]*cal.Holiday{}, no.Holidays...), noJulaften),
	},
}

// aliases maps alternative country identifiers (Banko used English names) to
// ISO codes.
var aliases = map[string]string{
	"NORWAY": "NO",
	"NORGE":  "NO",
}

var built = map[string]*Calendar{}

func init() {
	for code, c := range countries {
		bc := cal.NewBusinessCalendar()
		bc.AddHoliday(c.holidays...)
		built[code] = &Calendar{Code: code, Name: c.name, bc: bc}
	}
}

// Get returns the calendar for an ISO country code (or a known alias such as
// "norway"). Matching ignores case.
func Get(id string) (*Calendar, error) {
	key := strings.ToUpper(strings.TrimSpace(id))
	if code, ok := aliases[key]; ok {
		key = code
	}
	if c, ok := built[key]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("unsupported country %q (supported: %s)", id, strings.Join(Codes(), ", "))
}

// Codes lists the supported ISO country codes, sorted.
func Codes() []string {
	codes := make([]string, 0, len(built))
	for code := range built {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// All returns every supported calendar, sorted by code.
func All() []*Calendar {
	out := make([]*Calendar, 0, len(built))
	for _, code := range Codes() {
		out = append(out, built[code])
	}
	return out
}

// IsBankingDay reports whether banks are open on the date: not a weekend and
// not a holiday.
func (c *Calendar) IsBankingDay(d time.Time) bool {
	return c.bc.IsWorkday(d)
}

// HolidayOn returns the holiday on the date, if any. Weekends that are not
// holidays return false.
func (c *Calendar) HolidayOn(d time.Time) (Holiday, bool) {
	actual, observed, h := c.bc.IsHoliday(d)
	if !actual && !observed {
		return Holiday{}, false
	}
	return Holiday{Date: dateOnly(d), Name: h.Name, Kind: kind(h)}, true
}

// Holidays lists the holidays in a year, in date order. Holidays that fall on a
// weekend are included.
func (c *Calendar) Holidays(year int) []Holiday {
	var out []Holiday
	d := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	for d.Year() == year {
		if h, ok := c.HolidayOn(d); ok {
			out = append(out, h)
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

func kind(h *cal.Holiday) string {
	if h.Type == cal.ObservanceBank {
		return "bank"
	}
	return "public"
}

func dateOnly(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}
