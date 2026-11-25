package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// allowedEmailDomain is the only email domain allowed to log in.
const allowedEmailDomain = "@octopus.com"

const sessionName = "sequencer-session"

// authConfig is the GitHub OAuth app the web app logs users in with. Login
// needs both the client ID and the secret; without them nobody can log in.
type authConfig struct {
	ClientID     string
	ClientSecret string
	// BaseURL is where GitHub sends the user back to, as BaseURL + "/callback".
	BaseURL string
	// SessionKey is the secret the session cookie's signing and encryption
	// keys are derived from.
	SessionKey []byte
	// AuthDisabled (GITHUB_AUTH=false) opens the API keys page to everyone,
	// for testing locally without an OAuth app. Login itself is unaffected.
	AuthDisabled bool

	// githubURL and apiURL default to github.com and api.github.com. The tests
	// point them at a stub.
	githubURL string
	apiURL    string
}

func authConfigFromEnv() authConfig {
	cfg := authConfig{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		BaseURL:      os.Getenv("APP_BASE_URL"),
		SessionKey:   []byte(os.Getenv("SESSION_KEY")),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	if v := os.Getenv("GITHUB_AUTH"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("GITHUB_AUTH=%q is not a boolean", v)
		}
		cfg.AuthDisabled = !enabled
	}
	if len(cfg.SessionKey) == 0 {
		// A random key rather than a hard-coded one: sessions do not survive a
		// restart, but nobody can forge a cookie from a key in the source.
		cfg.SessionKey = make([]byte, 32)
		if _, err := rand.Read(cfg.SessionKey); err != nil {
			log.Fatalf("generating a session key: %v", err)
		}
		log.Print("SESSION_KEY is not set: using a random key, so logins do not survive a restart")
	}
	return cfg
}

// missing names the OAuth settings that are not set.
func (c authConfig) missing() []string {
	missing := []string{}
	if c.ClientID == "" {
		missing = append(missing, "GITHUB_CLIENT_ID")
	}
	if c.ClientSecret == "" {
		missing = append(missing, "GITHUB_CLIENT_SECRET")
	}
	return missing
}

func (c authConfig) enabled() bool {
	return len(c.missing()) == 0
}

// newSessionStore derives separate signing and encryption keys from the
// session key. The cookie carries the user's GitHub token (for revoking it at
// logout), so it is encrypted, not just signed.
func newSessionStore(cfg authConfig) *sessions.CookieStore {
	hashKey := sha256.Sum256(append([]byte("hash:"), cfg.SessionKey...))
	blockKey := sha256.Sum256(append([]byte("block:"), cfg.SessionKey...))
	store := sessions.NewCookieStore(hashKey[:], blockKey[:])
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   strings.HasPrefix(cfg.BaseURL, "https://"),
		// Lax, not Strict: the redirect back from GitHub is a cross-site
		// top-level GET, and it must carry the cookie holding the OAuth state.
		SameSite: http.SameSiteLaxMode,
	}
	return store
}

// githubAuth logs users in with GitHub. A nil *githubAuth is the app without
// OAuth configured: nobody is ever logged in.
type githubAuth struct {
	oauth  *oauth2.Config
	store  *sessions.CookieStore
	apiURL string
	render func(w http.ResponseWriter, name string, data pageData)
}

func newGitHubAuth(cfg authConfig, render func(w http.ResponseWriter, name string, data pageData)) *githubAuth {
	endpoint := github.Endpoint
	if cfg.githubURL != "" {
		endpoint = oauth2.Endpoint{
			AuthURL:  cfg.githubURL + "/login/oauth/authorize",
			TokenURL: cfg.githubURL + "/login/oauth/access_token",
		}
	}
	apiURL := cfg.apiURL
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	return &githubAuth{
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			// Read access to the profile and the email addresses: the login
			// and the domain check.
			Scopes:      []string{"read:user", "user:email"},
			Endpoint:    endpoint,
			RedirectURL: cfg.BaseURL + "/callback",
		},
		store:  newSessionStore(cfg),
		apiURL: apiURL,
		render: render,
	}
}

// loginPage serves the Octopus-branded login page.
func loginPage(render func(w http.ResponseWriter, name string, data pageData)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		render(w, "login.html", pageData{Version: version})
	}
}

func (a *githubAuth) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", loginPage(a.render))
	mux.HandleFunc("GET /login/github", a.handleLogin)
	mux.HandleFunc("GET /callback", a.handleCallback)
	mux.HandleFunc("POST /logout", a.handleLogout)
}

// current is the logged-in user's GitHub login, or "".
func (a *githubAuth) current(r *http.Request) string {
	if a == nil {
		return ""
	}
	// An undecodable cookie (an old key, tampering) is just a new session.
	session, _ := a.store.Get(r, sessionName)
	login, _ := session.Values["username"].(string)
	return login
}

