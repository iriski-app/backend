package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLoadsCBRURLFromEnvFile(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Query().Get("date_req") != "17/11/2026" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("CBR_BASE_URL="+server.URL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	previous, exists := os.LookupEnv("CBR_BASE_URL")
	if err := os.Unsetenv("CBR_BASE_URL"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv("CBR_BASE_URL", previous)
		} else {
			_ = os.Unsetenv("CBR_BASE_URL")
		}
	})
	args := os.Args
	os.Args = []string{"fx-collector", "--date", "2026-11-17"}
	t.Cleanup(func() { os.Args = args })
	err := run()
	if !called || err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("called=%v, error=%v", called, err)
	}
}
