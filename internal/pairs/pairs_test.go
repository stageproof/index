package pairs

import (
	"testing"
	"time"
)

func TestEnumerateAdjacentAndLatest(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	got := Enumerate([]Version{
		{Name: "1.0.0-rc.1", Created: now.AddDate(0, -1, 0)},
		{Name: "1.0.0", Created: now.AddDate(0, -1, 0)},
		{Name: "1.1.0", Created: now.AddDate(0, -2, 0)},
		{Name: "1.2.0+build.1", Created: now.AddDate(0, -1, 0)},
		{Name: "0.9.0", Created: now.AddDate(-3, 0, 0)},
		{Name: "1.2.0", Created: now.AddDate(0, -1, 0)},
	}, now)
	want := []Pair{
		{From: "0.9.0", To: "1.2.0"},
		{From: "1.0.0", To: "1.1.0"},
		{From: "1.0.0", To: "1.2.0"},
		{From: "1.1.0", To: "1.2.0"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pair %d got %#v want %#v", i, got[i], want[i])
		}
	}
}

func TestEnumerateFallsBackWhenDatesAreAbsent(t *testing.T) {
	versions := make([]Version, 0, 25)
	for i := 1; i <= 25; i++ {
		versions = append(versions, Version{Name: itoa(i) + ".0.0"})
	}
	got := Enumerate(versions, time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC))
	if len(got) == 0 {
		t.Fatal("expected pairs")
	}
	if got[0].From == "1.0.0" {
		t.Fatalf("oldest version was kept: %#v", got[0])
	}
	found := false
	for _, p := range got {
		if p.From == "6.0.0" && p.To == "7.0.0" {
			found = true
		}
		if p.To == "25.0.0" && p.From == "24.0.0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing an expected pair in %#v", got[:3])
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
