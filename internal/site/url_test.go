package site

import (
	"strings"
	"testing"
)

func TestBaseURLStaysOnGitHubPages(t *testing.T) {
	if BaseURL != "https://stageproof.github.io/index/" {
		t.Fatal(BaseURL)
	}
	if strings.Contains(BaseURL, "stageproof.dev") {
		t.Fatal(BaseURL)
	}
}
