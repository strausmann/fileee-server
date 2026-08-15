package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestUpstreamTimeout_HangingUpstreamReturns504 ist der Kern-Regressionstest für Issue #44: ein
// Upstream, der auf einen authentifizierten Request NIE antwortet (Verbindung offen, 0 Bytes —
// exakt das live beobachtete Symptom), darf den Handler NICHT unbegrenzt blockieren. Mit einem
// kurzen cfg.UpstreamTimeout MUSS der Request innerhalb einer klar von der (viel größeren)
// Test-Deadline unterscheidbaren Frist mit 504 "upstream_timeout" scheitern, statt (wie vor
// dieser Änderung) bis zum Client-Timeout oder ewig zu hängen.
func TestUpstreamTimeout_HangingUpstreamReturns504(t *testing.T) {
	routes := map[string]mockRoute{
		"POST /api/document-types/rest/query": {Hang: true},
	}
	cfg := Config{UpstreamTimeout: 80 * time.Millisecond}
	_, ts := newTestServerWithConfig(t, cfg, routes)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/document-types", nil)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/document-types: %v", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusGatewayTimeout {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, http.StatusGatewayTimeout, body)
	}
	var got statusError
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("Body als statusError dekodieren: %v", err)
	}
	if got.ErrorCode != "upstream_timeout" {
		t.Errorf("code = %q, want %q", got.ErrorCode, "upstream_timeout")
	}
	// Großzügige Obergrenze (1s) weit über der 80ms-Deadline, aber weit unter jeder Größenordnung,
	// die auf "hat doch gehangen" hindeuten würde — belegt, dass die Deadline tatsächlich griff,
	// statt nur zufällig durch einen anderen Pfad schnell zurückzukommen.
	if elapsed > time.Second {
		t.Fatalf("elapsed = %v, want deutlich unter 1s (80ms-Deadline haette greifen muessen)", elapsed)
	}
}

// TestUpstreamTimeout_ZeroDisablesDeadline stellt sicher, dass Config{} (UpstreamTimeout im
// Zero-Value 0) KEINE Deadline erzwingt — Voraussetzung dafür, dass die hunderte bestehenden
// Handler-Tests (die Config{} ohne dieses Feld bauen) durch diese Änderung nicht reihenweise mit
// einer sofort abgelaufenen 0s-Deadline brechen. Der Mock-Upstream antwortet absichtlich erst
// nach einer kurzen, vom Test kontrollierten Verzögerung (länger als die 80ms aus dem
// Hang-Test oben) — mit UpstreamTimeout=0 MUSS der Request trotzdem normal mit 200 durchgehen.
func TestUpstreamTimeout_ZeroDisablesDeadline(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	// Sicherheitsnetz, falls die Assertions unten fehlschlagen/paniken, BEVOR der reguläre
	// close() weiter unten läuft — sonst bliebe der Mock-Handler blockiert und
	// t.Cleanup(mockSrv.Close) (newTestFileeeClient) würde den Testlauf verzögern.
	t.Cleanup(closeRelease)

	routes := map[string]mockRoute{
		"POST /api/document-types/rest/query": {
			DelayUntil: release,
			Status:     http.StatusOK,
			Body:       []byte(`{"rows":[],"totalRows":0}`),
		},
	}
	cfg := Config{} // UpstreamTimeout bleibt Zero-Value (0) — wie in ~allen bestehenden Tests.
	_, ts := newTestServerWithConfig(t, cfg, routes)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/document-types", nil)

	done := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		done <- resp
	}()

	// Release erst NACH der 80ms-Frist aus dem Hang-Test — würde UpstreamTimeout=0 fälschlich
	// doch eine Deadline erzwingen, käme hier ein 504 statt eines blockierten Requests an.
	time.Sleep(150 * time.Millisecond)
	closeRelease()

	select {
	case err := <-errCh:
		t.Fatalf("GET /v1/document-types: %v", err)
	case resp := <-done:
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (UpstreamTimeout=0 sollte keine Deadline erzwingen), body=%s", resp.StatusCode, body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Request kam nicht zurück — Test-Setup selbst haengt")
	}
}

