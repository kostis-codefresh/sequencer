package main

import (
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kostis-codefresh/sequencer/ui"
)

type pageData struct {
	GeneratedAt string
	Keys        []apiKey
	KeyName     string // create-key form input, kept after a failed submit
	Error       string
}

func newHandler() http.Handler {
	tmpl := template.Must(template.ParseFS(ui.Embedded, "*.html"))
	static := http.FileServerFS(ui.Embedded)
	keys := newKeyStore()

	execute := func(w http.ResponseWriter, name string, data pageData) {
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("rendering %s: %v", name, err)
		}
	}
	render := func(w http.ResponseWriter, r *http.Request, name string) {
		if !strings.HasSuffix(name, ".html") || tmpl.Lookup(name) == nil {
			http.NotFound(w, r)
			return
		}
		execute(w, name, pageData{
			GeneratedAt: time.Now().UTC().Format("02 Jan 2006 15:04 MST"),
			Keys:        keys.list(),
		})
	}

	mux := http.NewServeMux()
	mux.Handle("GET /dashboard.css", static)
	mux.Handle("GET /img/", static)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, "index.html")
	})
	mux.HandleFunc("GET /{page}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, r.PathValue("page"))
	})

	// htmx endpoints: both reply with the re-rendered API keys section.
	// Validation errors use 200 because htmx does not swap 4xx responses by default.
	mux.HandleFunc("POST /api-keys", func(w http.ResponseWriter, r *http.Request) {
		data := pageData{}
		if err := keys.add(r.FormValue("name")); err != nil {
			data.Error = err.Error()
			data.KeyName = r.FormValue("name")
		}
		data.Keys = keys.list()
		execute(w, "api-keys-section", data)
	})
	mux.HandleFunc("DELETE /api-keys", func(w http.ResponseWriter, r *http.Request) {
		keys.remove(r.FormValue("name"))
		execute(w, "api-keys-section", pageData{Keys: keys.list()})
	})
	return mux
}

func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
