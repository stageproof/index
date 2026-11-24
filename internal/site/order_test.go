package site

import "testing"

func TestNewerOrdersSemver(t *testing.T) {
	if !newer("1.10.0", "1.9.0") || !newer("v1.2.0", "1.1.9") || newer("1.0.0", "1.0.1") {
		t.Fatal("semver order")
	}
}