// TestUpstreamTimeout_WaitProcessExemptFromShortDeadline ist der kritische Abgrenzungstest: die
// UpstreamTimeout-Middleware darf POST /v1/processes/{id}/wait NICHT einschränken, obwohl diese
// Route (Design-Spec §4.4) bewusst bis zu cfg.WaitMax (bis zu 300s) blockiert. Ein kurzes
// cfg.UpstreamTimeout (50ms), deutlich unter dem angeforderten ?timeout=300ms, darf die Route
// deshalb NICHT vorzeitig mit 504 abschneiden — ansonsten wäre die Wait-Semantik aus
// TestWaitProcess_NonTerminalTimeoutReturns200 durch diese Änderung gebrochen.
func TestUpstreamTimeout_WaitProcessExemptFromShortDeadline(t *testing.T) {
	routes := map[string]mockRoute{
		"GET /api/processes/proc-1": {
			Status: http.StatusOK,
			Body:   []byte(`{"id":"proc-1","status":"Running"}`),
		},
	}
	cfg := Config{WaitTimeout: 5 * time.Second, WaitMax: 5 * time.Second, UpstreamTimeout: 50 * time.Millisecond}
	_, ts := newTestServerWithConfig(t, cfg, routes)

	req := newAuthedRequest(t, http.MethodPost, ts.URL+"/v1/processes/proc-1/wait?timeout=300ms", nil)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/processes/proc-1/wait: %v", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (wait-process muss von der 50ms-UpstreamTimeout ausgenommen sein), body=%s", resp.StatusCode, body)
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= ~300ms (Beleg, dass die 50ms-Deadline NICHT griff)", elapsed)
	}
}

// TestUpstreamTimeout_SearchHydrationExempt ist der HIGH-Review-Fund zu Issue #44 (Lens-Review
// von PR #45): GET /v1/documents im SUCHMODUS (?query= gesetzt) macht Documents.Search + EINEN
// Documents.Get-Call PRO TREFFER (N+1-Hydration, handleListDocuments, handlers_documents.go) —
// Limit ist caller-gesteuert ohne Obergrenze. cfg.UpstreamTimeout ist als Middleware ein
// Wall-Clock-Budget für den GESAMTEN Request, nicht pro Einzel-Call — ohne Ausnahme würde jede
// nicht-triviale Suche mitten in der Hydration mit 504 abbrechen, obwohl sie vor dieser Änderung
// (langsam, aber unbegrenzt) erfolgreich durchlief. Der Fix (isUpstreamTimeoutExempt) nimmt
// GET /v1/documents MIT gesetztem query-Parameter aus derselben Begründung aus wie wait-process/
// upload/export: von Natur aus variable, potenziell lange Laufzeit. Der per-Call
// ResponseHeaderTimeout=30s von go-fileee (defaultTransport, client.go) schützt WEITERHIN jeden
// einzelnen Search-/Get-Call gegen einen echten Wedge — nur das Wall-Clock-Budget über die ganze
// Schleife entfällt.
//
// Simuliert 5 Treffer mit je 40ms Verzögerung pro Get-Hydration-Call (200ms Gesamtdauer) gegen
// eine 80ms-UpstreamTimeout — ohne die Ausnahme würde das zuverlässig 504en.
func TestUpstreamTimeout_SearchHydrationExempt(t *testing.T) {
	routes := map[string]mockRoute{
		"POST /api/documents/rest/query": {
			Status: http.StatusOK,
			Body:   []byte(`{"rows":["doc-1","doc-2","doc-3","doc-4","doc-5"],"totalRows":5}`),
		},
		// Wildcard-Pattern trifft JEDEN der 5 Get-Hydration-Calls einzeln — jeder bekommt seine
		// eigene 40ms-Verzögerung (siehe mockRoute.Delay-Doku), kumuliert 200ms > die 80ms
		// UpstreamTimeout unten.
		"GET /api/documents/rest/{id}": {
			Delay:  40 * time.Millisecond,
			Status: http.StatusOK,
			Body:   []byte(`{"id":"doc-x","version":1,"status":"DONE"}`),
		},
	}
	// newTestServerWithConfig/newTestFileeeClient (handlers_test.go) verdrahten fc bewusst OHNE
	// fileee.WithRateLimit — go-fileees eigener NewClient-Default greift dann (rps=1, burst=3,
	// client.go). Bei EnsureSession-vor-jedem-Call (Config.WithSessionFreshness bleibt hier
	// unangetastet, Default 0 = jeder Call verifiziert neu) verbraucht dieser Test 1 Search + 5
	// Get-Hydration-Calls × 2 Tokens (EnsureSession + eigentlicher Call) = 12 Tokens — weit über
	// dem Burst von 3, macht den Test unnötig langsam (real gemessen: ~9s statt ~200ms) OHNE
	// irgendetwas über die UpstreamTimeout-Exemption zu beweisen (reines Test-Rauschen von einem
	// UNRELATED Rate-Limiter). newTestServerWithRateUnlimitedClient baut deshalb — exakt wie
	// bereits in handlers_documents_pagination_test.go/watch_test.go etabliert (dortiger
	// Kommentar: "hebt den Default-Token-Bucket auf") — einen eigenen fc MIT
	// fileee.WithRateLimit(1000, 1000).
	cfg := Config{UpstreamTimeout: 80 * time.Millisecond}
	_, ts := newTestServerWithRateUnlimitedClient(t, cfg, routes)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/documents?query=rechnung", nil)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/documents?query=rechnung: %v", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("Body lesen: %v", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (Suchmodus muss von der 80ms-UpstreamTimeout ausgenommen sein), body=%s", resp.StatusCode, body)
	}
	if elapsed < 180*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= ~200ms (Beleg, dass alle 5 Hydration-Calls tatsächlich liefen und die 80ms-Deadline NICHT griff)", elapsed)
	}

	var decoded documentListBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Body als documentListBody dekodieren: %v", err)
	}
	if len(decoded.Items) != 5 {
		t.Fatalf("Items = %d, want 5 (alle Treffer hydriert)", len(decoded.Items))
	}
}