// requireLogin lets only logged-in users through to next. Anyone else is sent
// to the login page: htmx requests get HX-Redirect with a 200, because htmx
// does not act on a 3xx or 4xx, and everything else a 303. Without OAuth
// configured nobody can log in, so the answer is a 403.
func (a *githubAuth) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case a.current(r) != "":
			next(w, r)
		case a == nil:
			http.Error(w, "GitHub login is not configured", http.StatusForbidden)
		case r.Header.Get("HX-Request") != "":
			w.Header().Set("HX-Redirect", "/login")
		default:
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	}
}

// handleLogin redirects to GitHub. The state is a random value tied to this
// session, checked against the one GitHub echoes back, so a callback cannot be
// forged from another site.
func (a *githubAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateStateToken()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	session, _ := a.store.Get(r, sessionName)
	session.Values["oauth_state"] = state
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, a.oauth.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

func generateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (a *githubAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, _ := a.store.Get(r, sessionName)
	expectedState, _ := session.Values["oauth_state"].(string)
	// The state is single use, whatever the outcome, so a failure saves the
	// session with it removed.
	delete(session.Values, "oauth_state")
	fail := func(status int, message string) {
		if err := session.Save(r, w); err != nil {
			log.Printf("saving the session: %v", err)
		}
		w.WriteHeader(status)
		a.render(w, "login.html", pageData{Error: message, Version: version})
	}

	q := r.URL.Query()
	if expectedState == "" || q.Get("state") != expectedState {
		fail(http.StatusForbidden, "Invalid or missing state parameter. Please log in again.")
		return
	}
	// The user pressed Cancel on GitHub's authorize screen.
	if q.Get("error") != "" {
		fail(http.StatusForbidden, "GitHub login was cancelled.")
		return
	}

	token, err := a.oauth.Exchange(ctx, q.Get("code"))
	if err != nil {
		log.Printf("exchanging the OAuth code: %v", err)
		fail(http.StatusBadGateway, "Failed to exchange token: "+err.Error())
		return
	}
	client := a.oauth.Client(ctx, token)

	emails, err := a.getVerifiedEmails(client)
	if err != nil {
		fail(http.StatusBadGateway, "Failed to fetch GitHub email: "+err.Error())
		return
	}
	// Any verified address counts, not just the primary: many people keep a
	// personal primary and add their work address as a secondary.
	allowed := slices.ContainsFunc(emails, func(e string) bool {
		return strings.HasSuffix(strings.ToLower(e), allowedEmailDomain)
	})
	if !allowed {
		list := strings.Join(emails, ", ")
		log.Printf("login denied for %s", list)
		fail(http.StatusForbidden, fmt.Sprintf("Access Denied: None of your verified emails (%s) is a %s address.", list, allowedEmailDomain))
		return
	}

	username, err := a.getUsername(client)
	if err != nil {
		fail(http.StatusBadGateway, "Failed to fetch GitHub username: "+err.Error())
		return
	}

	// The token is kept because Logout needs it to revoke the grant.
	session.Values["username"] = username
	session.Values["token"] = token.AccessToken
	if err := session.Save(r, w); err != nil {
		log.Printf("saving the session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}
	log.Printf("%s logged in", username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout revokes the app's grant on GitHub, so the next login asks for
// authorization again, and clears the session. It cannot sign the user out of
// github.com itself. A failed revoke still logs out of this app.
func (a *githubAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := a.store.Get(r, sessionName)
	if token, _ := session.Values["token"].(string); token != "" {
		if err := a.revokeGrant(r.Context(), token); err != nil {
			log.Printf("revoking the GitHub grant: %v", err)
		}
	}
	session.Values = map[any]any{}
	session.Options.MaxAge = -1 // Deletes cookie
	if err := session.Save(r, w); err != nil {
		log.Printf("clearing the session: %v", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// revokeGrant deletes the OAuth app's authorization for the token's user, which
// also invalidates every token the app holds for them.
// https://docs.github.com/en/rest/apps/oauth-applications#delete-an-app-authorization
func (a *githubAuth) revokeGrant(ctx context.Context, token string) error {
	body, err := json.Marshal(map[string]string{"access_token": token})
	if err != nil {
		return err
	}
	endpoint := a.apiURL + "/applications/" + url.PathEscape(a.oauth.ClientID) + "/grant"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(a.oauth.ClientID, a.oauth.ClientSecret)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	return nil
}

// --- Helper Functions to call GitHub API ---

type githubUser struct {
	Login string `json:"login"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (a *githubAuth) getJSON(client *http.Client, path string, v any) error {
	resp, err := client.Get(a.apiURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// getVerifiedEmails returns every verified address on the account. Unverified
// ones are skipped: anyone can add an address they do not own.
func (a *githubAuth) getVerifiedEmails(client *http.Client) ([]string, error) {
	var emails []githubEmail
	if err := a.getJSON(client, "/user/emails", &emails); err != nil {
		return nil, err
	}

	var verified []string
	for _, e := range emails {
		if e.Verified {
			verified = append(verified, e.Email)
		}
	}
	if len(verified) == 0 {
		return nil, fmt.Errorf("no verified email found")
	}
	return verified, nil
}

func (a *githubAuth) getUsername(client *http.Client) (string, error) {
	var u githubUser
	if err := a.getJSON(client, "/user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}
