package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/config"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const githubTokenAccount = "github-oauth-token-v1"

// githubOAuthClientID may be injected in release builds with -ldflags. Local
// development can use REASONIX_GITHUB_CLIENT_ID instead.
var githubOAuthClientID string

var githubCoordinatePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9])?$`)

type GitHubConnectionView struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Login      string `json:"login,omitempty"`
	Name       string `json:"name,omitempty"`
	AvatarURL  string `json:"avatarUrl,omitempty"`
	ProfileURL string `json:"profileUrl,omitempty"`
	Scopes     string `json:"scopes,omitempty"`
	Error      string `json:"error,omitempty"`
}

type GitHubDeviceFlowStart struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

type GitHubDeviceFlowPollResult struct {
	Status     string               `json:"status"`
	Interval   int                  `json:"interval,omitempty"`
	Connection GitHubConnectionView `json:"connection"`
}

type GitHubRepositoryView struct {
	ID            int64  `json:"id"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	Description   string `json:"description,omitempty"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Language      string `json:"language,omitempty"`
	DefaultBranch string `json:"defaultBranch"`
	HTMLURL       string `json:"htmlUrl"`
	CloneURL      string `json:"cloneUrl"`
	UpdatedAt     string `json:"updatedAt"`
	Stars         int    `json:"stars"`
}

type GitHubRepositoryPage struct {
	Repositories []GitHubRepositoryView `json:"repositories"`
	NextCursor   string                 `json:"nextCursor,omitempty"`
}

type GitHubCloneResult struct {
	Path string `json:"path"`
}

type githubUser struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type githubAPIRepository struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	Language    string `json:"language"`
	Default     string `json:"default_branch"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
	UpdatedAt   string `json:"updated_at"`
	Stars       int    `json:"stargazers_count"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (a *App) githubClientIDValue() string {
	if strings.TrimSpace(a.githubClientID) != "" {
		return strings.TrimSpace(a.githubClientID)
	}
	if strings.TrimSpace(githubOAuthClientID) != "" {
		return strings.TrimSpace(githubOAuthClientID)
	}
	return strings.TrimSpace(os.Getenv("REASONIX_GITHUB_CLIENT_ID"))
}

func (a *App) githubHTTP() *http.Client {
	if a.githubHTTPClient != nil {
		return a.githubHTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (a *App) githubGetCredential() (string, error) {
	if a.githubCredentialGet != nil {
		return a.githubCredentialGet(githubTokenAccount)
	}
	return config.GetSystemCredential(githubTokenAccount)
}

func (a *App) githubSetCredential(value string) error {
	if a.githubCredentialSet != nil {
		return a.githubCredentialSet(githubTokenAccount, value)
	}
	return config.SetSystemCredential(githubTokenAccount, value)
}

func (a *App) githubDeleteCredential() error {
	if a.githubCredentialDelete != nil {
		return a.githubCredentialDelete(githubTokenAccount)
	}
	return config.DeleteSystemCredential(githubTokenAccount)
}

func (a *App) githubWebURL(path string) string {
	base := strings.TrimRight(a.githubWebBase, "/")
	if base == "" {
		base = "https://github.com"
	}
	return base + path
}

func (a *App) githubAPIURL(path string) string {
	base := strings.TrimRight(a.githubAPIBase, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return base + path
}

func (a *App) GitHubConnection() (GitHubConnectionView, error) {
	view := GitHubConnectionView{Configured: a.githubClientIDValue() != ""}
	token, err := a.githubGetCredential()
	if errors.Is(err, config.ErrSystemCredentialNotFound) {
		return view, nil
	}
	if err != nil {
		view.Error = err.Error()
		return view, nil
	}
	user, scopes, err := a.githubCurrentUser(a.bootContext(), token)
	if err != nil {
		return view, err
	}
	view.Connected = true
	view.Login = user.Login
	view.Name = user.Name
	view.AvatarURL = safeGitHubAvatarURL(user.AvatarURL)
	view.ProfileURL = "https://github.com/" + user.Login
	view.Scopes = scopes
	return view, nil
}

func (a *App) StartGitHubDeviceFlow() (GitHubDeviceFlowStart, error) {
	clientID := a.githubClientIDValue()
	if clientID == "" {
		return GitHubDeviceFlowStart{}, fmt.Errorf("this build has no GitHub OAuth client ID; set REASONIX_GITHUB_CLIENT_ID")
	}
	form := url.Values{"client_id": {clientID}, "scope": {"read:user repo"}}
	req, err := http.NewRequestWithContext(a.bootContext(), http.MethodPost, a.githubWebURL("/login/device/code"), strings.NewReader(form.Encode()))
	if err != nil {
		return GitHubDeviceFlowStart{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := a.githubJSON(req, &response); err != nil {
		return GitHubDeviceFlowStart{}, err
	}
	if response.DeviceCode == "" || response.UserCode == "" || response.VerificationURI == "" {
		return GitHubDeviceFlowStart{}, fmt.Errorf("GitHub returned an incomplete device authorization response")
	}
	if response.Interval < 5 {
		response.Interval = 5
	}
	return GitHubDeviceFlowStart{DeviceCode: response.DeviceCode, UserCode: response.UserCode, VerificationURI: response.VerificationURI, ExpiresIn: response.ExpiresIn, Interval: response.Interval}, nil
}

func (a *App) PollGitHubDeviceFlow(deviceCode string) (GitHubDeviceFlowPollResult, error) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return GitHubDeviceFlowPollResult{}, fmt.Errorf("device code is required")
	}
	form := url.Values{
		"client_id":   {a.githubClientIDValue()},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	req, err := http.NewRequestWithContext(a.bootContext(), http.MethodPost, a.githubWebURL("/login/oauth/access_token"), strings.NewReader(form.Encode()))
	if err != nil {
		return GitHubDeviceFlowPollResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := a.githubJSON(req, &response); err != nil {
		return GitHubDeviceFlowPollResult{}, err
	}
	if response.Error != "" {
		switch response.Error {
		case "authorization_pending":
			return GitHubDeviceFlowPollResult{Status: "pending"}, nil
		case "slow_down":
			return GitHubDeviceFlowPollResult{Status: "pending", Interval: 10}, nil
		case "access_denied", "expired_token":
			return GitHubDeviceFlowPollResult{Status: response.Error}, nil
		default:
			return GitHubDeviceFlowPollResult{}, fmt.Errorf("GitHub authorization failed: %s", response.Error)
		}
	}
	if response.AccessToken == "" {
		return GitHubDeviceFlowPollResult{}, fmt.Errorf("GitHub authorization returned no access token")
	}
	user, scopes, err := a.githubCurrentUser(a.bootContext(), response.AccessToken)
	if err != nil {
		return GitHubDeviceFlowPollResult{}, err
	}
	if err := a.githubSetCredential(response.AccessToken); err != nil {
		return GitHubDeviceFlowPollResult{}, err
	}
	return GitHubDeviceFlowPollResult{Status: "connected", Connection: GitHubConnectionView{
		Configured: true, Connected: true, Login: user.Login, Name: user.Name, AvatarURL: safeGitHubAvatarURL(user.AvatarURL), ProfileURL: "https://github.com/" + user.Login, Scopes: scopes,
	}}, nil
}

func (a *App) DisconnectGitHub() error {
	return a.githubDeleteCredential()
}

func (a *App) GitHubRepositories(query, visibility, cursor string) (GitHubRepositoryPage, error) {
	token, err := a.githubGetCredential()
	if err != nil {
		if errors.Is(err, config.ErrSystemCredentialNotFound) {
			return GitHubRepositoryPage{}, fmt.Errorf("connect GitHub before loading repositories")
		}
		return GitHubRepositoryPage{}, err
	}
	page := 1
	if cursor != "" {
		page, err = strconv.Atoi(cursor)
		if err != nil || page < 1 || page > 1000 {
			return GitHubRepositoryPage{}, fmt.Errorf("invalid repository cursor")
		}
	}
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	if visibility != "public" && visibility != "private" {
		visibility = "all"
	}
	params := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "sort": {"updated"}, "affiliation": {"owner,collaborator,organization_member"}, "visibility": {visibility}}
	req, err := http.NewRequestWithContext(a.bootContext(), http.MethodGet, a.githubAPIURL("/user/repos?")+params.Encode(), nil)
	if err != nil {
		return GitHubRepositoryPage{}, err
	}
	a.githubAPIHeaders(req, token)
	var rows []githubAPIRepository
	if err := a.githubJSON(req, &rows); err != nil {
		return GitHubRepositoryPage{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	result := GitHubRepositoryPage{Repositories: make([]GitHubRepositoryView, 0, len(rows))}
	for _, row := range rows {
		if !githubCoordinatePattern.MatchString(row.Owner.Login) || !githubCoordinatePattern.MatchString(row.Name) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(row.FullName+" "+row.Description), needle) {
			continue
		}
		result.Repositories = append(result.Repositories, GitHubRepositoryView{
			ID: row.ID, Owner: row.Owner.Login, Name: row.Name, FullName: row.Owner.Login + "/" + row.Name, Description: row.Description,
			Private: row.Private, Fork: row.Fork, Language: row.Language, DefaultBranch: row.Default,
			CloneURL: "https://github.com/" + row.Owner.Login + "/" + row.Name + ".git", HTMLURL: "https://github.com/" + row.Owner.Login + "/" + row.Name, UpdatedAt: row.UpdatedAt, Stars: row.Stars,
		})
	}
	if len(rows) == 100 {
		result.NextCursor = strconv.Itoa(page + 1)
	}
	return result, nil
}

func safeGitHubAvatarURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "avatars.githubusercontent.com") {
		return ""
	}
	return u.String()
}

func (a *App) PickGitHubCloneParent() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose where to clone the repository", DefaultDirectory: dialogDefaultDirectory("")})
}

func (a *App) CloneGitHubRepository(owner, repo, parentDir string) (GitHubCloneResult, error) {
	owner, repo = strings.TrimSpace(owner), strings.TrimSpace(repo)
	if !githubCoordinatePattern.MatchString(owner) || !githubCoordinatePattern.MatchString(repo) || owner == "." || owner == ".." || repo == "." || repo == ".." {
		return GitHubCloneResult{}, fmt.Errorf("invalid GitHub repository coordinate")
	}
	parentAbs, err := filepath.Abs(strings.TrimSpace(parentDir))
	if err != nil || strings.TrimSpace(parentDir) == "" {
		return GitHubCloneResult{}, fmt.Errorf("a valid clone destination is required")
	}
	info, err := os.Stat(parentAbs)
	if err != nil || !info.IsDir() {
		return GitHubCloneResult{}, fmt.Errorf("clone destination must be an existing directory")
	}
	target := filepath.Join(parentAbs, repo)
	rel, err := filepath.Rel(parentAbs, target)
	if err != nil || rel != repo {
		return GitHubCloneResult{}, fmt.Errorf("clone target escapes the selected destination")
	}
	if _, err := os.Stat(target); err == nil {
		return GitHubCloneResult{}, fmt.Errorf("target folder already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return GitHubCloneResult{}, fmt.Errorf("inspect clone target: %w", err)
	}
	remoteURL := "https://github.com/" + owner + "/" + repo + ".git"
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return GitHubCloneResult{}, fmt.Errorf("Git is not installed or not available on PATH")
	}
	ctx, cancel := context.WithTimeout(a.bootContext(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitPath, "clone", "--origin", "origin", "--", remoteURL, target)
	cmd.Env = os.Environ()
	redactToken := ""
	if token, tokenErr := a.githubGetCredential(); tokenErr == nil && token != "" {
		redactToken = token
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		cmd.Env = append(cmd.Env,
			"GIT_TERMINAL_PROMPT=0",
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=http.https://github.com/.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+auth,
			"GIT_CONFIG_KEY_1=credential.helper",
			"GIT_CONFIG_VALUE_1=",
		)
	}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if redactToken != "" {
			message = strings.ReplaceAll(message, redactToken, "[credential redacted]")
			message = strings.ReplaceAll(message, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+redactToken)), "[credential redacted]")
		}
		if len(message) > 1200 {
			message = message[len(message)-1200:]
		}
		return GitHubCloneResult{}, fmt.Errorf("git clone failed: %s", message)
	}
	return GitHubCloneResult{Path: target}, nil
}

func (a *App) githubCurrentUser(ctx context.Context, token string) (githubUser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.githubAPIURL("/user"), nil)
	if err != nil {
		return githubUser{}, "", err
	}
	a.githubAPIHeaders(req, token)
	var user githubUser
	if err := a.githubJSON(req, &user); err != nil {
		return githubUser{}, "", err
	}
	if !githubCoordinatePattern.MatchString(user.Login) || user.Login == "." || user.Login == ".." {
		return githubUser{}, "", fmt.Errorf("GitHub returned an invalid account profile")
	}
	return user, req.Response.Header.Get("X-OAuth-Scopes"), nil
}

func (a *App) githubAPIHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Reasonix-Desktop")
}

func (a *App) githubJSON(req *http.Request, dst any) error {
	resp, err := a.githubHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("GitHub request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read GitHub response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &apiErr)
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("GitHub request failed (%d): %s", resp.StatusCode, apiErr.Message)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	// Make response headers available to callers that need OAuth scope metadata.
	req.Response = resp
	return nil
}
