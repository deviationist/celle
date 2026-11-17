// Package api wires the Huma operations.
//
// Every operation answers in JSON by default. With ?raw (or Accept:
// text/plain) it answers with the bare value as plain text, which is what
// Google Sheets' IMPORTDATA puts straight into a cell.
package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/deviationist/celle/internal/calendar"
	"github.com/deviationist/celle/internal/dates"
)

// Config holds what the handlers need from outside.
type Config struct {
	Version   string
	DefaultTZ *time.Location
	// DefaultPayday ("1"–"31" or "last") is used when a payday request has
	// no day. Empty means day is required.
	DefaultPayday string
	// DefaultCountry is used when a request has no country. Empty means
	// country is required where a calendar is needed.
	DefaultCountry string
	// Tokens are the accepted API tokens. Empty means every operation except
	// the public ones answers 401.
	Tokens []string
	// Now is the clock; tests replace it.
	Now func() time.Time
}

type server struct {
	cfg Config
}

// New returns the HTTP handler for the whole service.
func New(cfg Config) http.Handler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	mux := http.NewServeMux()

	hc := huma.DefaultConfig("celle", cfg.Version)
	hc.Info.Description = "Date helpers for spreadsheets: day counts, banking days, holidays and paydays. " +
		"Add `raw` to any query string to get the bare value as text/plain, for Google Sheets' IMPORTDATA."
	// No $schema links: they wrap response bodies, and the plain-text format
	// needs the original types.
	hc.CreateHooks = nil
	hc.Formats = map[string]huma.Format{
		"application/json": huma.DefaultJSONFormat,
		"json":             huma.DefaultJSONFormat,
		"text/plain":       textFormat,
		"text":             textFormat,
	}
	hc.Components.SecuritySchemes = securitySchemes()
	hc.Security = []map[string][]string{{"bearer": {}}, {"query": {}}}
	a := humago.New(mux, hc)
	a.UseMiddleware(tokenAuth(a, cfg.Tokens))

	s := &server{cfg: cfg}
	s.register(a)

	return withRaw(withCommonHeaders(mux))
}

func (s *server) register(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/healthz",
		Summary: "Health check", Tags: []string{"meta"},
		Security: []map[string][]string{}, Metadata: map[string]any{metaPublic: true},
	}, func(ctx context.Context, _ *struct{}) (*TextOutput, error) {
		return &TextOutput{Body: TextBody{Value: "ok"}}, nil
	})

	huma.Register(a, huma.Operation{
		OperationID: "list-countries", Method: http.MethodGet, Path: "/v1/countries",
		Summary: "Countries with a banking calendar", Tags: []string{"holidays"},
	}, s.countries)

	huma.Register(a, huma.Operation{
		OperationID: "list-holidays", Method: http.MethodGet, Path: "/v1/holidays",
		Summary: "Holidays in a year", Tags: []string{"holidays"},
		Description: "Public holidays and bank-closing days. As text: one `date,name` line per holiday.",
	}, s.holidays)

	huma.Register(a, huma.Operation{
		OperationID: "days-until", Method: http.MethodGet, Path: "/v1/days-until",
		Summary: "Calendar days from today to a date", Tags: []string{"days"},
		Description: "Negative when the date has passed.",
	}, s.daysUntil)

	huma.Register(a, huma.Operation{
		OperationID: "banking-days-until", Method: http.MethodGet, Path: "/v1/banking-days-until",
		Summary: "Banking days from today to a date", Tags: []string{"days"},
		Description: "Counts the banking days after today, up to and including the date. Negative when the date has passed.",
	}, s.bankingDaysUntil)

	huma.Register(a, huma.Operation{
		OperationID: "banking-day", Method: http.MethodGet, Path: "/v1/banking-day",
		Summary: "Move a date to a banking day", Tags: []string{"days"},
		Description: "Returns the date itself if banks are open, otherwise the previous (default) or next banking day.",
	}, s.bankingDay)

	huma.Register(a, huma.Operation{
		OperationID: "payday", Method: http.MethodGet, Path: "/v1/payday",
		Summary: "Payday date", Tags: []string{"payday"},
		Description: "Without year and month: the next payday on or after today. With them: the payday in that month.",
	}, s.payday)

	huma.Register(a, huma.Operation{
		OperationID: "days-until-payday", Method: http.MethodGet, Path: "/v1/days-until-payday",
		Summary: "Calendar days from today to payday", Tags: []string{"payday"},
		Description: "Same inputs as /v1/payday. Negative when an explicit month's payday has passed.",
	}, s.daysUntilPayday)
}

// --- inputs shared between operations ---

type TZParam struct {
	TZ string `query:"tz" doc:"IANA time zone that decides what 'today' is" example:"Europe/Oslo"`
}

func (s *server) location(tz string) (*time.Location, error) {
	if tz == "" {
		return s.cfg.DefaultTZ, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("unknown time zone %q", tz))
	}
	return loc, nil
}

func (s *server) today(tz string) (time.Time, error) {
	loc, err := s.location(tz)
	if err != nil {
		return time.Time{}, err
	}
	return dates.Today(s.cfg.Now(), loc), nil
}

