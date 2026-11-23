// Package pairs chooses the stable version pairs the index builds.
package pairs

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Version is one published chart version.
type Version struct {
	Name    string
	Created time.Time
}

// Pair is an upgrade from one stable version to a newer one.
type Pair struct {
	From string
	To   string
}

// Stable returns the stable versions, oldest first.
func Stable(versions []Version) []Version {
	stable := make([]Version, 0, len(versions))
	for _, v := range versions {
		if isStable(v.Name) {
			stable = append(stable, v)
		}
	}
	sort.Slice(stable, func(i, j int) bool {
		return semver.Compare(canon(stable[i].Name), canon(stable[j].Name)) < 0
	})
	return stable
}

// Enumerate returns the pairs for one chart.
// Versions inside the last 24 months are kept. When no version has a created
// time, the newest 20 stable versions are kept instead.
// Pairs are every adjacent step in that set, plus a jump to the newest stable
// version from each of the newest 20 stable versions.
func Enumerate(versions []Version, now time.Time) []Pair {
	stable := Stable(versions)
	dated := false
	for _, v := range stable {
		if !v.Created.IsZero() {
			dated = true
		}
	}
	if len(stable) == 0 {
		return nil
	}
	window := stable
	if dated {
		cutoff := now.AddDate(-2, 0, 0)
		window = make([]Version, 0, len(stable))
		for _, v := range stable {
			if !v.Created.IsZero() && !v.Created.Before(cutoff) {
				window = append(window, v)
			}
		}
	} else if len(window) > 20 {
		window = window[len(window)-20:]
	}
	latest := stable[len(stable)-1].Name
	seen := map[string]struct{}{}
	var out []Pair
	add := func(from, to string) {
		if from == to || semver.Compare(canon(from), canon(to)) >= 0 {
			return
		}
		key := from + "\x00" + to
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, Pair{From: from, To: to})
	}
	for i := 0; i+1 < len(window); i++ {
		add(window[i].Name, window[i+1].Name)
	}
	start := 0
	if len(stable) > 20 {
		start = len(stable) - 20
	}
	for _, v := range stable[start:] {
		add(v.Name, latest)
	}
	sort.Slice(out, func(i, j int) bool {
		if c := semver.Compare(canon(out[i].From), canon(out[j].From)); c != 0 {
			return c < 0
		}
		return semver.Compare(canon(out[i].To), canon(out[j].To)) < 0
	})
	return out
}

func isStable(v string) bool {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return false
	}
	if !strings.HasPrefix(raw, "v") {
		raw = "v" + raw
	}
	if !semver.IsValid(raw) || semver.Prerelease(raw) != "" || semver.Build(raw) != "" {
		return false
	}
	return canon(v) != ""
}

func canon(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return semver.Canonical(v)
}
