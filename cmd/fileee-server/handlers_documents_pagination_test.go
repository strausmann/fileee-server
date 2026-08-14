package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/strausmann/go-fileee/fileee"
)

// This file is the regression coverage for issue #39: "GET /v1/documents diff cursor does not
// paginate (blocks full document iteration)". See documentPageCursor's doc comment
// (handlers_documents.go) for the root-cause analysis (go-fileee@v0.2.0's restService[T].Diff
// never advances Start) and why the fix routes the no-query branch through Documents.Query
// instead.
//
// The mock Fileee backend below is deliberately STATEFUL and driven by the request's actual
// "start"/"limit" fields (decoded from the JSON body), not by a call counter or a fixed
// mockRoute — this is what makes the test a genuine regression guard: run it against the OLD
// Documents.Diff-based implementation (which always sent start=0) and it fails exactly the way
// the live-reproduced bug did (page 2 == page 1). Run it against the fix, and Start actually
// advances between calls, so it passes.

// paginatingQueryFixture is a small, fixed 5-document corpus used to exercise multi-page
// pagination end to end.
var paginatingQueryFixture = []string{
	`{"id":"doc-1","version":1,"status":"DONE"}`,
	`{"id":"doc-2","version":1,"status":"DONE"}`,
	`{"id":"doc-3","version":1,"status":"DONE"}`,
	`{"id":"doc-4","version":1,"status":"DONE"}`,
	`{"id":"doc-5","version":1,"status":"DONE"}`,
}

