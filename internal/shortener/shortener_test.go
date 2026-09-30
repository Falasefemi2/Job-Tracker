package shortener

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Falasefemi2/jobtracker/internal/domain"
)

type fakeStore struct {
	byCode       map[string]domain.ShortURL
	byLongURL    map[string]domain.ShortURL
	created      []domain.ShortURL
	clicks       []string
	collideNext  bool
	collideEvery bool
	lookupErr    error
	createErr    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byCode:    map[string]domain.ShortURL{},
		byLongURL: map[string]domain.ShortURL{},
	}
}

func (f *fakeStore) GetByCode(ctx context.Context, code string) (domain.ShortURL, error) {
	short, ok := f.byCode[code]
	if !ok {
		return domain.ShortURL{}, fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
	}
	return short, nil
}

func (f *fakeStore) GetByLongURL(ctx context.Context, longURL string) (domain.ShortURL, error) {
	if f.lookupErr != nil {
		return domain.ShortURL{}, f.lookupErr
	}
	short, ok := f.byLongURL[longURL]
	if !ok {
		return domain.ShortURL{}, fmt.Errorf("no short code for %q: %w", longURL, sql.ErrNoRows)
	}
	return short, nil
}

func (f *fakeStore) Create(ctx context.Context, s domain.ShortURL) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.collideEvery || f.collideNext {
		f.collideNext = false
		return fmt.Errorf("short code %q is taken: %w", s.Code, ErrCodeTaken)
	}
	f.byCode[s.Code] = s
	f.byLongURL[s.LongURL] = s
	f.created = append(f.created, s)
	return nil
}

func (f *fakeStore) IncrementClicks(ctx context.Context, code string) error {
	f.clicks = append(f.clicks, code)
	short, ok := f.byCode[code]
	if !ok {
		return fmt.Errorf("short code %q not found: %w", code, sql.ErrNoRows)
	}
	short.Clicks++
	f.byCode[code] = short
	return nil
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "empty stays empty", raw: "   ", want: ""},
		{name: "bare host gains https", raw: "boards.example.com/jobs/1", want: "https://boards.example.com/jobs/1"},
		{name: "existing scheme kept", raw: "http://example.com/a", want: "http://example.com/a"},
		{name: "query preserved", raw: "example.com/jobs?id=7&x=1", want: "https://example.com/jobs?id=7&x=1"},
		{name: "unsupported scheme", raw: "ftp://files.example.com/job", wantErr: "only http and https"},
		{name: "missing host", raw: "https:///jobs/1", wantErr: "missing host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.raw)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCodeIsRandomBase62(t *testing.T) {
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		code, err := Code()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != codeLength {
			t.Fatalf("expected %d characters, got %d (%q)", codeLength, len(code), code)
		}
		for _, r := range code {
			if !strings.ContainsRune(codeAlphabet, r) {
				t.Fatalf("character %q is outside the base62 alphabet", r)
			}
		}
		if seen[code] {
			t.Fatalf("code %q was generated twice in %d draws", code, i)
		}
		seen[code] = true
	}
}

func TestShortenCreatesAndReuses(t *testing.T) {
	store := newFakeStore()
	s := New(store, "https://jt.example.com/")

	link, err := s.Shorten(context.Background(), "boards.example.com/jobs/1")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected one stored mapping, got %d", len(store.created))
	}
	if store.created[0].LongURL != "https://boards.example.com/jobs/1" {
		t.Fatalf("unexpected stored URL %q", store.created[0].LongURL)
	}
	if link != "https://jt.example.com/s/"+store.created[0].Code {
		t.Fatalf("unexpected short link %q", link)
	}

	again, err := s.Shorten(context.Background(), "  boards.example.com/jobs/1  ")
	if err != nil {
		t.Fatal(err)
	}
	if again != link {
		t.Fatalf("expected the same short link %q, got %q", link, again)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected the mapping to be reused, got %d creates", len(store.created))
	}
}

