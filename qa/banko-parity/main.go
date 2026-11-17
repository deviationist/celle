// Command banko-parity compares celle with Banko's /api/utils endpoints.
//
//	go run ./qa/banko-parity gen > cases.jsonl
//	(cd <banko checkout> && php <celle>/qa/banko-parity/banko-harness.php) < cases.jsonl > banko.jsonl
//	go run ./qa/banko-parity compare cases.jsonl banko.jsonl
//
// Every case pins "now", runs the same question through both, and the compare
// step groups the differences by cause. See README.md for the findings.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deviationist/celle/internal/api"
	"github.com/deviationist/celle/internal/dates"
)

type Case struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Now   string `json:"now"`
	Path  string `json:"path"`  // Banko
	Celle string `json:"celle"` // celle
}

type Result struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

const token = "parity-token-0123456789"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: banko-parity gen | compare cases.jsonl banko.jsonl")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "gen":
		gen(os.Stdout)
	case "compare":
		if len(os.Args) != 4 {
			fmt.Fprintln(os.Stderr, "usage: banko-parity compare cases.jsonl banko.jsonl")
			os.Exit(2)
		}
		os.Exit(compare(os.Args[2], os.Args[3]))
	default:
		fmt.Fprintln(os.Stderr, "unknown command", os.Args[1])
		os.Exit(2)
	}
}

// Instants used as "now". The last two are just before and just after
// midnight in Oslo, where Banko (UTC) and celle (Europe/Oslo) disagree.
var nows = []string{
	"2026-11-17T10:00:00Z",
	"2026-02-28T12:00:00Z",
	"2027-12-31T08:00:00Z",
	"2026-11-17T22:30:00Z", // 23:30 Oslo, same day in both
	"2026-11-17T23:30:00Z", // 00:30 Oslo on the 18th, Banko still on the 17th
}