// newPaginatingTestServer builds a *Server backed by a mock Fileee upstream whose
// "POST /api/documents/rest/query" route genuinely honors the request's start/limit fields
// against docs (returning docs[start:start+limit] and totalRows=len(docs)) — unlike the static
// mockRoute fixtures used elsewhere in this package (handlers_test.go), which always answer with
// the same fixed body regardless of what the client sent. calls, if non-nil, is incremented once
// per request the mock backend receives (used by tests that assert an exact call count).
func newPaginatingTestServer(t *testing.T, docs []string, calls *int) (*Server, *httptest.Server) {
	t.Helper()

	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/f/user-session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authorized":true,"secondsBlocked":0}`))
	})
	mux.HandleFunc("GET /api/f/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/documents/rest/query", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if calls != nil {
			*calls++
		}
		mu.Unlock()

		var req struct {
			Start int `json:"start"`
			Limit int `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}

		start := req.Start
		limit := req.Limit
		if limit <= 0 {
			limit = len(docs)
		}
		end := start + limit
		var page []string
		if start >= 0 && start < len(docs) {
			if end > len(docs) {
				end = len(docs)
			}
			page = docs[start:end]
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body := fmt.Sprintf(`{"rows":[%s],"totalRows":%d}`, joinJSON(page), len(docs))
		_, _ = w.Write([]byte(body))
	})

	mockSrv := httptest.NewServer(mux)
	t.Cleanup(mockSrv.Close)

	store := fileee.NewFileSessionStore(filepath.Join(t.TempDir(), "session.json"))
	seedSession := &fileee.Session{
		Cookies: []*http.Cookie{{Name: "JSESSIONID", Value: testJWTWithSub("test-user-1")}},
		SavedAt: time.Now(),
	}
	if err := store.Save(context.Background(), seedSession); err != nil {
		t.Fatalf("seed session store: %v", err)
	}

	creds := fileee.Credentials{Username: "test@example.invalid", Password: "test-pw"}
	fc, err := fileee.NewClient(creds,
		fileee.WithBaseURL(mockSrv.URL),
		fileee.WithSessionStore(store),
		fileee.WithRateLimit(1000, 1000),
	)
	if err != nil {
		t.Fatalf("fileee.NewClient: %v", err)
	}
	sc := fileee.NewShareClient(
		fileee.WithBaseURL(mockSrv.URL), fileee.WithStaticBaseURL(mockSrv.URL),
		fileee.WithRateLimit(1000, 1000),
	)

	cfg := Config{
		APIToken:        testAPIToken,
		DocsPublic:      true,
		ClientIPHeaders: defaultClientIPHeaders,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewServer(cfg, fc, sc, log)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	return s, ts
}

// joinJSON concatenates already-JSON-encoded document fixtures with commas — a tiny helper so
// the mock handler above stays readable.
func joinJSON(rows []string) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += ","
		}
		out += r
	}
	return out
}

// fetchDocumentsPage performs one GET /v1/documents?limit=...&cursor=... call against ts and
// decodes the response.
func fetchDocumentsPage(t *testing.T, ts *httptest.Server, limit int, cursor string) documentListBody {
	t.Helper()

	url := fmt.Sprintf("%s/v1/documents?limit=%d", ts.URL, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	req := newAuthedRequest(t, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}

	var got documentListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode documentListBody: %v (body=%s)", err, body)
	}
	return got
}

// TestListDocuments_WithoutQuery_CursorAdvancesAcrossPages is the direct acceptance test for
// issue #39's first checkbox: "Passing the cursor from response N to request N+1 returns the
// next page (new document IDs), not a repeat." Two consecutive GET /v1/documents calls, the
// second one passing the cursor the first returned, must yield DIFFERENT documents.
func TestListDocuments_WithoutQuery_CursorAdvancesAcrossPages(t *testing.T) {
	_, ts := newPaginatingTestServer(t, paginatingQueryFixture, nil)

	page1 := fetchDocumentsPage(t, ts, 2, "")
	if len(page1.Items) != 2 {
		t.Fatalf("page1 Items = %+v, want 2 documents", page1.Items)
	}
	if page1.Items[0].ID != "doc-1" || page1.Items[1].ID != "doc-2" {
		t.Fatalf("page1 = %+v, want doc-1,doc-2", page1.Items)
	}
	if page1.Cursor == "" {
		t.Fatal("page1.Cursor is empty, expected a follow-up token (5 documents, limit 2)")
	}

	page2 := fetchDocumentsPage(t, ts, 2, page1.Cursor)
	if len(page2.Items) != 2 {
		t.Fatalf("page2 Items = %+v, want 2 documents", page2.Items)
	}
	if page2.Items[0].ID != "doc-3" || page2.Items[1].ID != "doc-4" {
		t.Fatalf("page2 = %+v, want doc-3,doc-4 (i.e. NOT a repeat of page1) — this is the exact"+
			" issue #39 symptom if it fails: same first/last document ID as page1", page2.Items)
	}

	// The regression itself, stated as directly as possible: page2 must not be byte-identical to
	// page1 (before the fix, both responses' Items were identical).
	if page1.Items[0].ID == page2.Items[0].ID {
		t.Fatalf("page1 and page2 both start with %q — cursor did not advance (issue #39)", page1.Items[0].ID)
	}
}

// TestListDocuments_WithoutQuery_CursorTerminatesAtEnd is the acceptance test for issue #39's
// second checkbox: "Iterating from an empty/initial cursor eventually reaches the end (no
// infinite repeat, correct termination)." It walks the full 5-document fixture with a
// page size that does not evenly divide it (limit=2 over 5 documents: pages of 2, 2, 1), and
// asserts: every document is seen EXACTLY once, in order, and the walk terminates (empty
// cursor) instead of looping.
func TestListDocuments_WithoutQuery_CursorTerminatesAtEnd(t *testing.T) {
	var calls int
	_, ts := newPaginatingTestServer(t, paginatingQueryFixture, &calls)

	const limit = 2
	const maxIterations = 10 // generous ceiling; a real infinite loop is caught long before this

	var seen []string
	cursor := ""
	iterations := 0
	for {
		iterations++
		if iterations > maxIterations {
			t.Fatalf("did not terminate after %d iterations (seen so far: %v) — infinite repeat (issue #39)", maxIterations, seen)
		}

		page := fetchDocumentsPage(t, ts, limit, cursor)
		if len(page.Items) == 0 && cursor != "" {
			t.Fatal("got an empty page with a non-empty request cursor — should have terminated one call earlier instead")
		}
		for _, item := range page.Items {
			seen = append(seen, item.ID)
		}
		if page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}

	wantIDs := []string{"doc-1", "doc-2", "doc-3", "doc-4", "doc-5"}
	if len(seen) != len(wantIDs) {
		t.Fatalf("seen = %v (len %d), want %v (len %d) — duplicates or missing documents", seen, len(seen), wantIDs, len(wantIDs))
	}
	for i, id := range wantIDs {
		if seen[i] != id {
			t.Errorf("seen[%d] = %q, want %q", i, seen[i], id)
		}
	}

	// 5 documents / limit 2 => pages of 2, 2, 1 => exactly 3 upstream calls. More would mean an
	// extra, wasted trailing call after termination should have already happened; fewer would
	// mean documents were skipped.
	if calls != 3 {
		t.Errorf("upstream Documents.Query calls = %d, want 3 (pages of 2, 2, 1)", calls)
	}
}

// TestListDocuments_WithoutQuery_EmptyAccountTerminatesImmediately is the degenerate case of
// termination: an account with zero documents must return an empty cursor right away instead of
// requiring the caller to guess that "no items" also means "no more pages".
func TestListDocuments_WithoutQuery_EmptyAccountTerminatesImmediately(t *testing.T) {
	_, ts := newPaginatingTestServer(t, nil, nil)

	page := fetchDocumentsPage(t, ts, 100, "")
	if len(page.Items) != 0 {
		t.Fatalf("Items = %+v, want empty", page.Items)
	}
	if page.Cursor != "" {
		t.Errorf("Cursor = %q, want empty (empty account has no further page)", page.Cursor)
	}
	if page.TotalRows != 0 {
		t.Errorf("TotalRows = %d, want 0", page.TotalRows)
	}
}
