package calendar

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func TestNorwayBankingDays(t *testing.T) {
	no, err := Get("NO")
	if err != nil {
		t.Fatal(err)
	}
	closed := []time.Time{
		d(2026, 1, 1),   // Første nyttårsdag
		d(2026, 4, 2),   // Skjærtorsdag (Easter 2026-04-05)
		d(2026, 4, 3),   // Langfredag
		d(2026, 4, 6),   // Andre påskedag
		d(2026, 5, 1),   // Arbeidernes dag
		d(2026, 5, 14),  // Kristi himmelfartsdag
		d(2026, 5, 25),  // Andre pinsedag
		d(2026, 12, 24), // Julaften: NBO closed (Banko missed this)
		d(2026, 12, 25),
		d(2027, 5, 17),  // Grunnlovsdag on a Monday
		d(2027, 3, 25),  // Skjærtorsdag (Easter 2027-03-28)
		d(2026, 11, 21), // Saturday
	}
	for _, day := range closed {
		if no.IsBankingDay(day) {
			t.Errorf("%s should not be a banking day", day.Format("2006-01-02 Mon"))
		}
	}
	open := []time.Time{
		d(2026, 12, 31), // Nyttårsaften: NBO open (Banko had it closed)
		d(2026, 12, 23),
		d(2026, 4, 1), // Onsdag før skjærtorsdag
		d(2026, 11, 17),
	}
	for _, day := range open {
		if !no.IsBankingDay(day) {
			t.Errorf("%s should be a banking day", day.Format("2006-01-02 Mon"))
		}
	}
}

func TestGetAliases(t *testing.T) {
	for _, id := range []string{"NO", "no", " No ", "norway", "Norway", "norge"} {
		c, err := Get(id)
		if err != nil || c.Code != "NO" {
			t.Errorf("Get(%q) = %v, %v", id, c, err)
		}
	}
	if _, err := Get("XX"); err == nil {
		t.Error("Get(XX) should fail")
	}
}

func TestHolidaysIncludeKinds(t *testing.T) {
	no, _ := Get("NO")
	hs := no.Holidays(2026)
	var julaften *Holiday
	for i := range hs {
		if hs[i].Date.Equal(d(2026, 12, 24)) {
			julaften = &hs[i]
		}
		if hs[i].Date.Equal(d(2026, 12, 31)) {
			t.Error("nyttårsaften must not be listed")
		}
	}
	if julaften == nil || julaften.Kind != "bank" {
		t.Fatalf("julaften missing or wrong kind: %+v", julaften)
	}
	if len(hs) == 0 || hs[0].Name != "Første nyttårsdag" || hs[0].Kind != "public" {
		t.Errorf("first holiday = %+v", hs[0])
	}
}