func TestShortenSkipsEmptyURL(t *testing.T) {
	store := newFakeStore()
	link, err := New(store, "").Shorten(context.Background(), "  ")
	if err != nil || link != "" {
		t.Fatalf("expected an empty link and no error, got %q %v", link, err)
	}
	if len(store.created) != 0 {
		t.Fatalf("expected nothing stored, got %+v", store.created)
	}
}

func TestShortenRetriesOnCodeCollision(t *testing.T) {
	store := newFakeStore()
	store.collideNext = true

	link, err := New(store, "").Shorten(context.Background(), "https://example.com/jobs/1")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected one stored mapping, got %d", len(store.created))
	}
	if link != DefaultBaseURL+"/s/"+store.created[0].Code {
		t.Fatalf("unexpected short link %q", link)
	}
}

func TestShortenGivesUpAfterRepeatedCollisions(t *testing.T) {
	store := newFakeStore()
	store.collideEvery = true

	_, err := New(store, "").Shorten(context.Background(), "https://example.com/jobs/1")
	if err == nil {
		t.Fatal("expected an error after exhausting code attempts")
	}
	if !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("expected ErrCodeTaken to survive, got %v", err)
	}
}

func TestShortenSurfacesStoreErrors(t *testing.T) {
	store := newFakeStore()
	store.createErr = errors.New("disk on fire")
	_, err := New(store, "").Shorten(context.Background(), "https://example.com/jobs/1")
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("expected the create error to propagate, got %v", err)
	}
}

func TestLookupDoesNotCreate(t *testing.T) {
	store := newFakeStore()
	store.byLongURL["https://example.com/jobs/1"] = domain.ShortURL{Code: "aB3xK9z", LongURL: "https://example.com/jobs/1"}
	s := New(store, "https://jt.example.com")

	link, err := s.Lookup(context.Background(), "example.com/jobs/1")
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://jt.example.com/s/aB3xK9z" {
		t.Fatalf("unexpected link %q", link)
	}

	missing, err := s.Lookup(context.Background(), "https://example.com/jobs/2")
	if err != nil || missing != "" {
		t.Fatalf("expected an empty link and no error, got %q %v", missing, err)
	}
	if len(store.created) != 0 {
		t.Fatalf("expected lookup to stay read-only, got %+v", store.created)
	}
}

func TestResolveIncrementsClicks(t *testing.T) {
	store := newFakeStore()
	store.byCode["aB3xK9z"] = domain.ShortURL{Code: "aB3xK9z", LongURL: "https://example.com/jobs/1"}
	s := New(store, "")

	for i := 1; i <= 3; i++ {
		long, err := s.Resolve(context.Background(), "aB3xK9z")
		if err != nil {
			t.Fatal(err)
		}
		if long != "https://example.com/jobs/1" {
			t.Fatalf("unexpected target %q", long)
		}
		if store.byCode["aB3xK9z"].Clicks != int64(i) {
			t.Fatalf("expected %d clicks, got %d", i, store.byCode["aB3xK9z"].Clicks)
		}
	}
}

func TestResolveUnknownCode(t *testing.T) {
	_, err := New(newFakeStore(), "").Resolve(context.Background(), "nope123")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows so the handler can answer 404, got %v", err)
	}
}

func TestResolveEmptyCode(t *testing.T) {
	_, err := New(newFakeStore(), "").Resolve(context.Background(), "  ")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows so the handler can answer 404, got %v", err)
	}
}

func TestLinkTrimsBaseURL(t *testing.T) {
	if got := New(newFakeStore(), "  https://jt.example.com///  ").Link("aB3xK9z"); got != "https://jt.example.com/s/aB3xK9z" {
		t.Fatalf("unexpected link %q", got)
	}
	if got := New(newFakeStore(), "").Link("  "); got != "" {
		t.Fatalf("expected an empty link for a blank code, got %q", got)
	}
}
