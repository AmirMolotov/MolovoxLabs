//go:build windows

package main

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"molovoxlabs/web"
)

func TestEmbeddedAssets(t *testing.T) {
	h := assetHandler("/session/")
	if err := fs.WalkDir(web.Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", "/session/"+strings.TrimSuffix(path, "index.html"), nil))
		if response.Code != 200 {
			t.Errorf("%s returned %d", path, response.Code)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/", "/app.js", "/session/../go.mod", "/session/missing.js", "/session/assets.go", "/session/go.mod"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 404 {
			t.Fatalf("%s should be unavailable, got %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/session/", nil))
	if !strings.Contains(response.Body.String(), "id=\"polarityTest\"") {
		t.Fatal("Missing inversion diagnostic")
	}
}
