package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Config types
// ---------------------------------------------------------------------------

type Config struct {
	Server   string `json:"server"`
	Email    string `json:"email"`
	APIToken string `json:"api_token"`
}

// ---------------------------------------------------------------------------
// Jira API response types
// ---------------------------------------------------------------------------

type JiraUser struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
	Active       bool   `json:"active"`
}

type JiraSearchResponse struct {
	Total         int          `json:"total"`
	Issues        []JiraIssue  `json:"issues"`
	NextPageToken string       `json:"nextPageToken"`
}

type JiraIssue struct {
	Key    string          `json:"key"`
	Self   string          `json:"self"`
	Fields JiraIssueFields `json:"fields"`
}

type JiraIssueFields struct {
	Summary     string           `json:"summary"`
	Description any              `json:"description"`
	Status      *JiraNameField   `json:"status"`
	IssueType   *JiraNameField   `json:"issuetype"`
	Priority    *JiraNameField   `json:"priority"`
	Assignee    *JiraUser        `json:"assignee"`
	Reporter    *JiraUser        `json:"reporter"`
	Created     string           `json:"created"`
	Updated     string           `json:"updated"`
	Labels      []string         `json:"labels"`
	Components  []JiraNameField  `json:"components"`
}

type JiraNameField struct {
	Name string `json:"name"`
}

type JiraCommentsResponse struct {
	Comments []JiraComment `json:"comments"`
	Total    int           `json:"total"`
}

type JiraComment struct {
	Author  *JiraUser `json:"author"`
	Body    any       `json:"body"`
	Created string    `json:"created"`
	Updated string    `json:"updated"`
}

type JiraAPIError struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

// ---------------------------------------------------------------------------
// Compact output types (agent-friendly)
// ---------------------------------------------------------------------------

type IssueView struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
	Assignee string `json:"assignee"`
	Created  string `json:"created"`
	Updated  string `json:"updated"`
	URL      string `json:"url"`
}

type IssueDetailView struct {
	IssueView
	Description string       `json:"description"`
	Comments    []CommentView `json:"comments,omitempty"`
}

