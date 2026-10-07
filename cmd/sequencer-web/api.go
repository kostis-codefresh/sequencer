package main

import (
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

const maxBodyBytes = 1 << 20

// requireAPIKey rejects requests without a valid "Authorization: Bearer <key>" header.
// Every failure gets the same response so callers cannot tell which part was wrong.
func requireAPIKey(keys *keyStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		if !strings.EqualFold(scheme, "Bearer") || !keys.valid(token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="sequencer"`)
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newAPIHandler(tasks *taskStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]task{"tasks": tasks.list()})
	})

	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
		var req struct {
			Name string `json:"name"`
			User string `json:"user"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		t, err := tasks.add(req.Name, req.User)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Location", "/api/tasks/"+strconv.Itoa(t.ID))
		writeJSON(w, http.StatusCreated, t)
	})

	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || !tasks.remove(id) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writing JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
