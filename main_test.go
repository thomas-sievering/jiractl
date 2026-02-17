package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchTransitionExactWins(t *testing.T) {
	transitions := []JiraTransition{
		{ID: "1", Name: "Done (QA)"},
		{ID: "2", Name: "Done"},
	}

	matched, matchedBy, warning, err := matchTransition(transitions, "Done")
	if err != nil {
		t.Fatalf("matchTransition returned error: %v", err)
	}
	if matched.Name != "Done" {
		t.Fatalf("expected exact match 'Done', got %q", matched.Name)
	}
	if matchedBy != "exact" {
		t.Fatalf("expected matchedBy=exact, got %q", matchedBy)
	}
	if warning != "" {
		t.Fatalf("expected no warning for exact match, got %q", warning)
	}
}

func TestMatchTransitionPrefixAmbiguityWarns(t *testing.T) {
	transitions := []JiraTransition{
		{ID: "10", Name: "Done"},
		{ID: "11", Name: "Done (QA)"},
	}

	matched, matchedBy, warning, err := matchTransition(transitions, "Do")
	if err != nil {
		t.Fatalf("matchTransition returned error: %v", err)
	}
	if matched.Name != "Done" {
		t.Fatalf("expected shortest deterministic match 'Done', got %q", matched.Name)
	}
	if matchedBy != "prefix" {
		t.Fatalf("expected matchedBy=prefix, got %q", matchedBy)
	}
	if warning == "" {
		t.Fatal("expected ambiguity warning for prefix match")
	}
}

func TestMatchTransitionNoMatchIncludesAvailable(t *testing.T) {
	transitions := []JiraTransition{
		{ID: "1", Name: "In Progress"},
		{ID: "2", Name: "Done"},
	}

	_, _, _, err := matchTransition(transitions, "Blocked")
	if err == nil {
		t.Fatal("expected error for missing transition")
	}
	msg := err.Error()
	if msg == "" || !(containsAll(msg, []string{"Blocked", "In Progress", "Done"})) {
		t.Fatalf("expected error to include query and available transitions, got %q", msg)
	}
}

func TestSearchIssuesReturnsPaginationMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		nextToken := r.URL.Query().Get("nextPageToken")
		if nextToken == "" {
			writeJSON(t, w, JiraSearchResponse{
				Total: 5,
				Issues: []JiraIssue{
					{Key: "PROJ-1"},
					{Key: "PROJ-2"},
				},
				NextPageToken: "page-2",
			})
			return
		}
		writeJSON(t, w, JiraSearchResponse{Total: 5, Issues: []JiraIssue{}})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := Config{Server: ts.URL, Email: "user@example.com", APIToken: "token"}
	result, err := searchIssues(cfg, "project = PROJ", 2)
	if err != nil {
		t.Fatalf("searchIssues returned error: %v", err)
	}

	if got := len(result.Issues); got != 2 {
		t.Fatalf("expected 2 issues, got %d", got)
	}
	if result.Total != 5 {
		t.Fatalf("expected total=5, got %d", result.Total)
	}
	if !result.HasMore {
		t.Fatal("expected has_more=true when server has additional pages")
	}
}

func TestGetCommentsRespectsLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/comment", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("maxResults"); got != "37" {
			t.Fatalf("expected maxResults=37, got %q", got)
		}
		if got := r.URL.Query().Get("orderBy"); got != "-created" {
			t.Fatalf("expected orderBy=-created, got %q", got)
		}
		writeJSON(t, w, JiraCommentsResponse{Comments: []JiraComment{}, Total: 0})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := Config{Server: ts.URL, Email: "user@example.com", APIToken: "token"}
	if _, err := getComments(cfg, "PROJ-1", 37); err != nil {
		t.Fatalf("getComments returned error: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("failed to write json response: %v", err)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
