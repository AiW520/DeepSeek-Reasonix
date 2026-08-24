package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/config"
)

func githubTestApp(t *testing.T, handler http.HandlerFunc) (*App, *string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	token := ""
	a := &App{githubClientID: "client-test", githubAPIBase: server.URL, githubWebBase: server.URL, githubHTTPClient: server.Client()}
	a.githubCredentialGet = func(string) (string, error) {
		if token == "" {
			return "", config.ErrSystemCredentialNotFound
		}
		return token, nil
	}
	a.githubCredentialSet = func(_ string, value string) error { token = value; return nil }
	a.githubCredentialDelete = func(string) error { token = ""; return nil }
	return a, &token
}

func TestGitHubDeviceFlowStoresTokenAfterProfileValidation(t *testing.T) {
	a, token := githubTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/device/code":
			_ = r.ParseForm()
			if r.Form.Get("client_id") != "client-test" || !strings.Contains(r.Form.Get("scope"), "repo") {
				t.Errorf("unexpected device form: %v", r.Form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"device_code":"device","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`))
		case "/login/oauth/access_token":
			_, _ = w.Write([]byte(`{"access_token":"oauth-secret","scope":"repo,read:user"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer oauth-secret" {
				t.Errorf("authorization header missing")
			}
			w.Header().Set("X-OAuth-Scopes", "repo, read:user")
			_, _ = w.Write([]byte(`{"login":"octocat","name":"The Octocat","avatar_url":"https://avatars.example/octocat","html_url":"https://github.com/octocat"}`))
		default:
			http.NotFound(w, r)
		}
	})
	flow, err := a.StartGitHubDeviceFlow()
	if err != nil || flow.UserCode != "ABCD-1234" {
		t.Fatalf("flow = %+v, %v", flow, err)
	}
	result, err := a.PollGitHubDeviceFlow(flow.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "connected" || result.Connection.Login != "octocat" {
		t.Fatalf("poll = %+v", result)
	}
	if *token != "oauth-secret" {
		t.Fatalf("stored token = %q", *token)
	}
}

func TestGitHubRepositoriesFiltersAndNeverExposesToken(t *testing.T) {
	a, token := githubTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
		}
		if r.URL.Query().Get("visibility") != "private" {
			t.Errorf("visibility = %q", r.URL.Query().Get("visibility"))
		}
		_, _ = w.Write([]byte(`[{"id":1,"name":"secret","full_name":"acme/secret","description":"video tools","private":true,"default_branch":"main","html_url":"https://github.com/acme/secret","clone_url":"https://github.com/acme/secret.git","owner":{"login":"acme"}}]`))
	})
	*token = "test-token"
	page, err := a.GitHubRepositories("video", "private", "")
	if err != nil || len(page.Repositories) != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	encoded := page.Repositories[0].CloneURL + page.Repositories[0].HTMLURL
	if strings.Contains(encoded, *token) {
		t.Fatal("repository view exposed OAuth token")
	}
}

func TestGitHubCloneRejectsUnsafeCoordinatesAndExistingTarget(t *testing.T) {
	a := &App{githubCredentialGet: func(string) (string, error) { return "", config.ErrSystemCredentialNotFound }}
	parent := t.TempDir()
	for _, tc := range [][2]string{{"../owner", "repo"}, {"owner", "../repo"}, {"owner/name", "repo"}} {
		if _, err := a.CloneGitHubRepository(tc[0], tc[1], parent); err == nil {
			t.Fatalf("accepted unsafe coordinate %q/%q", tc[0], tc[1])
		}
	}
	if err := os.Mkdir(filepath.Join(parent, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CloneGitHubRepository("owner", "repo", parent); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing target error = %v", err)
	}
	if _, err := a.GitHubRepositories("", "all", url.QueryEscape("../2")); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}

func TestGitHubPollDoesNotStoreTokenWhenProfileFails(t *testing.T) {
	a, token := githubTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/oauth/access_token" {
			_, _ = w.Write([]byte(`{"access_token":"must-not-store"}`))
			return
		}
		http.Error(w, `{"message":"bad credentials"}`, http.StatusUnauthorized)
	})
	if _, err := a.PollGitHubDeviceFlow("device"); err == nil {
		t.Fatal("profile failure accepted")
	}
	if *token != "" {
		t.Fatal("token stored before profile validation")
	}
}

func TestGitHubConnectionSurfacesKeyringFailureWithoutPretendingConnected(t *testing.T) {
	a := &App{githubClientID: "client", githubCredentialGet: func(string) (string, error) { return "", errors.New("vault unavailable") }}
	view, err := a.GitHubConnection()
	if err != nil || view.Connected || !strings.Contains(view.Error, "vault unavailable") {
		t.Fatalf("view = %+v, err = %v", view, err)
	}
}
