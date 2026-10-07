package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPagesRenderGenerationDate(t *testing.T) {
	h := newHandler()
	for _, path := range []string{"/", "/tasks.html", "/blueprints.html"} {
		rec := get(t, h, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Contains(t, rec.Body.String(), "UTC</span>", path)
		assert.NotContains(t, rec.Body.String(), "{{", path)
	}
}

func TestOverviewSystemInfo(t *testing.T) {
	rec := get(t, newHandler(), "/")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "System info")
	assert.Contains(t, body, "<strong>Goroutines:</strong>")
	assert.Contains(t, body, runtime.Version())
	assert.NotContains(t, body, "Project Details")
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.5 KiB", formatBytes(1536))
	assert.Equal(t, "2.0 MiB", formatBytes(2<<20))
	assert.Equal(t, "3.0 GiB", formatBytes(3<<30))
}

func TestStaticAssets(t *testing.T) {
	h := newHandler()
	for _, path := range []string{"/dashboard.css", "/img/sequencer-logo.png"} {
		assert.Equal(t, http.StatusOK, get(t, h, path).Code, path)
	}
}

func TestUnknownPage(t *testing.T) {
	h := newHandler()
	assert.Equal(t, http.StatusNotFound, get(t, h, "/nope.html").Code)
	assert.Equal(t, http.StatusNotFound, get(t, h, "/api-keys-section").Code)
}

func createKey(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api-keys", strings.NewReader(url.Values{"name": {name}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

func TestCreateAPIKey(t *testing.T) {
	h := newHandler()
	body := createKey(t, h, "ci-bot")
	assert.Contains(t, body, "<td>ci-bot</td>")
	assert.Contains(t, body, "<code>sk_")
	assert.NotContains(t, body, "alert-danger")

	page := get(t, h, "/api-keys.html").Body.String()
	assert.Contains(t, page, "<td>ci-bot</td>")
	assert.Contains(t, page, "htmx.min.js")
}

func TestCreateAPIKeyValidation(t *testing.T) {
	h := newHandler()
	createKey(t, h, "ci-bot")

	body := createKey(t, h, "ci-bot")
	assert.Contains(t, body, "already exists")
	assert.Equal(t, 1, strings.Count(body, "<td>ci-bot</td>"))

	body = createKey(t, h, "   ")
	assert.Contains(t, body, "name of key is required")
}

func TestDeleteAPIKey(t *testing.T) {
	h := newHandler()
	createKey(t, h, "ci bot")
	createKey(t, h, "deployer")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api-keys?name=ci+bot", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<td>ci bot</td>")
	assert.Contains(t, rec.Body.String(), "<td>deployer</td>")

	body := createKey(t, h, "a&b")
	assert.Contains(t, body, `hx-delete="/api-keys?name=a%26b"`)
}