// TestIsUpstreamTimeoutExempt prüft die Ausnahmeliste der Middleware direkt gegen echte
// Methode/Pfad-Kombinationen (inkl. Query-String, wo relevant) — ohne den vollen Server-Umweg.
// Ausgenommen sind ausschließlich Routen mit von Natur aus variabler/langer Laufzeit
// (Design-Analog zu go-fileees eigenem Verzicht auf ein pauschales http.Client.Timeout, siehe
// Config.UpstreamTimeout-Doku): der blockierende Wait-Endpunkt, Upload, ZIP-Export, binäre
// PDF-/Bild-Downloads (direkt und über den anonymen Share-Proxy) sowie GET /v1/documents im
// Suchmodus (N+1-Hydration, siehe TestUpstreamTimeout_SearchHydrationExempt).
func TestIsUpstreamTimeoutExempt(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/v1/processes/proc-1/wait", true},
		{http.MethodPost, "/v1/documents", true},
		{http.MethodPost, "/v1/documents/export-zip", true},
		{http.MethodGet, "/v1/documents/doc-1/pdf", true},
		{http.MethodGet, "/v1/pages/page-1/image", true},
		{http.MethodGet, "/v1/share-objects/tok/documents/doc-1/pdf", true},
		{http.MethodGet, "/v1/share-objects/tok/pages/page-1/image", true},
		{http.MethodGet, "/v1/documents?query=rechnung", true},

		{http.MethodGet, "/v1/document-types", false},
		{http.MethodGet, "/v1/documents", false},
		// Leerer query-Parameter (Key vorhanden, Wert leer) aktiviert NICHT den Suchmodus —
		// handleListDocuments prüft `in.Query != ""` (handlers_documents.go), der Page-Zweig
		// läuft, also keine Ausnahme.
		{http.MethodGet, "/v1/documents?query=", false},
		{http.MethodGet, "/v1/documents/doc-1", false},
		{http.MethodGet, "/v1/processes/proc-1", false},
		{http.MethodPost, "/v1/share", false},
		{http.MethodPost, "/v1/documents/doc-1/unshare", false},
		{http.MethodDelete, "/v1/documents/doc-1", false},
		{http.MethodGet, "/v1/pages/page-1/ocr", false},
		{http.MethodGet, "/healthz", false},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			if got := isUpstreamTimeoutExempt(r); got != tc.want {
				t.Errorf("isUpstreamTimeoutExempt(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
			}
		})
	}
}
