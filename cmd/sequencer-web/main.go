package main

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/kostis-codefresh/sequencer/ui"
)

func newHandler() http.Handler {
	tmpl := template.Must(template.ParseFS(ui.Embedded, "*.html"))
	static := http.FileServerFS(ui.Embedded)

	render := func(w http.ResponseWriter, r *http.Request, name string) {
		t := tmpl.Lookup(name)
		if t == nil {
			http.NotFound(w, r)
			return
		}
		data := struct{ GeneratedAt string }{time.Now().UTC().Format("02 Jan 2006 15:04 MST")}
		if err := t.Execute(w, data); err != nil {
			log.Printf("rendering %s: %v", name, err)
		}
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
