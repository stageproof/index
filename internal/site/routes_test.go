package site

import (
	"strings"
	"testing"
)

func TestRoutesStayFrozen(t *testing.T) {
	if Root != "/" || Charts != "/charts/" || API != "/api/" || APIIndex != "/api/v1/index.json" || Sitemap != "/sitemap.xml" {
		t.Fatal("root routes changed")
	}
	chart, err := Chart("fixtures", "web")
	if err != nil || chart != "/chart/fixtures/web/" {
		t.Fatalf("chart %s %v", chart, err)
	}
	up, err := Upgrade("fixtures", "web", "0.1.0", "0.2.0")
	if err != nil || up != "/upgrade/fixtures/web/0.1.0/0.2.0/" {
		t.Fatalf("upgrade %s %v", up, err)
	}
	api, err := APIUpgrade("fixtures", "web", "0.1.0", "0.2.0")
	if err != nil || api != "/api/v1/upgrade/fixtures/web/0.1.0/0.2.0.json" {
		t.Fatalf("api %s %v", api, err)
	}
	if Href(Charts) != "/index/charts/" || Absolute(Charts) != "https://stageproof.github.io/index/charts/" {
		t.Fatalf("href %s absolute %s", Href(Charts), Absolute(Charts))
	}
	bug := BugURL("fixtures", "web", "0.1.0", "0.2.0", "v0.3.0")
	for _, part := range []string{"template=bug.yml", "fixtures", "web", "0.1.0", "0.2.0", "v0.3.0"} {
		if !strings.Contains(bug, part) {
			t.Fatalf("bug url missing %s: %s", part, bug)
		}
	}
	if strings.Contains(bug, "stageproof.dev") {
		t.Fatal(bug)
	}
}

func TestRoutesRejectParentSegments(t *testing.T) {
	cases := [][4]string{
		{"../etc", "web", "0.1.0", "0.2.0"},
		{"fixtures", "..", "0.1.0", "0.2.0"},
		{"fixtures", "web", "../passwd", "0.2.0"},
		{"fixtures", "web", "0.1.0", "/etc"},
		{"fixtures", "web/x", "0.1.0", "0.2.0"},
	}
	for _, c := range cases {
		if _, err := Upgrade(c[0], c[1], c[2], c[3]); err == nil {
			t.Fatalf("accepted %q", c)
		}
	}
}
