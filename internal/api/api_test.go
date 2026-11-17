package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "test-token-0123456789"

func newTestServer(now time.Time) http.Handler {
	oslo, _ := time.LoadLocation("Europe/Oslo")
	return New(Config{Version: "test", DefaultTZ: oslo, Tokens: []string{testToken}, Now: func() time.Time { return now }})
}

func get(h http.Handler, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var now = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

func TestAuth(t *testing.T) {
	h := newTestServer(now)
	cases := []struct {
		name   string
		path   string
		header map[string]string
		want   int
	}{
		{"health is public", "/healthz", nil, 200},
		{"docs are public", "/openapi.json", nil, 200},
		{"no token", "/v1/days-until?date=2026-12-24", nil, 401},
		{"wrong token", "/v1/days-until?date=2026-12-24&token=nope", nil, 401},
		{"query token", "/v1/days-until?date=2026-12-24&token=" + testToken, nil, 200},
		{"bearer token", "/v1/days-until?date=2026-12-24", map[string]string{"Authorization": "Bearer " + testToken}, 200},
		{"wrong bearer", "/v1/days-until?date=2026-12-24", map[string]string{"Authorization": "Bearer nope"}, 401},
	}
	for _, c := range cases {
		if rec := get(h, c.path, c.header); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
}

func TestEmptyTokenListLocksEverything(t *testing.T) {
	oslo, _ := time.LoadLocation("Europe/Oslo")
	h := New(Config{DefaultTZ: oslo})
	if rec := get(h, "/v1/days-until?date=2026-12-24&token=", nil); rec.Code != 401 {
		t.Errorf("status %d, want 401", rec.Code)
	}
}

func TestRawOutput(t *testing.T) {
	h := newTestServer(now)
	tok := "&token=" + testToken
	cases := map[string]string{
		"/v1/days-until?raw&date=2026-12-24":                                        "37",
		"/v1/days-until?raw&date=46380":                                             "37",
		"/v1/banking-days-until?raw&date=2026-12-24&country=NO":                     "26",
		"/v1/banking-day?raw&date=2026-12-25&country=NO":                            "2026-12-23",
		"/v1/banking-day?raw&date=2026-12-25&country=NO&roll=next":                  "2026-12-28",
		"/v1/banking-day?raw&date=2026-12-25&country=norway&date_format=DD.MM.YYYY": "23.12.2026",
		"/v1/payday?raw&day=25":                                                     "2026-11-25",
		"/v1/payday?raw&day=25&year=2026&month=12&country=NO":                       "2026-12-23",
		"/v1/payday?raw&day=31&year=2027&month=2":                                   "2027-02-28",
		"/v1/days-until-payday?raw&day=25&year=2026&month=10&country=NO":            "-25",
		"/v1/days-until-payday?raw&day=last&country=NO":                             "13",
	}
	for path, want := range cases {
		rec := get(h, path+tok, nil)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
			t.Errorf("%s: %d %q, want %q", path, rec.Code, rec.Body.String(), want)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("%s: Content-Type %q", path, ct)
		}
	}
}

func TestRawErrorIsReadableInACell(t *testing.T) {
	h := newTestServer(now)
	rec := get(h, "/v1/days-until?raw&date=garbage&token="+testToken, nil)
	if rec.Code != 400 || !strings.HasPrefix(rec.Body.String(), "ERROR: ") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestJSONOutput(t *testing.T) {
	h := newTestServer(now)
	rec := get(h, "/v1/payday?day=25&year=2026&month=12&country=NO&token="+testToken, nil)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if body["date"] != "2026-12-23" || body["days_until"] != float64(36) {
		t.Errorf("body = %v", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
}

func TestTimeZone(t *testing.T) {
	late := time.Date(2026, 11, 17, 23, 30, 0, 0, time.UTC) // 00:30 on the 18th in Oslo
	h := newTestServer(late)
	tok := "&token=" + testToken
	if b := get(h, "/v1/days-until?raw&date=2026-11-18"+tok, nil).Body.String(); b != "0" {
		t.Errorf("Oslo default: %q, want 0", b)
	}
	if b := get(h, "/v1/days-until?raw&date=2026-11-18&tz=UTC"+tok, nil).Body.String(); b != "1" {
		t.Errorf("tz=UTC: %q, want 1", b)
	}
	if rec := get(h, "/v1/days-until?raw&date=2026-11-18&tz=Mars/Base"+tok, nil); rec.Code != 400 {
		t.Errorf("bad tz: %d", rec.Code)
	}
}

func TestPaydayNeedsBothYearAndMonth(t *testing.T) {
	h := newTestServer(now)
	if rec := get(h, "/v1/payday?day=25&year=2026&token="+testToken, nil); rec.Code != 400 {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

func TestHolidaysText(t *testing.T) {
	h := newTestServer(now)
	b := get(h, "/v1/holidays?raw&country=NO&year=2026&token="+testToken, nil).Body.String()
	if !strings.Contains(b, "2026-12-24,Julaften\n") || strings.Contains(b, "2026-12-31") {
		t.Errorf("holidays text:\n%s", b)
	}
}