func gen(w io.Writer) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	n := 0
	emit := func(kind, now, banko, celle string) {
		n++
		_ = enc.Encode(Case{ID: fmt.Sprint(n), Kind: kind, Now: now, Path: banko, Celle: celle})
	}

	// 1. Roll to the previous banking day: every day 2020–2035.
	for d := dates.Date(2020, 1, 1); d.Year() <= 2035; d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		emit("roll", nows[0],
			"/api/utils/bank-holiday-calc?raw&country=norway&date="+ds,
			"/v1/banking-day?raw&country=NO&date="+ds)
	}

	// 2. Payday in an explicit month, with and without a country.
	days := []string{"last"}
	for i := 1; i <= 31; i++ {
		days = append(days, fmt.Sprint(i))
	}
	for y := 2020; y <= 2035; y++ {
		for m := 1; m <= 12; m++ {
			for _, day := range days {
				q := fmt.Sprintf("year=%d&month=%d", y, m)
				bday := day
				if day == "last" {
					bday = "last-in-month"
				}
				emit("payday", nows[0],
					fmt.Sprintf("/api/utils/next-payment-date?raw&%s&payment_day=%s", q, bday),
					fmt.Sprintf("/v1/payday?raw&%s&day=%s", q, day))
				emit("payday-no", nows[0],
					fmt.Sprintf("/api/utils/next-payment-date?raw&%s&payment_day=%s&country=norway", q, bday),
					fmt.Sprintf("/v1/payday?raw&%s&day=%s&country=NO", q, day))
			}
		}
	}

	// 3. Days until a date, around each "now".
	for _, now := range nows {
		t, _ := time.Parse(time.RFC3339, now)
		for off := -400; off <= 400; off += 3 {
			ds := t.AddDate(0, 0, off).Format("2006-01-02")
			emit("days-until", now,
				"/api/utils/days-until-date?raw&date="+ds,
				"/v1/days-until?raw&date="+ds)
		}
	}

	// 4. Days until payday in an explicit month, around each "now".
	for _, now := range nows {
		t, _ := time.Parse(time.RFC3339, now)
		for off := -14; off <= 14; off++ {
			mt := t.AddDate(0, off, 0)
			for _, day := range []string{"1", "15", "25", "30", "31", "last"} {
				bday := day
				if day == "last" {
					bday = "last-in-month"
				}
				q := fmt.Sprintf("year=%d&month=%d", mt.Year(), int(mt.Month()))
				emit("days-until-payday", now,
					fmt.Sprintf("/api/utils/days-until-payment-date?raw&%s&payment_day=%s&country=norway", q, bday),
					fmt.Sprintf("/v1/days-until-payday?raw&%s&day=%s&country=NO", q, day))
			}
		}
	}
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func runCelle(c Case) (int, string) {
	now, _ := time.Parse(time.RFC3339, c.Now)
	oslo, _ := time.LoadLocation("Europe/Oslo")
	h := api.New(api.Config{Version: "parity", DefaultTZ: oslo, Tokens: []string{token}, Now: func() time.Time { return now }})
	req := httptest.NewRequest(http.MethodGet, c.Celle+"&token="+url.QueryEscape(token), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

type diff struct {
	c            Case
	banko, celle string
}

func compare(casesPath, bankoPath string) int {
	cases, err := readJSONL[Case](casesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	results, err := readJSONL[Result](bankoPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	byID := map[string]Result{}
	for _, r := range results {
		byID[r.ID] = r
	}

	total := map[string]int{}
	groups := map[string][]diff{}
	for _, c := range cases {
		total[c.Kind]++
		r, ok := byID[c.ID]
		if !ok {
			groups[c.Kind+": no Banko result"] = append(groups[c.Kind+": no Banko result"], diff{c: c})
			continue
		}
		bBody := strings.TrimSpace(r.Body)
		if r.Status != 200 {
			bBody = fmt.Sprintf("HTTP %d %s", r.Status, bBody)
		}
		code, cBody := runCelle(c)
		if code != 200 {
			cBody = fmt.Sprintf("HTTP %d %s", code, cBody)
		}
		if bBody == cBody {
			continue
		}
		key := c.Kind + ": " + classify(c, bBody, cBody)
		groups[key] = append(groups[key], diff{c: c, banko: bBody, celle: cBody})
	}

	kinds := make([]string, 0, len(total))
	for k := range total {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Println("Cases per kind:")
	for _, k := range kinds {
		fmt.Printf("  %-18s %6d\n", k, total[k])
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\nDifferences: %d groups\n", len(keys))
	unexplained := 0
	for _, k := range keys {
		ds := groups[k]
		fmt.Printf("\n## %s (%d)\n", k, len(ds))
		if strings.HasSuffix(k, "unexplained") {
			unexplained += len(ds)
		}
		for i, d := range ds {
			if i == 6 {
				fmt.Printf("  … %d more\n", len(ds)-i)
				break
			}
			fmt.Printf("  now=%s  %s\n    banko=%q  celle=%q\n", d.c.Now, d.c.Celle, d.banko, d.celle)
		}
	}
	if unexplained > 0 {
		fmt.Printf("\n%d unexplained differences\n", unexplained)
		return 1
	}
	fmt.Println("\nNo unexplained differences.")
	return 0
}

// classify names the known cause of a difference, or "unexplained".
func classify(c Case, banko, celle string) string {
	u, _ := url.Parse(c.Celle)
	q := u.Query()
	now, _ := time.Parse(time.RFC3339, c.Now)
	oslo, _ := time.LoadLocation("Europe/Oslo")
	tzShift := now.UTC().Day() != now.In(oslo).Day()

	switch c.Kind {
	case "roll", "payday", "payday-no":
		bd, errB := time.Parse("2006-01-02", banko)
		_, errC := time.Parse("2006-01-02", celle)
		if errB != nil || errC != nil {
			return "unexplained"
		}
		if c.Kind != "roll" {
			if int(bd.Month()) != atoi(q.Get("month")) {
				return "Banko overflows into the next month (day past month end)"
			}
		}
		if bd.Month() == time.December && bd.Day() == 24 {
			return "Banko pays on julaften (banks closed)"
		}
		if cd, err := time.Parse("2006-01-02", celle); err == nil && cd.Month() == time.December && cd.Day() == 31 {
			return "Banko skips nyttårsaften (banks open in Norway)"
		}
		return "unexplained"
	case "days-until":
		if tzShift {
			return "time zone: Banko counts from the UTC date, celle from Oslo"
		}
		return "unexplained"
	case "days-until-payday":
		if banko == strings.TrimPrefix(celle, "-") && strings.HasPrefix(celle, "-") {
			return "Banko reports a passed payday as positive (sign flipped twice)"
		}
		if tzShift {
			return "time zone: Banko counts from the UTC date, celle from Oslo"
		}
		// Overflow or julaften in the payday itself shows up as a different count.
		return "different payday (see payday groups)"
	}
	return "unexplained"
}

func atoi(s string) int {
	n := 0
	fmt.Sscan(s, &n)
	return n
}
