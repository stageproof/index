package release

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileMatchesSHA256RejectsTamperedBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stageproof")
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fileMatchesSHA256(path, LinuxAMD64BinarySHA256) {
		t.Fatal("tampered binary matched pinned hash")
	}
}

func TestEnsureDoesNotReuseTamperedCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stageproof")
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	// Client fails instead of trusting the tampered cached binary.
	client := &http.Client{Transport: roundTripError{}}
	if _, err := Ensure(client, "", dir); err == nil {
		t.Fatal("Ensure reused tampered cached binary")
	}
}

type roundTripError struct{}

func (roundTripError) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrHandlerTimeout
}