func (s *server) getCalendar(country string) (*calendar.Calendar, error) {
	if country == "" {
		country = s.cfg.DefaultCountry
	}
	if country == "" {
		return nil, huma.Error400BadRequest("country is required (no default configured)")
	}
	c, err := calendar.Get(country)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return c, nil
}

func parseDate(name, v string) (time.Time, error) {
	d, err := dates.Parse(v)
	if err != nil {
		return time.Time{}, huma.Error400BadRequest(fmt.Sprintf("%s: %v", name, err))
	}
	return d, nil
}

const dateFormatDoc = "Output pattern for the date: YYYY, YY, MM, M, DD and D are replaced. Default YYYY-MM-DD"

// --- countries ---

type Country struct {
	Code string `json:"code" example:"NO"`
	Name string `json:"name" example:"Norway"`
}

type CountriesBody struct {
	Countries []Country `json:"countries"`
}

func (b CountriesBody) PlainText() string {
	out := ""
	for _, c := range b.Countries {
		out += c.Code + "," + c.Name + "\n"
	}
	return out
}

type CountriesOutput struct{ Body CountriesBody }

func (s *server) countries(ctx context.Context, _ *struct{}) (*CountriesOutput, error) {
	var b CountriesBody
	for _, c := range calendar.All() {
		b.Countries = append(b.Countries, Country{Code: c.Code, Name: c.Name})
	}
	return &CountriesOutput{Body: b}, nil
}

// --- holidays ---

type HolidaysInput struct {
	Country    string `query:"country" doc:"ISO 3166 country code. Defaults to the server's CELLE_DEFAULT_COUNTRY" example:"NO"`
	Year       int    `query:"year" doc:"Defaults to the current year"`
	DateFormat string `query:"date_format" doc:"Output pattern for the dates"`
	TZParam
}

type HolidayItem struct {
	Date string `json:"date" example:"2026-12-24"`
	Name string `json:"name" example:"Julaften"`
	Kind string `json:"kind" enum:"public,bank" doc:"public = public holiday, bank = only banks are closed"`
}

type HolidaysBody struct {
	Country  string        `json:"country"`
	Year     int           `json:"year"`
	Holidays []HolidayItem `json:"holidays"`
}

func (b HolidaysBody) PlainText() string {
	out := ""
	for _, h := range b.Holidays {
		out += h.Date + "," + h.Name + "\n"
	}
	return out
}

type HolidaysOutput struct{ Body HolidaysBody }

func (s *server) holidays(ctx context.Context, in *HolidaysInput) (*HolidaysOutput, error) {
	c, err := s.getCalendar(in.Country)
	if err != nil {
		return nil, err
	}
	year := in.Year
	if year == 0 {
		t, err := s.today(in.TZ)
		if err != nil {
			return nil, err
		}
		year = t.Year()
	}
	if year < 1900 || year > 9999 {
		return nil, huma.Error400BadRequest("year must be 1900–9999")
	}
	b := HolidaysBody{Country: c.Code, Year: year, Holidays: []HolidayItem{}}
	for _, h := range c.Holidays(year) {
		b.Holidays = append(b.Holidays, HolidayItem{Date: dates.Format(h.Date, in.DateFormat), Name: h.Name, Kind: h.Kind})
	}
	return &HolidaysOutput{Body: b}, nil
}

// --- day counts ---

type DaysUntilInput struct {
	Date string `query:"date" required:"true" doc:"YYYY-MM-DD, DD.MM.YYYY or a spreadsheet serial number" example:"2026-12-24"`
	TZParam
}

type DaysBody struct {
	Today string `json:"today" example:"2026-11-17"`
	Date  string `json:"date" example:"2026-12-24"`
	Days  int    `json:"days" example:"37"`
}

func (b DaysBody) PlainText() string { return fmt.Sprint(b.Days) }

type DaysOutput struct{ Body DaysBody }

func (s *server) daysUntil(ctx context.Context, in *DaysUntilInput) (*DaysOutput, error) {
	d, err := parseDate("date", in.Date)
	if err != nil {
		return nil, err
	}
	today, err := s.today(in.TZ)
	if err != nil {
		return nil, err
	}
	return &DaysOutput{Body: DaysBody{Today: dates.Format(today, ""), Date: dates.Format(d, ""), Days: dates.DaysBetween(today, d)}}, nil
}

type BankingDaysUntilInput struct {
	Date    string `query:"date" required:"true" doc:"YYYY-MM-DD, DD.MM.YYYY or a spreadsheet serial number" example:"2026-12-24"`
	Country string `query:"country" doc:"ISO 3166 country code. Defaults to the server's CELLE_DEFAULT_COUNTRY" example:"NO"`
	TZParam
}

func (s *server) bankingDaysUntil(ctx context.Context, in *BankingDaysUntilInput) (*DaysOutput, error) {
	d, err := parseDate("date", in.Date)
	if err != nil {
		return nil, err
	}
	c, err := s.getCalendar(in.Country)
	if err != nil {
		return nil, err
	}
	today, err := s.today(in.TZ)
	if err != nil {
		return nil, err
	}
	return &DaysOutput{Body: DaysBody{Today: dates.Format(today, ""), Date: dates.Format(d, ""), Days: dates.BankingDaysBetween(c, today, d)}}, nil
}

