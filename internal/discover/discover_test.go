package discover

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunFiltersDeprecatedBitnamiAndDuplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/packages/search":
			_, _ = w.Write([]byte(`{"packages":[
				{"name":"old","deprecated":true,"stars":100,"repository":{"name":"acme","url":"https://charts.example/old"}},
				{"name":"wordpress","deprecated":false,"stars":90,"repository":{"name":"bitnami","url":"https://charts.bitnami.com/bitnami"}},
				{"name":"web","deprecated":false,"stars":80,"repository":{"name":"acme","url":"https://charts.example"}},
				{"name":"web","deprecated":false,"stars":80,"repository":{"name":"acme","url":"https://charts.example"}},
				{"name":"db","deprecated":false,"stars":70,"repository":{"name":"acme","url":"https://charts.example"}}
			]}`))
		case "/api/v1/packages/helm/acme/web":
			_, _ = w.Write([]byte(`{"home_url":"https://example.com/web","links":[{"name":"Source","url":"https://example.com/src"},{"name":"Release notes","url":"https://example.com/releases"}]}`))
		case "/api/v1/packages/helm/acme/db":
			_, _ = w.Write([]byte(`{"home_url":"https://example.com/db","links":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	file, err := Run(Options{BaseURL: srv.URL, Client: srv.Client(), Limit: 300})
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Charts) != 2 {
		t.Fatalf("got %#v", file.Charts)
	}
	if file.Charts[0].Chart != "web" || file.Charts[0].Stars != 80 {
		t.Fatalf("first %#v", file.Charts[0])
	}
	if file.Charts[0].Homepage != "https://example.com/web" || file.Charts[0].ReleaseNotesURL != "https://example.com/releases" {
		t.Fatalf("links %#v", file.Charts[0])
	}
	if file.Charts[1].Chart != "db" || file.Charts[1].ReleaseNotesURL != "" {
		t.Fatalf("second %#v", file.Charts[1])
	}
}
