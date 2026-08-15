package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// UpstreamTimeout deckelt den Request-Context jedes NICHT ausgenommenen Requests auf timeout
// (Issue #44) — Handler, die einen Upstream-Call an s.fc/s.sc delegieren, erben diese Deadline
// über ctx (huma reicht r.Context() unverändert durch, siehe handlers_*.go). Läuft die Deadline
// ab, während der Handler auf eine Fileee-Antwort wartet, bricht der zugrunde liegende
// http.Transport den Roundtrip mit context.DeadlineExceeded ab; mapError (errors.go) übersetzt
// das auf HTTP 504 "upstream_timeout" statt den Request unbegrenzt hängen zu lassen.
//
// timeout <= 0 deaktiviert die Middleware vollständig (kein context.WithTimeout, next.ServeHTTP
// bekommt r unverändert) — siehe Config.UpstreamTimeout-Doku für die Begründung des 0-als-"aus"-
// Defaults. exempt entscheidet PRO REQUEST, ob überhaupt eine Deadline gesetzt wird; ist exempt
// nil, wird für KEINEN Request eine Ausnahme gemacht.
func UpstreamTimeout(timeout time.Duration, exempt func(r *http.Request) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if timeout <= 0 || (exempt != nil && exempt(r)) {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isUpstreamTimeoutExempt liefert true für Routen, deren Laufzeit von Natur aus variabel oder
// lang ist und die deshalb NICHT der kurzen UpstreamTimeout-Deadline unterliegen dürfen — dieselbe
// Begründung, aus der go-fileees eigener http.Client bewusst KEIN pauschales Timeout setzt (siehe
// Config.UpstreamTimeout-Doku): ein zu kurzes Timeout würde große Uploads/ZIP-Exports/Downloads
// mittendrin abschneiden, und POST /v1/processes/{id}/wait blockiert AUSDRÜCKLICH bis zu
// cfg.WaitMax (bis zu 300s, Design-Spec §4.4) — das ist kein Hänger, sondern die Handler-eigene,
// bereits selbst gedeckelte Wartesemantik (handleWaitProcess, handlers_share.go).
//
// Ausgenommen (Methode + Pfad-Muster):
//   - POST /v1/processes/{id}/wait               — Suffix "/wait"
//   - POST /v1/documents                          — Upload (exakter Pfad, NICHT GET-Liste)
//   - POST /v1/documents/export-zip               — ZIP-Export (exakter Pfad)
//   - GET  .../pdf                                 — Voll-PDF, direkt UND Share-Proxy
//   - GET  .../image                               — Seitenbild, direkt UND Share-Proxy
//
// Alle übrigen Routen (inkl. der schlanken JSON-GETs/-POSTs, deren unbegrenztes Hängen Issue #44
// überhaupt erst auslöste) bleiben der Deadline unterworfen.
func isUpstreamTimeoutExempt(r *http.Request) bool {
	path := r.URL.Path
	switch r.Method {
	case http.MethodPost:
		if strings.HasSuffix(path, "/wait") {
			return true
		}
		if path == "/v1/documents" || path == "/v1/documents/export-zip" {
			return true
		}
	case http.MethodGet:
		if strings.HasSuffix(path, "/pdf") || strings.HasSuffix(path, "/image") {
			return true
		}
	}
	return false
}
