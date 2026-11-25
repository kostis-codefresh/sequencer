package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// githubStub stands in for github.com and api.github.com: it hands out
// "oauth-token" for any code, answers /user and /user/emails with its fields,
// and records grant revocations.
type githubStub struct {
	*httptest.Server
	login   string
	emails  []githubEmail
	revoked []string // the access tokens whose grant was deleted
	auth    string   // the basic-auth user:password of the last revocation
}

func newGitHubStub(t *testing.T, login string, emails ...githubEmail) *githubStub {
	t.Helper()
	stub := &githubStub{login: login, emails: emails}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"oauth-token","token_type":"bearer"}`))
	})
	requireToken := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer oauth-token" {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if requireToken(w, r) {
			_ = json.NewEncoder(w).Encode(githubUser{Login: stub.login})
		}
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, r *http.Request) {
		if requireToken(w, r) {
			_ = json.NewEncoder(w).Encode(stub.emails)
		}
	})
	mux.HandleFunc("DELETE /applications/{client}/grant", func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		stub.auth = user + ":" + pass
		var body struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		stub.revoked = append(stub.revoked, body.AccessToken)
		w.WriteHeader(http.StatusNoContent)
	})
	stub.Server = httptest.NewServer(mux)
	t.Cleanup(stub.Close)
	return stub
}

// testAuthConfig is an OAuth app that is fully configured, so Login is offered.
var testAuthConfig = authConfig{
	ClientID: "test-client-id", ClientSecret: "test-client-secret",
	BaseURL: "http://localhost:8080", SessionKey: []byte("test-session-key"),
}

// sessionCookie is the session cookie of a user logged in as login with token,
// built through the same store the handler decodes it with.
func sessionCookie(t *testing.T, cfg authConfig, login, token string) *http.Cookie {
	t.Helper()
	store := newSessionStore(cfg)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	session, err := store.Get(req, sessionName)
	require.NoError(t, err, "a new session should not fail")
	session.Values["username"] = login
	session.Values["token"] = token
	require.NoError(t, session.Save(req, rec), "saving the session should succeed")
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "saving the session should set one cookie")
	return cookies[0]
}

// withCookie sends cookie with every request to h.
func withCookie(h http.Handler, cookie *http.Cookie) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		h.ServeHTTP(w, r)
	})
}

// loggedInHandler is the app as seen by test-user, logged in with test-token.
func loggedInHandler(t *testing.T) http.Handler {
	t.Helper()
	return withCookie(newHandler(testAuthConfig), sessionCookie(t, testAuthConfig, "test-user", "test-token"))
}

func (s *githubStub) config() authConfig {
	cfg := testAuthConfig
	cfg.githubURL = s.URL
	cfg.apiURL = s.URL
	return cfg
}

// login runs the whole flow against h: /login/github, then GitHub's redirect
// back to /callback with the state it was given. It returns the callback's
// response.
func login(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	start := get(t, h, "/login/github")
	require.Equal(t, http.StatusTemporaryRedirect, start.Code, "login should redirect to GitHub")
	authorize, err := url.Parse(start.Header().Get("Location"))
	require.NoError(t, err, "the redirect should be a URL")
	cookies := start.Result().Cookies()
	require.Len(t, cookies, 1, "login should set the session cookie holding the state")

	req := httptest.NewRequest(http.MethodGet, "/callback?code=the-code&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginPage(t *testing.T) {
	rec := get(t, newHandler(testAuthConfig), "/login")

	assert.Equal(t, http.StatusOK, rec.Code, "the login page should render")
	for _, want := range []string{
		`src="/img/octopus-deploy-logo.png"`, "Octopus Employees only", `href="/login/github"`, "Login with GitHub",
	} {
		assert.Contains(t, rec.Body.String(), want, "the login page should contain %q", want)
	}
}

func TestOctopusLogo(t *testing.T) {
	rec := get(t, newHandler(testAuthConfig), "/img/octopus-deploy-logo.png")

	assert.Equal(t, http.StatusOK, rec.Code, "the logo should be served")
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"), "content type mismatch")
	assert.True(t, strings.HasPrefix(rec.Body.String(), "\x89PNG"), "body should be the PNG")
}

func TestLoginRedirectsToGitHub(t *testing.T) {
	rec := get(t, newHandler(testAuthConfig), "/login/github")

	require.Equal(t, http.StatusTemporaryRedirect, rec.Code, "login should redirect")
	location, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err, "the redirect should be a URL")
	assert.Equal(t, "https://github.com/login/oauth/authorize", location.Scheme+"://"+location.Host+location.Path, "should redirect to GitHub's authorize page")
	q := location.Query()
	assert.Equal(t, "test-client-id", q.Get("client_id"), "client_id mismatch")
	assert.Equal(t, "http://localhost:8080/callback", q.Get("redirect_uri"), "redirect_uri should be APP_BASE_URL/callback")
	assert.Equal(t, "read:user user:email", q.Get("scope"), "scope mismatch")
	assert.NotEmpty(t, q.Get("state"), "a state should be sent")
}

func TestCallback(t *testing.T) {
	testCases := []struct {
		name         string
		emails       []githubEmail
		expectedCode int
		expectedBody string
		loggedIn     bool
	}{
		{
			name:         "Happy Path scenario",
			emails:       []githubEmail{{Email: "Jane@Octopus.com", Primary: true, Verified: true}},
			expectedCode: http.StatusSeeOther, loggedIn: true,
		},
		{
			name: "Accepts a verified octopus secondary email",
			emails: []githubEmail{
				{Email: "jane@example.com", Primary: true, Verified: true},
				{Email: "jane@octopus.com", Verified: true},
			},
			expectedCode: http.StatusSeeOther, loggedIn: true,
		},
		{
			name:         "Problematic scenario",
			emails:       []githubEmail{{Email: "jane@example.com", Primary: true, Verified: true}},
			expectedCode: http.StatusForbidden, expectedBody: "Access Denied: None of your verified emails (jane@example.com) is a @octopus.com address.",
		},
		{
			name: "An unverified octopus secondary does not count",
			emails: []githubEmail{
				{Email: "jane@example.com", Primary: true, Verified: true},
				{Email: "jane@octopus.com"},
			},
			expectedCode: http.StatusForbidden, expectedBody: "Access Denied: None of your verified emails (jane@example.com) is a @octopus.com address.",
		},
		{
			name:         "An unverified octopus email does not count",
			emails:       []githubEmail{{Email: "jane@octopus.com", Primary: true}},
			expectedCode: http.StatusBadGateway, expectedBody: "no verified email found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newGitHubStub(t, "jane", tc.emails...)
			h := newHandler(gh.config())

			rec := login(t, h)

			assert.Equal(t, tc.expectedCode, rec.Code, "callback status mismatch")
			assert.Contains(t, rec.Body.String(), tc.expectedBody, "callback body mismatch")
			home := get(t, withCookie(h, rec.Result().Cookies()[0]), "/").Body.String()
			if tc.loggedIn {
				assert.Equal(t, "/", rec.Header().Get("Location"), "a login should land on the home page")
				assert.Contains(t, home, `<span class="nav-user me-2">jane</span>`, "the navbar should name the GitHub login")
				assert.Contains(t, home, `action="/logout"`, "a logged-in user should get Logout")
			} else {
				assert.Contains(t, home, `href="/login"`, "a denied user should not be logged in")
			}
		})
	}
}

func TestCallbackRejectsABadState(t *testing.T) {
	testCases := []struct {
		name  string
		state string
	}{
		{name: "Missing state", state: ""},
		{name: "Forged state", state: "not-the-state"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newGitHubStub(t, "jane", githubEmail{Email: "jane@octopus.com", Primary: true, Verified: true})
			h := newHandler(gh.config())
			start := get(t, h, "/login/github")

			req := httptest.NewRequest(http.MethodGet, "/callback?code=the-code&state="+tc.state, nil)
			req.AddCookie(start.Result().Cookies()[0])
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code, "a bad state should be refused")
			assert.Contains(t, rec.Body.String(), "Invalid or missing state parameter", "body mismatch")
		})
	}
}

func TestLogout(t *testing.T) {
	gh := newGitHubStub(t, "jane", githubEmail{Email: "jane@octopus.com", Primary: true, Verified: true})
	h := newHandler(gh.config())
	session := login(t, h).Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code, "logout should redirect")
	assert.Equal(t, "/", rec.Header().Get("Location"), "logout should land on the home page")
	assert.Equal(t, []string{"oauth-token"}, gh.revoked, "logout should revoke the user's grant on GitHub")
	assert.Equal(t, "test-client-id:test-client-secret", gh.auth, "the revocation authenticates as the OAuth app")
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "logout should reset the session cookie")
	assert.Negative(t, cookies[0].MaxAge, "the session cookie should be deleted")
}

func TestLogoutIsNotAGet(t *testing.T) {
	gh := newGitHubStub(t, "jane")
	h := newHandler(gh.config())

	rec := get(t, withCookie(h, sessionCookie(t, gh.config(), "jane", "oauth-token")), "/logout")

	// GET /logout falls through to the GET /{page} catch-all.
	assert.Equal(t, http.StatusNotFound, rec.Code, "a cross-site GET should not log anyone out")
	assert.Empty(t, gh.revoked, "nothing should be revoked")
}

func TestAPIKeysNeedLogin(t *testing.T) {
	h := newHandler(testAuthConfig)

	page := get(t, h, "/api-keys.html")
	assert.Equal(t, http.StatusSeeOther, page.Code, "an anonymous visitor should be sent to log in")
	assert.Equal(t, "/login", page.Header().Get("Location"), "redirect target mismatch")

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api-keys?name=ci", nil)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "%s: htmx needs a 200 to act on HX-Redirect", method)
		assert.Equal(t, "/login", rec.Header().Get("HX-Redirect"), "%s: should send htmx to the login page", method)
		assert.Empty(t, rec.Body.String(), "%s: nothing should be rendered", method)
	}
}

func TestLoginButton(t *testing.T) {
	anonymous := get(t, newHandler(testAuthConfig), "/").Body.String()
	assert.Contains(t, anonymous, `href="/login"`, "a visitor should be offered Login")
	assert.NotContains(t, anonymous, `action="/logout"`, "a visitor should not get Logout")

	loggedIn := get(t, loggedInHandler(t), "/").Body.String()
	assert.Contains(t, loggedIn, `<span class="nav-user me-2">test-user</span>`, "the navbar should name the user")
	assert.NotContains(t, loggedIn, `href="/login"`, "a logged-in user should not be offered Login")
}

func TestWithoutOAuthSettings(t *testing.T) {
	h := newHandler(authConfig{BaseURL: "http://localhost:8080"})

	assert.Contains(t, get(t, h, "/").Body.String(), `href="/login"`, "Login should be shown without OAuth settings")
	assert.Equal(t, http.StatusOK, get(t, h, "/login").Code, "the login page should still render")
	assert.Equal(t, http.StatusNotFound, get(t, h, "/login/github").Code, "the OAuth routes should not be registered")
	assert.Equal(t, http.StatusForbidden, get(t, h, "/api-keys.html").Code, "nobody can log in, so the page is closed")
}

func TestLoginTemplateIsNotAPage(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, get(t, newHandler(testAuthConfig), "/login.html").Code)
}

func TestAuthDisabled(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         authConfig
		githubLogin int
	}{
		{name: "with OAuth settings", cfg: testAuthConfig, githubLogin: http.StatusTemporaryRedirect},
		{name: "without OAuth settings", cfg: authConfig{BaseURL: "http://localhost:8080"}, githubLogin: http.StatusNotFound},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.AuthDisabled = true
			h := newHandler(cfg)

			assert.Equal(t, http.StatusOK, get(t, h, "/api-keys.html").Code, "the API keys page should be open")
			body := createKey(t, h, "ci")
			assert.Contains(t, body, "<code>sk_", "anyone should be able to create a key")

			assert.Contains(t, get(t, h, "/").Body.String(), `href="/login"`, "Login should still be offered")
			login := get(t, h, "/login")
			assert.Equal(t, http.StatusOK, login.Code, "the login page should render")
			assert.Contains(t, login.Body.String(), "Octopus Employees only")
			assert.Equal(t, tc.githubLogin, get(t, h, "/login/github").Code, "/login/github status mismatch")
		})
	}
}

func TestGitHubAuthEnv(t *testing.T) {
	t.Setenv("SESSION_KEY", "test-session-key")

	t.Setenv("GITHUB_AUTH", "")
	assert.False(t, authConfigFromEnv().AuthDisabled, "auth should be on by default")

	t.Setenv("GITHUB_AUTH", "false")
	assert.True(t, authConfigFromEnv().AuthDisabled, "GITHUB_AUTH=false should turn auth off")

	t.Setenv("GITHUB_AUTH", "true")
	assert.False(t, authConfigFromEnv().AuthDisabled, "GITHUB_AUTH=true should keep auth on")
}
