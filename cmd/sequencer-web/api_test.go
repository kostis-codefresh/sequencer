package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var keyValue = regexp.MustCompile(`sk_[0-9a-f]{32}`)

// newKey creates an API key through the UI endpoint and returns its value.
func newKey(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	key := keyValue.FindString(createKey(t, h, name))
	require.NotEmpty(t, key)
	return key
}

func apiCall(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, `Bearer realm="sequencer"`, rec.Header().Get("WWW-Authenticate"))
	assert.JSONEq(t, `{"error":"invalid or missing API key"}`, rec.Body.String())
}

func TestAPIRequiresKey(t *testing.T) {
	h := newHandler()
	key := newKey(t, h, "ci")

	assertUnauthorized(t, apiCall(t, h, http.MethodGet, "/api/tasks", "", ""))
	assertUnauthorized(t, apiCall(t, h, http.MethodGet, "/api/tasks", "sk_wrong", ""))
	assertUnauthorized(t, apiCall(t, h, http.MethodGet, "/api/tasks?api_key="+key, "", ""))
	assertUnauthorized(t, apiCall(t, h, http.MethodPost, "/api/tasks", "", `{"name":"a","user":"b"}`))
	assertUnauthorized(t, apiCall(t, h, http.MethodGet, "/api/unknown", "", ""))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Header.Set("Authorization", "Basic "+key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assertUnauthorized(t, rec)

	ok := apiCall(t, h, http.MethodGet, "/api/tasks", key, "")
	assert.Equal(t, http.StatusOK, ok.Code)
	assert.Equal(t, "no-store", ok.Header().Get("Cache-Control"))
}

func TestAPIDeletedKeyIsRevoked(t *testing.T) {
	h := newHandler()
	key := newKey(t, h, "ci")
	require.Equal(t, http.StatusOK, apiCall(t, h, http.MethodGet, "/api/tasks", key, "").Code)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api-keys?name=ci", nil))
	assertUnauthorized(t, apiCall(t, h, http.MethodGet, "/api/tasks", key, ""))
}

func TestAPITasksLifecycle(t *testing.T) {
	h := newHandler()
	key := newKey(t, h, "ci")

	rec := apiCall(t, h, http.MethodPost, "/api/tasks", key, `{"name":"build","user":"kostis"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "/api/tasks/1", rec.Header().Get("Location"))
	var created task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, 1, created.ID)
	assert.Equal(t, "build", created.Name)
	assert.Equal(t, "kostis", created.User)

	rec = apiCall(t, h, http.MethodGet, "/api/tasks", key, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var listed struct{ Tasks []task }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Tasks, 1)
	assert.Equal(t, "build", listed.Tasks[0].Name)

	assert.Contains(t, get(t, h, "/tasks.html").Body.String(), "<td>build</td>")

	assert.Equal(t, http.StatusNoContent, apiCall(t, h, http.MethodDelete, "/api/tasks/1", key, "").Code)
	assert.Equal(t, http.StatusNotFound, apiCall(t, h, http.MethodDelete, "/api/tasks/1", key, "").Code)
	assert.Equal(t, http.StatusNotFound, apiCall(t, h, http.MethodDelete, "/api/tasks/abc", key, "").Code)
}

func TestAPICreateTaskValidation(t *testing.T) {
	h := newHandler()
	key := newKey(t, h, "ci")

	for name, body := range map[string]string{
		"malformed":     `{"name":`,
		"unknown field": `{"name":"a","user":"b","extra":1}`,
		"missing user":  `{"name":"a"}`,
		"blank name":    `{"name":"  ","user":"b"}`,
	} {
		rec := apiCall(t, h, http.MethodPost, "/api/tasks", key, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		assert.Contains(t, rec.Body.String(), `"error"`, name)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"name":"a","user":"b"}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestTasksPageDelete(t *testing.T) {
	h := newHandler()
	key := newKey(t, h, "ci")
	apiCall(t, h, http.MethodPost, "/api/tasks", key, `{"name":"build","user":"kostis"}`)
	apiCall(t, h, http.MethodPost, "/api/tasks", key, `{"name":"test","user":"kostis"}`)

	table := get(t, h, "/tasks/table").Body.String()
	assert.Contains(t, table, `hx-delete="/tasks?id=1"`)
	assert.NotContains(t, table, "<html")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/tasks?id=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<td>build</td>")
	assert.Contains(t, rec.Body.String(), "<td>test</td>")
}