type CommentView struct {
	Author  string `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printRootHelp()
		return nil
	}

	switch os.Args[1] {
	case "auth":
		return runAuth(os.Args[2:])
	case "issues":
		return runIssues(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("jiractl dev")
		return nil
	case "help", "--help", "-h":
		printRootHelp()
		return nil
	default:
		printRootHelp()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

// ---------------------------------------------------------------------------
// Help functions
// ---------------------------------------------------------------------------

func printRootHelp() {
	fmt.Println("jiractl: Jira Cloud CLI for agents")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  auth login    Authenticate with Jira Cloud")
	fmt.Println("  auth status   Show current auth status")
	fmt.Println("  auth logout   Remove stored credentials")
	fmt.Println("  issues mine   List issues assigned to you")
	fmt.Println("  issues view   View a single issue by key")
	fmt.Println("  issues search Search issues with JQL")
	fmt.Println("  version       Print version")
	fmt.Println("  help          Show this help")
	fmt.Println()
	fmt.Println("Use --json on data commands for agent-friendly output.")
}

func printAuthHelp() {
	fmt.Println("jiractl auth commands:")
	fmt.Println("  auth login   --server URL --email EMAIL [--token TOKEN]")
	fmt.Println("  auth status  [--json]")
	fmt.Println("  auth logout")
}

func printIssuesHelp() {
	fmt.Println("jiractl issues commands:")
	fmt.Println("  issues mine    [--limit N] [--status STATUS] [--json]")
	fmt.Println("  issues view    ISSUE-KEY [--json]")
	fmt.Println("  issues search  --jql \"...\" [--limit N] [--json]")
}

// ---------------------------------------------------------------------------
// Auth commands
// ---------------------------------------------------------------------------

func runAuth(args []string) error {
	if len(args) == 0 {
		printAuthHelp()
		return nil
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:])
	case "status":
		return runAuthStatus(args[1:])
	case "logout":
		return runAuthLogout(args[1:])
	case "help", "--help", "-h":
		printAuthHelp()
		return nil
	default:
		printAuthHelp()
		return fmt.Errorf("unknown auth command %q", args[0])
	}
}

func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	server := fs.String("server", "", "Jira Cloud server URL (e.g. https://company.atlassian.net)")
	email := fs.String("email", "", "Jira account email")
	token := fs.String("token", "", "Jira API token (prompts if not provided)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve server: flag > env > prompt
	srv := firstNonEmpty(*server, os.Getenv("JIRACTL_SERVER"))
	if srv == "" {
		return errors.New("--server is required (or set JIRACTL_SERVER)")
	}
	srv = strings.TrimRight(srv, "/")

	// Resolve email: flag > env > prompt
	em := firstNonEmpty(*email, os.Getenv("JIRACTL_EMAIL"))
	if em == "" {
		return errors.New("--email is required (or set JIRACTL_EMAIL)")
	}

	// Resolve token: flag > env > prompt
	tok := firstNonEmpty(*token, os.Getenv("JIRACTL_API_TOKEN"))
	if tok == "" {
		if !isInteractive() {
			return errors.New("--token is required in non-interactive mode (or set JIRACTL_API_TOKEN)")
		}
		fmt.Print("API token: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			tok = strings.TrimSpace(scanner.Text())
		}
		if tok == "" {
			return errors.New("API token cannot be empty")
		}
	}

	// Verify credentials by calling /rest/api/3/myself
	client := buildHTTPClient(srv, em, tok)
	req, err := http.NewRequest(http.MethodGet, srv+"/rest/api/3/myself", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", srv, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth verification failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var user JiraUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	// Save config
	cfg := Config{
		Server:   srv,
		Email:    em,
		APIToken: tok,
	}
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Authenticated as %s (%s) on %s\n", user.DisplayName, em, srv)
	return nil
}

func runAuthStatus(args []string) error {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAuthConfig()
	if err != nil {
		return err
	}

	if *jsonOut {
		out := map[string]any{
			"authenticated": true,
			"server":        cfg.Server,
			"email":         cfg.Email,
		}
		return printJSON(out)
	}

	fmt.Printf("Authenticated: yes\n")
	fmt.Printf("Server:        %s\n", cfg.Server)
	fmt.Printf("Email:         %s\n", cfg.Email)
	return nil
}

func runAuthLogout(args []string) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Already logged out.")
			return nil
		}
		return err
	}
	fmt.Println("Logged out. Config removed.")
	return nil
}

// ---------------------------------------------------------------------------
// Issues commands
// ---------------------------------------------------------------------------

func runIssues(args []string) error {
	if len(args) == 0 {
		printIssuesHelp()
		return nil
	}

	switch args[0] {
	case "mine":
		return runIssuesMine(args[1:])
	case "view":
		return runIssuesView(args[1:])
	case "search":
		return runIssuesSearch(args[1:])
	case "help", "--help", "-h":
		printIssuesHelp()
		return nil
	default:
		printIssuesHelp()
		return fmt.Errorf("unknown issues command %q", args[0])
	}
}

func runIssuesMine(args []string) error {
	fs := flag.NewFlagSet("issues mine", flag.ContinueOnError)
	limit := fs.Int("limit", 50, "max issues to return")
	status := fs.String("status", "", "filter by status (e.g. \"In Progress\")")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *limit <= 0 {
		return errors.New("--limit must be greater than 0")
	}

	cfg, err := loadAuthConfig()
	if err != nil {
		return err
	}

	jql := "assignee = currentUser() ORDER BY updated DESC"
	if *status != "" {
		jql = fmt.Sprintf("assignee = currentUser() AND status = %q ORDER BY updated DESC", *status)
	}

	issues, err := searchIssues(cfg, jql, *limit)
	if err != nil {
		return err
	}

	views := issuesToViews(issues, cfg.Server)

	if *jsonOut {
		out := map[string]any{
			"server": cfg.Server,
			"count":  len(views),
			"issues": views,
		}
		return printJSON(out)
	}

	if len(views) == 0 {
		fmt.Println("No issues assigned to you.")
		return nil
	}

	fmt.Printf("Assigned issues (%d):\n", len(views))
	for _, v := range views {
		fmt.Printf("- %-12s  [%s]  %s\n", v.Key, v.Status, v.Summary)
	}
	return nil
}

func runIssuesView(args []string) error {
	fs := flag.NewFlagSet("issues view", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return errors.New("issue key is required (e.g. jiractl issues view PROJ-123)")
	}
	issueKey := strings.ToUpper(remaining[0])

	cfg, err := loadAuthConfig()
	if err != nil {
		return err
	}

	issue, err := getIssue(cfg, issueKey)
	if err != nil {
		return err
	}

	comments, err := getComments(cfg, issueKey)
	if err != nil {
		return err
	}

	view := issueToDetailView(issue, cfg.Server, comments)

	if *jsonOut {
		return printJSON(view)
	}

	fmt.Printf("Key:         %s\n", view.Key)
	fmt.Printf("Summary:     %s\n", view.Summary)
	fmt.Printf("Status:      %s\n", view.Status)
	fmt.Printf("Type:        %s\n", view.Type)
	fmt.Printf("Priority:    %s\n", view.Priority)
	fmt.Printf("Assignee:    %s\n", view.Assignee)
	fmt.Printf("Created:     %s\n", view.Created)
	fmt.Printf("Updated:     %s\n", view.Updated)
	fmt.Printf("URL:         %s\n", view.URL)
	if view.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", view.Description)
	}
	if len(view.Comments) > 0 {
		fmt.Printf("\nComments (%d):\n", len(view.Comments))
		for _, c := range view.Comments {
			fmt.Printf("\n  %s (%s):\n  %s\n", c.Author, c.Created, c.Body)
		}
	}
	return nil
}

func runIssuesSearch(args []string) error {
	fs := flag.NewFlagSet("issues search", flag.ContinueOnError)
	jql := fs.String("jql", "", "JQL query string")
	limit := fs.Int("limit", 50, "max issues to return")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *jql == "" {
		return errors.New("--jql is required (e.g. --jql \"project = PROJ\")")
	}
	if *limit <= 0 {
		return errors.New("--limit must be greater than 0")
	}

	cfg, err := loadAuthConfig()
	if err != nil {
		return err
	}

	issues, err := searchIssues(cfg, *jql, *limit)
	if err != nil {
		return err
	}

	views := issuesToViews(issues, cfg.Server)

	if *jsonOut {
		out := map[string]any{
			"server": cfg.Server,
			"count":  len(views),
			"issues": views,
		}
		return printJSON(out)
	}

	if len(views) == 0 {
		fmt.Println("No issues found.")
		return nil
	}

	fmt.Printf("Issues (%d):\n", len(views))
	for _, v := range views {
		fmt.Printf("- %-12s  [%s]  %s\n", v.Key, v.Status, v.Summary)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Jira API calls
// ---------------------------------------------------------------------------

func searchIssues(cfg Config, jql string, limit int) ([]JiraIssue, error) {
	var all []JiraIssue
	nextPageToken := ""

	client := buildHTTPClient(cfg.Server, cfg.Email, cfg.APIToken)

	for len(all) < limit {
		maxResults := minInt(limit-len(all), 100)

		u, err := url.Parse(cfg.Server + "/rest/api/3/search/jql")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("jql", jql)
		q.Set("maxResults", fmt.Sprintf("%d", maxResults))
		q.Set("fields", "summary,status,issuetype,priority,assignee,reporter,created,updated,labels,components")
		if nextPageToken != "" {
			q.Set("nextPageToken", nextPageToken)
		}
		u.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("jira api request failed: %w", err)
		}

		var searchResp JiraSearchResponse
		if err := decodeAPIResponse(resp, &searchResp); err != nil {
			return nil, err
		}

		all = append(all, searchResp.Issues...)

		if len(searchResp.Issues) == 0 || searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}

	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func getIssue(cfg Config, issueKey string) (JiraIssue, error) {
	u := cfg.Server + "/rest/api/3/issue/" + url.PathEscape(issueKey) +
		"?fields=summary,description,status,issuetype,priority,assignee,reporter,created,updated,labels,components"

	client := buildHTTPClient(cfg.Server, cfg.Email, cfg.APIToken)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return JiraIssue{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return JiraIssue{}, fmt.Errorf("jira api request failed: %w", err)
	}

	var issue JiraIssue
	if err := decodeAPIResponse(resp, &issue); err != nil {
		return JiraIssue{}, err
	}
	return issue, nil
}

func getComments(cfg Config, issueKey string) ([]JiraComment, error) {
	u := cfg.Server + "/rest/api/3/issue/" + url.PathEscape(issueKey) + "/comment?orderBy=-created&maxResults=20"

	client := buildHTTPClient(cfg.Server, cfg.Email, cfg.APIToken)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira api request failed: %w", err)
	}

	var commentsResp JiraCommentsResponse
	if err := decodeAPIResponse(resp, &commentsResp); err != nil {
		return nil, err
	}
	return commentsResp.Comments, nil
}

// ---------------------------------------------------------------------------
// HTTP / API helpers
// ---------------------------------------------------------------------------

func buildHTTPClient(server, email, token string) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &basicAuthTransport{
			email: email,
			token: token,
			base:  http.DefaultTransport,
		},
	}
}

type basicAuthTransport struct {
	email string
	token string
	base  http.RoundTripper
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	creds := base64.StdEncoding.EncodeToString([]byte(t.email + ":" + t.token))
	r.Header.Set("Authorization", "Basic "+creds)
	return t.base.RoundTrip(r)
}

func decodeAPIResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		trimmed := strings.TrimSpace(string(body))

		var apiErr JiraAPIError
		if err := json.Unmarshal(body, &apiErr); err == nil {
			msgs := apiErr.ErrorMessages
			for k, v := range apiErr.Errors {
				msgs = append(msgs, fmt.Sprintf("%s: %s", k, v))
			}
			if len(msgs) > 0 {
				return fmt.Errorf("jira api error (%s): %s", resp.Status, strings.Join(msgs, "; "))
			}
		}

		if trimmed == "" {
			trimmed = resp.Status
		}
		return fmt.Errorf("jira api error (%s): %s", resp.Status, trimmed)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// ---------------------------------------------------------------------------
// Config loading / saving
// ---------------------------------------------------------------------------

func configDir() (string, error) {
	var root string
	if runtime.GOOS == "windows" {
		root = os.Getenv("APPDATA")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, "AppData", "Roaming")
		}
	} else {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg != "" {
			root = xdg
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, ".config")
		}
	}
	d := filepath.Join(root, "jiractl")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func configPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

func saveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func loadConfig() (Config, error) {
	var cfg Config
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// loadAuthConfig resolves auth from flags > env > config file and validates.
func loadAuthConfig() (Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return Config{}, err
	}

	// Override with env vars if present
	if v := os.Getenv("JIRACTL_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("JIRACTL_EMAIL"); v != "" {
		cfg.Email = v
	}
	if v := os.Getenv("JIRACTL_API_TOKEN"); v != "" {
		cfg.APIToken = v
	}

	if cfg.Server == "" || cfg.Email == "" || cfg.APIToken == "" {
		return Config{}, errors.New("not authenticated; run: jiractl auth login --server URL --email EMAIL")
	}

	cfg.Server = strings.TrimRight(cfg.Server, "/")
	return cfg, nil
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

func issueToView(issue JiraIssue, server string) IssueView {
	return IssueView{
		Key:      issue.Key,
		Summary:  issue.Fields.Summary,
		Status:   nameOrEmpty(issue.Fields.Status),
		Type:     nameOrEmpty(issue.Fields.IssueType),
		Priority: nameOrEmpty(issue.Fields.Priority),
		Assignee: userEmail(issue.Fields.Assignee),
		Created:  formatDate(issue.Fields.Created),
		Updated:  formatDate(issue.Fields.Updated),
		URL:      server + "/browse/" + issue.Key,
	}
}

func issuesToViews(issues []JiraIssue, server string) []IssueView {
	views := make([]IssueView, len(issues))
	for i, issue := range issues {
		views[i] = issueToView(issue, server)
	}
	return views
}

func issueToDetailView(issue JiraIssue, server string, comments []JiraComment) IssueDetailView {
	dv := IssueDetailView{
		IssueView:   issueToView(issue, server),
		Description: adfToText(issue.Fields.Description),
	}
	for _, c := range comments {
		dv.Comments = append(dv.Comments, CommentView{
			Author:  userDisplayName(c.Author),
			Body:    adfToText(c.Body),
			Created: formatDate(c.Created),
		})
	}
	return dv
}

func userDisplayName(u *JiraUser) string {
	if u == nil {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.EmailAddress
}

// adfToText extracts plain text from Jira's Atlassian Document Format.
func adfToText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	doc, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	adfExtract(doc, &sb)
	return strings.TrimSpace(sb.String())
}

func adfExtract(node map[string]any, sb *strings.Builder) {
	// Text node
	if text, ok := node["text"].(string); ok {
		sb.WriteString(text)
	}

	// Recurse into content array
	content, ok := node["content"].([]any)
	if !ok {
		return
	}

	nodeType, _ := node["type"].(string)
	for i, child := range content {
		childMap, ok := child.(map[string]any)
		if !ok {
			continue
		}
		adfExtract(childMap, sb)

		// Add newline between block-level siblings
		childType, _ := childMap["type"].(string)
		if childType == "paragraph" || childType == "heading" || childType == "bulletList" ||
			childType == "orderedList" || childType == "blockquote" || childType == "codeBlock" ||
			childType == "mediaSingle" || childType == "rule" {
			if i < len(content)-1 {
				sb.WriteString("\n")
			}
		}
	}

	// List items get newlines
	if nodeType == "listItem" {
		sb.WriteString("\n")
	}
}

func nameOrEmpty(f *JiraNameField) string {
	if f == nil {
		return ""
	}
	return f.Name
}

func userEmail(u *JiraUser) string {
	if u == nil {
		return ""
	}
	if u.EmailAddress != "" {
		return u.EmailAddress
	}
	return u.DisplayName
}

func formatDate(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", s)
	if err != nil {
		// Try alternate format
		t, err = time.Parse("2006-01-02T15:04:05.999-0700", s)
		if err != nil {
			// Return date portion if parsing fails
			if len(s) >= 10 {
				return s[:10]
			}
			return s
		}
	}
	return t.Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// JSON output
// ---------------------------------------------------------------------------

func printJSON(v any) error {
	pretty := strings.TrimSpace(os.Getenv("JIRACTL_JSON_PRETTY")) == "1"
	var (
		b   []byte
		err error
	)
	if pretty {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// ---------------------------------------------------------------------------
// General helpers
// ---------------------------------------------------------------------------

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isInteractive() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
