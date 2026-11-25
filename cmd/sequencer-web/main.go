package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kostis-codefresh/sequencer/ui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

type pageData struct {
	Version string
	Keys    []apiKey
	Tasks   []task
	KeyName string // create-key form input, kept after a failed submit
	Error   string
	// User is the logged-in GitHub login, or "". The navbar shows Login without it.
	User string
}

// healthStatus is the /health body. Besides liveness it says which OAuth
// settings are missing and which base URL GitHub redirects back to, the two
// things a login that does not work usually comes down to.
type healthStatus struct {
	Status                string   `json:"status"`
	GitHubClientIDSet     bool     `json:"github_client_id_set"`
	GitHubClientSecretSet bool     `json:"github_client_secret_set"`
	Missing               []string `json:"missing"`
	AppBaseURL            string   `json:"app_base_url"`
	GitHubAuth            bool     `json:"github_auth"`
}

// newHandler serves the dashboard. The API keys page and its endpoints need a
// GitHub login; without the OAuth settings in cfg nobody can log in, so they
// are unreachable, unless cfg.AuthDisabled opens them to everyone.
func newHandler(cfg authConfig) http.Handler {
	tmpl := template.Must(template.ParseFS(ui.Embedded, "*.html"))
	static := http.FileServerFS(ui.Embedded)
	keys := newKeyStore()
	tasks := newTaskStore()

	execute := func(w http.ResponseWriter, name string, data pageData) {
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("rendering %s: %v", name, err)
		}
	}
	var auth *githubAuth
	if cfg.enabled() {
		auth = newGitHubAuth(cfg, execute)
	}
	render := func(w http.ResponseWriter, r *http.Request, name string) {
		// login.html is served only by GET /login, which fills in its data.
		if !strings.HasSuffix(name, ".html") || name == "login.html" || tmpl.Lookup(name) == nil {
			http.NotFound(w, r)
			return
		}
		data := pageData{
			Version: version,
			Keys:    keys.list(),
			Tasks:   tasks.list(),
			User:    auth.current(r),
		}
		execute(w, name, data)
	}

	requireLogin := auth.requireLogin
	if cfg.AuthDisabled {
		requireLogin = func(next http.HandlerFunc) http.HandlerFunc { return next }
	}

	mux := http.NewServeMux()
	if auth != nil {
		auth.register(mux)
	} else {
		// The navbar always offers Login, so the page is always there, though
		// without an OAuth app its button leads nowhere.
		mux.HandleFunc("GET /login", loginPage(execute))
	}
	mux.Handle("GET /dashboard.css", static)
	mux.Handle("GET /img/", static)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, "index.html")
	})
	mux.HandleFunc("GET /{page}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, r.PathValue("page"))
	})
	mux.HandleFunc("GET /api-keys.html", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		render(w, r, "api-keys.html")
	}))
	// Liveness only: it touches no store or external service. A missing OAuth
	// setting is reported, not failed on: the public pages work without it.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthStatus{
			Status:                "ok",
			GitHubClientIDSet:     cfg.ClientID != "",
			GitHubClientSecretSet: cfg.ClientSecret != "",
			Missing:               cfg.missing(),
			AppBaseURL:            cfg.BaseURL,
			GitHubAuth:            !cfg.AuthDisabled,
		})
	})
	mux.HandleFunc("GET /debug", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, row := range sysInfo() {
			_, _ = fmt.Fprintf(w, "%s: %s\n", row.Label, row.Value)
		}
	})

	// htmx endpoints: both reply with the re-rendered API keys section.
	// Validation errors use 200 because htmx does not swap 4xx responses by default.
	mux.HandleFunc("POST /api-keys", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		data := pageData{}
		if err := keys.add(r.FormValue("name")); err != nil {
			data.Error = err.Error()
			data.KeyName = r.FormValue("name")
		}
		data.Keys = keys.list()
		execute(w, "api-keys-section", data)
	}))
	mux.HandleFunc("DELETE /api-keys", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		keys.remove(r.FormValue("name"))
		execute(w, "api-keys-section", pageData{Keys: keys.list()})
	}))

	// htmx endpoint for the tasks page's polling refresh.
	mux.HandleFunc("GET /tasks/table", func(w http.ResponseWriter, _ *http.Request) {
		execute(w, "tasks-section", pageData{Tasks: tasks.list()})
	})

	// Every /api/ route requires an API key.
	mux.Handle("/api/", requireAPIKey(keys, newAPIHandler(tasks)))
	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	cfg := authConfigFromEnv()
	if missing := cfg.missing(); len(missing) > 0 {
		log.Printf("%s not set: GitHub login is disabled, so the API keys page is unreachable", strings.Join(missing, " and "))
	}
	if cfg.AuthDisabled {
		log.Print("GITHUB_AUTH=false: the API keys page is open to everyone")
	}
	log.Printf("GitHub OAuth callback URL: %s/callback", cfg.BaseURL)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
