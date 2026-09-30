package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeResolver struct {
	links  map[string]string
	err    error
	called []string
}

func (f *fakeResolver) Resolve(ctx context.Context, code string) (string, error) {
	f.called = append(f.called, code)
	if f.err != nil {
		return "", f.err
	}
	long, ok := f.links[code]
	if !ok {
		return "", fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
	}
	return long, nil
}

func TestRedirectResolvesShortCode(t *testing.T) {
	resolver := &fakeResolver{links: map[string]string{
		"aB3xK9z": "https://boards.example.com/jobs/1",
	}}
	server := httptest.NewServer(NewHandler(resolver))
	defer server.Close()

	// The client must not follow the redirect, or we would measure the far end.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(server.URL + "/s/aB3xK9z")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://boards.example.com/jobs/1" {
		t.Fatalf("unexpected Location %q", got)
	}
	if len(resolver.called) != 1 || resolver.called[0] != "aB3xK9z" {
		t.Fatalf("expected the code to be resolved once, got %+v", resolver.called)
	}
}

func TestRedirectUnknownCode(t *testing.T) {
	server := httptest.NewServer(NewHandler(&fakeResolver{links: map[string]string{}}))
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/s/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRedirectUpstreamFailure(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("connection refused")}
	server := httptest.NewServer(NewHandler(resolver))
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/s/aB3xK9z")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if strings.Contains(body, "connection refused") {
		t.Fatalf("expected the internal error to stay hidden, got %q", body)
	}
}

func TestRejectsNonGETRequests(t *testing.T) {
	resolver := &fakeResolver{links: map[string]string{"aB3xK9z": "https://example.com/jobs/1"}}
	server := httptest.NewServer(NewHandler(resolver))
	defer server.Close()

	resp, err := server.Client().Post(server.URL+"/s/aB3xK9z", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
	if len(resolver.called) != 0 {
		t.Fatalf("expected no resolution attempt, got %+v", resolver.called)
	}
}

func TestHealthAndIndex(t *testing.T) {
	server := httptest.NewServer(NewHandler(&fakeResolver{links: map[string]string{}}))
	defer server.Close()

	for path, want := range map[string]string{
		"/healthz": "ok",
		"/":        "jobtracker short links",
	} {
		resp, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, resp.StatusCode)
		}
		body := readBody(t, resp)
		_ = resp.Body.Close()
		if !strings.Contains(body, want) {
			t.Fatalf("%s: expected body containing %q, got %q", path, want, body)
		}
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
