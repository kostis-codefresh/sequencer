package main

import (
	"net/http"
	"net/http/httptest"
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
	for _, path := range []string{"/", "/tasks.html"} {
		rec := get(t, h, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Contains(t, rec.Body.String(), "UTC</span>", path)
		assert.NotContains(t, rec.Body.String(), "{{", path)
	}
}

func TestStaticAssets(t *testing.T) {
	h := newHandler()
	for _, path := range []string{"/dashboard.css", "/img/sequencer-logo.png"} {
		assert.Equal(t, http.StatusOK, get(t, h, path).Code, path)
	}
}

func TestUnknownPage(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, get(t, newHandler(), "/nope.html").Code)
}
