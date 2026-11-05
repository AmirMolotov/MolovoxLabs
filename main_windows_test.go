//go:build windows

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	h := assetHandler("/session/")
	for _, path := range []string{"/session/", "/session/app.js", "/session/routing.js", "/session/dsp.js", "/session/style.css", "/session/studio.html", "/session/studio.js", "/session/studio-controls.js", "/session/studio-vst.js", "/session/studio.css", "/session/studio-spacing.css"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
	for _, path := range []string{"/", "/app.js", "/session/../go.mod", "/session/missing.js"} {
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