// --- banking day ---

type BankingDayInput struct {
	Date       string `query:"date" required:"true" doc:"YYYY-MM-DD, DD.MM.YYYY or a spreadsheet serial number" example:"2026-12-25"`
	Country    string `query:"country" doc:"ISO 3166 country code. Defaults to the server's CELLE_DEFAULT_COUNTRY" example:"NO"`
	Roll       string `query:"roll" enum:"previous,next" default:"previous" doc:"Direction to move a non-banking day"`
	DateFormat string `query:"date_format" doc:"Output pattern for the date: YYYY, YY, MM, M, DD and D are replaced. Default YYYY-MM-DD"`
}

type DateBody struct {
	Date      string `json:"date" example:"2026-12-23"`
	Requested string `json:"requested,omitempty" example:"2026-12-25" doc:"The date before rolling"`
	Holiday   string `json:"holiday,omitempty" example:"Første juledag" doc:"Holiday on the requested date, if any"`
	DaysUntil *int   `json:"days_until,omitempty" doc:"Calendar days from today"`
}

func (b DateBody) PlainText() string { return b.Date }

type DateOutput struct{ Body DateBody }

func (s *server) bankingDay(ctx context.Context, in *BankingDayInput) (*DateOutput, error) {
	d, err := parseDate("date", in.Date)
	if err != nil {
		return nil, err
	}
	c, err := s.getCalendar(in.Country)
	if err != nil {
		return nil, err
	}
	b := DateBody{Date: dates.Format(dates.ToBankingDay(c, d, dates.Roll(in.Roll)), in.DateFormat), Requested: dates.Format(d, "")}
	if h, ok := c.HolidayOn(d); ok {
		b.Holiday = h.Name
	}
	return &DateOutput{Body: b}, nil
}

// --- payday ---

type PaydayInput struct {
	Day        string `query:"day" doc:"Day of the month (1–31) or \"last\". A day past the month's end means its last day. Defaults to the server's CELLE_DEFAULT_PAYDAY" example:"25"`
	Year       int    `query:"year" doc:"With month: that month's payday. Without both: the next payday"`
	Month      int    `query:"month" minimum:"0" maximum:"12" doc:"1–12, used together with year"`
	Country    string `query:"country" doc:"ISO 3166 country code; a payday that is not a banking day is moved to one. Defaults to the server's CELLE_DEFAULT_COUNTRY; \"none\" turns moving off" example:"NO"`
	Roll       string `query:"roll" enum:"previous,next" default:"previous" doc:"Direction to move a payday that is not a banking day"`
	DateFormat string `query:"date_format" doc:"Output pattern for the date: YYYY, YY, MM, M, DD and D are replaced. Default YYYY-MM-DD"`
	TZParam
}

func (s *server) resolvePayday(in *PaydayInput) (payday, today time.Time, err error) {
	dayStr := in.Day
	if dayStr == "" {
		dayStr = s.cfg.DefaultPayday
	}
	if dayStr == "" {
		return payday, today, huma.Error400BadRequest("day is required (no default configured)")
	}
	day, err := dates.ParsePayday(dayStr)
	if err != nil {
		return payday, today, huma.Error400BadRequest(err.Error())
	}
	var c *calendar.Calendar
	country := in.Country
	if country == "" {
		country = s.cfg.DefaultCountry
	}
	if country != "" && !strings.EqualFold(country, "none") {
		if c, err = s.getCalendar(country); err != nil {
			return payday, today, err
		}
	}
	if today, err = s.today(in.TZ); err != nil {
		return payday, today, err
	}
	roll := dates.Roll(in.Roll)
	switch {
	case in.Year == 0 && in.Month == 0:
		payday = dates.NextPayday(today, day, c, roll)
	case in.Year == 0 || in.Month == 0:
		return payday, today, huma.Error400BadRequest("give both year and month, or neither for the next payday")
	case in.Year < 1900 || in.Year > 9999:
		return payday, today, huma.Error400BadRequest("year must be 1900–9999")
	default:
		payday = dates.Payday(in.Year, time.Month(in.Month), day, c, roll)
	}
	return payday, today, nil
}

func (s *server) payday(ctx context.Context, in *PaydayInput) (*DateOutput, error) {
	p, today, err := s.resolvePayday(in)
	if err != nil {
		return nil, err
	}
	days := dates.DaysBetween(today, p)
	return &DateOutput{Body: DateBody{Date: dates.Format(p, in.DateFormat), DaysUntil: &days}}, nil
}

func (s *server) daysUntilPayday(ctx context.Context, in *PaydayInput) (*DaysOutput, error) {
	p, today, err := s.resolvePayday(in)
	if err != nil {
		return nil, err
	}
	return &DaysOutput{Body: DaysBody{Today: dates.Format(today, ""), Date: dates.Format(p, ""), Days: dates.DaysBetween(today, p)}}, nil
}
