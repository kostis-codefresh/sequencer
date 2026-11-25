package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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

func TestPagesRender(t *testing.T) {
	h := loggedInHandler(t)
	for _, path := range []string{"/", "/tasks.html", "/blueprints.html"} {
		rec := get(t, h, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "{{", path)
	}
}

func TestSidebarActiveLink(t *testing.T) {
	h := loggedInHandler(t)
	pages := map[string]string{
		"/":                  "index.html",
		"/tasks.html":        "tasks.html",
		"/environments.html": "environments.html",
		"/agents.html":       "agents.html",
		"/blueprints.html":   "blueprints.html",
		"/api-keys.html":     "api-keys.html",
	}
	for path, page := range pages {
		rec := get(t, h, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		body := rec.Body.String()
		assert.Equal(t, 1, strings.Count(body, `nav-link-active" href="`+page+`"`), path)
		assert.Equal(t, 1, strings.Count(body, "nav-link-active"), path)
		assert.Contains(t, body, `href="skills.html"`, path)
	}
	assert.Equal(t, http.StatusNotFound, get(t, h, "/partials.tmpl").Code)
}

func TestOverviewVersion(t *testing.T) {
	rec := get(t, loggedInHandler(t), "/")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "<strong>Version:</strong> dev</p>")
	assert.NotContains(t, body, "Goroutines")
	assert.NotContains(t, body, "Project Details")
}

func TestDebug(t *testing.T) {
	rec := get(t, loggedInHandler(t), "/debug")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	assert.Contains(t, body, "Go version: "+runtime.Version()+"\n")
	assert.Contains(t, body, "\nGoroutines: ")
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.5 KiB", formatBytes(1536))
	assert.Equal(t, "2.0 MiB", formatBytes(2<<20))
	assert.Equal(t, "3.0 GiB", formatBytes(3<<30))
}

func TestStaticAssets(t *testing.T) {
	h := loggedInHandler(t)
	for _, path := range []string{"/dashboard.css", "/img/sequencer-logo.png"} {
		assert.Equal(t, http.StatusOK, get(t, h, path).Code, path)
	}
}

func TestUnknownPage(t *testing.T) {
	h := loggedInHandler(t)
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
	h := loggedInHandler(t)
	body := createKey(t, h, "ci-bot")
	assert.Contains(t, body, "<td>ci-bot</td>")
	assert.Contains(t, body, "<code>sk_")
	assert.NotContains(t, body, "alert-danger")

	page := get(t, h, "/api-keys.html").Body.String()
	assert.Contains(t, page, "<td>ci-bot</td>")
	assert.Contains(t, page, "htmx.min.js")
}

// TestKeysOnlyOnSettingsPage guards against a public template (or the shared
// sidebar partial) printing keys: render passes them only to api-keys.html.
func TestKeysOnlyOnSettingsPage(t *testing.T) {
	h := loggedInHandler(t)
	key := regexp.MustCompile(`sk_[0-9a-f]+`).FindString(createKey(t, h, "ci-bot"))
	require.NotEmpty(t, key)

	for _, path := range []string{"/", "/tasks.html", "/environments.html", "/agents.html", "/blueprints.html"} {
		rec := get(t, h, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "sk_", path)
	}
	assert.Contains(t, get(t, h, "/api-keys.html").Body.String(), key)
}

func TestCreateAPIKeyValidation(t *testing.T) {
	h := loggedInHandler(t)
	createKey(t, h, "ci-bot")

	body := createKey(t, h, "ci-bot")
	assert.Contains(t, body, "already exists")
	assert.Equal(t, 1, strings.Count(body, "<td>ci-bot</td>"))

	body = createKey(t, h, "   ")
	assert.Contains(t, body, "name of key is required")
}

func TestDeleteAPIKey(t *testing.T) {
	h := loggedInHandler(t)
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

func TestHealth(t *testing.T) {
	testCases := []struct {
		name string
		cfg  authConfig
		want string
	}{
		{
			name: "configured",
			cfg:  testAuthConfig,
			want: `{"status":"ok","github_client_id_set":true,"github_client_secret_set":true,"missing":[],"app_base_url":"http://localhost:8080","github_auth":true}`,
		},
		{
			name: "missing settings",
			cfg:  authConfig{BaseURL: "https://sequencer.example.com"},
			want: `{"status":"ok","github_client_id_set":false,"github_client_secret_set":false,"missing":["GITHUB_CLIENT_ID","GITHUB_CLIENT_SECRET"],"app_base_url":"https://sequencer.example.com","github_auth":true}`,
		},
		{
			name: "auth disabled",
			cfg:  authConfig{BaseURL: "http://localhost:8080", AuthDisabled: true},
			want: `{"status":"ok","github_client_id_set":false,"github_client_secret_set":false,"missing":["GITHUB_CLIENT_ID","GITHUB_CLIENT_SECRET"],"app_base_url":"http://localhost:8080","github_auth":false}`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, newHandler(tc.cfg), "/health")
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tc.want, rec.Body.String())
		})
	}
}
