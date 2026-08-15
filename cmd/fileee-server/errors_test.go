package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/strausmann/go-fileee/fileee"
)

// wantStatus liest über errors.As den HTTP-Status aus dem von mapError gelieferten Fehler (via
// huma.StatusError) — unabhängig davon, ob der Fehler zusätzlich mit Retry-After-Headern
// umschlossen ist (huma.ErrorWithHeaders liefert einen Wrapper, der nur huma.HeadersError
// erfüllt; errors.As findet den inneren huma.StatusError über dessen Unwrap-Kette).
func wantStatus(t *testing.T, err error) int {
	t.Helper()
	var se huma.StatusError
	if !errors.As(err, &se) {
		t.Fatalf("mapError-Resultat erfüllt huma.StatusError nicht: %#v", err)
	}
	return se.GetStatus()
}

// wantBody liest über errors.As den eigenen *statusError (Feldpaar error/code) aus dem von
// mapError gelieferten Fehler.
func wantBody(t *testing.T, err error) *statusError {
	t.Helper()
	var se *statusError
	if !errors.As(err, &se) {
		t.Fatalf("mapError-Resultat enthält keinen *statusError: %#v", err)
	}
	return se
}

func TestMapError_Nil(t *testing.T) {
	if err := mapError(nil); err != nil {
		t.Fatalf("mapError(nil) = %v, want nil", err)
	}
}

func TestMapError_DuplicateDocument(t *testing.T) {
	got := mapError(fileee.ErrDuplicateDocument)
	if status := wantStatus(t, got); status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
	if code := wantBody(t, got).ErrorCode; code != "duplicate" {
		t.Errorf("code = %q, want %q", code, "duplicate")
	}
}

func TestMapError_UnsupportedFileType(t *testing.T) {
	got := mapError(fileee.ErrUnsupportedFileType)
	if status := wantStatus(t, got); status != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", status, http.StatusUnsupportedMediaType)
	}
	if code := wantBody(t, got).ErrorCode; code != "unsupported_file_type" {
		t.Errorf("code = %q, want %q", code, "unsupported_file_type")
	}
}

// TestMapError_RateLimited_ViaAPIError belegt, dass ein *fileee.APIError mit HTTPStatus 429 über
// dessen Is-Methode (fileee/errors.go) als ErrRateLimited erkannt wird — ohne dass mapError den
// Status selbst raten muss.
func TestMapError_RateLimited_ViaAPIError(t *testing.T) {
	apiErr := &fileee.APIError{HTTPStatus: http.StatusTooManyRequests}
	got := mapError(apiErr)
	if status := wantStatus(t, got); status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", status, http.StatusTooManyRequests)
	}
	if code := wantBody(t, got).ErrorCode; code != "rate_limited" {
		t.Errorf("code = %q, want %q", code, "rate_limited")
	}
}

// TestMapError_Blocked_RetryAfter belegt den Huma-Header-aus-Fehler-Mechanismus: mapError liefert
// für *fileee.BlockedError einen Fehler, der huma.HeadersError erfüllt und SecondsBlocked als
// Retry-After-Header (Sekunden, als String) trägt.
func TestMapError_Blocked_RetryAfter(t *testing.T) {
	got := mapError(&fileee.BlockedError{SecondsBlocked: 42})
	if status := wantStatus(t, got); status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}

	var he huma.HeadersError
	if !errors.As(got, &he) {
		t.Fatalf("mapError-Resultat für BlockedError erfüllt huma.HeadersError nicht: %#v", got)
	}
	if ra := he.GetHeaders().Get("Retry-After"); ra != "42" {
		t.Errorf("Retry-After = %q, want %q", ra, "42")
	}

	if code := wantBody(t, got).ErrorCode; code != "blocked" {
		t.Errorf("code = %q, want %q", code, "blocked")
	}
}

func TestMapError_NotFound(t *testing.T) {
	got := mapError(fileee.ErrNotFound)
	if status := wantStatus(t, got); status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", status, http.StatusNotFound)
	}
	if code := wantBody(t, got).ErrorCode; code != "not_found" {
		t.Errorf("code = %q, want %q", code, "not_found")
	}
}

// TestMapError_NotFound_ViaAPIError belegt denselben Fall wie TestMapError_NotFound, aber über
// einen *fileee.APIError mit HTTPStatus 404 statt des bloßen Sentinel-Fehlers.
func TestMapError_NotFound_ViaAPIError(t *testing.T) {
	apiErr := &fileee.APIError{HTTPStatus: http.StatusNotFound, Code: "NOT_FOUND"}
	got := mapError(apiErr)
	if status := wantStatus(t, got); status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", status, http.StatusNotFound)
	}
}

// TestMapError_SessionExpired bildet nach, wie die Lib ErrSessionExpired nach einem
// fehlgeschlagenen Reauth tatsächlich zurückgibt (fileee/auth.go: errors.Join(ErrSessionExpired,
// fmt.Errorf(...))) — errors.Is muss das über den Join-Baum hinweg finden.
func TestMapError_SessionExpired(t *testing.T) {
	wrapped := errors.Join(fileee.ErrSessionExpired, fmt.Errorf("fileee: reauth fehlgeschlagen: %w", errors.New("boom")))
	got := mapError(wrapped)
	if status := wantStatus(t, got); status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if code := wantBody(t, got).ErrorCode; code != "upstream_auth" {
		t.Errorf("code = %q, want %q", code, "upstream_auth")
	}
}

// TestMapError_DeadlineExceeded_Direct belegt die direkte Zuordnung für Issue #44: ein
// unverpackter context.DeadlineExceeded muss auf 504 "upstream_timeout" abgebildet werden.
func TestMapError_DeadlineExceeded_Direct(t *testing.T) {
	got := mapError(context.DeadlineExceeded)
	if status := wantStatus(t, got); status != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d", status, http.StatusGatewayTimeout)
	}
	if code := wantBody(t, got).ErrorCode; code != "upstream_timeout" {
		t.Errorf("code = %q, want %q", code, "upstream_timeout")
	}
}

// TestMapError_DeadlineExceeded_WrappedLikeRealClient bildet nach, wie ein abgelaufener
// Upstream-Request in der Praxis tatsächlich bei mapError ankommt: go-fileees eigene
// Fehler-Wraps (fmt.Errorf("fileee: get %s: %w", path, err), fileee/client.go) um einen
// *url.Error (net/http.Client.Do), der wiederum context.DeadlineExceeded als Ursache trägt
// (net/url.Error.Unwrap). errors.Is muss das über BEIDE Unwrap-Ebenen hinweg finden.
func TestMapError_DeadlineExceeded_WrappedLikeRealClient(t *testing.T) {
	urlErr := &url.Error{Op: "Get", URL: "https://my.fileee.com/api/document-types/rest/query", Err: context.DeadlineExceeded}
	wrapped := fmt.Errorf("fileee: get %s: %w", "/api/document-types/rest/query", urlErr)

	got := mapError(wrapped)
	if status := wantStatus(t, got); status != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d", status, http.StatusGatewayTimeout)
	}
	if code := wantBody(t, got).ErrorCode; code != "upstream_timeout" {
		t.Errorf("code = %q, want %q", code, "upstream_timeout")
	}
}

// TestMapError_Canceled_Direct belegt den Review-Fund zu PR #45: ein unverpackter
// context.Canceled (Client hat die Verbindung abgebrochen) muss auf 499 "request_canceled"
// abgebildet werden — NICHT auf 500 "internal_error" (das würde einen harmlosen Client-Abbruch
// als Serverfehler in Metriken/Logs zählen).
func TestMapError_Canceled_Direct(t *testing.T) {
	got := mapError(context.Canceled)
	if status := wantStatus(t, got); status != clientClosedRequestStatus {
		t.Errorf("status = %d, want %d", status, clientClosedRequestStatus)
	}
	if code := wantBody(t, got).ErrorCode; code != "request_canceled" {
		t.Errorf("code = %q, want %q", code, "request_canceled")
	}
}

// TestMapError_Canceled_WrappedLikeRealClient bildet nach, wie ein Client-Abbruch in der Praxis
// tatsächlich bei mapError ankommt — analog TestMapError_DeadlineExceeded_WrappedLikeRealClient,
// aber mit context.Canceled statt context.DeadlineExceeded als Ursache (Go's http.Server bricht
// r.Context() bei getrennter Client-Verbindung mit Canceled ab, nicht mit DeadlineExceeded,
// unabhängig davon, ob FILEEE_UPSTREAM_TIMEOUT überhaupt gesetzt ist).
func TestMapError_Canceled_WrappedLikeRealClient(t *testing.T) {
	urlErr := &url.Error{Op: "Get", URL: "https://my.fileee.com/api/document-types/rest/query", Err: context.Canceled}
	wrapped := fmt.Errorf("fileee: get %s: %w", "/api/document-types/rest/query", urlErr)

	got := mapError(wrapped)
	if status := wantStatus(t, got); status != clientClosedRequestStatus {
		t.Errorf("status = %d, want %d", status, clientClosedRequestStatus)
	}
	if code := wantBody(t, got).ErrorCode; code != "request_canceled" {
		t.Errorf("code = %q, want %q", code, "request_canceled")
	}
}

// TestMapError_GenericAPIError_PassThroughStatus belegt, dass ein *fileee.APIError, der keinem
// der bekannten Sentinel-Fälle entspricht, mit SEINEM EIGENEN HTTPStatus durchgereicht wird (Spec
// §12: "sonstiger APIError → dessen HTTPStatus").
func TestMapError_GenericAPIError_PassThroughStatus(t *testing.T) {
	apiErr := &fileee.APIError{HTTPStatus: http.StatusForbidden, Code: "SOME_CODE", Message: "nope"}
	got := mapError(apiErr)
	if status := wantStatus(t, got); status != http.StatusForbidden {
		t.Errorf("status = %d, want %d", status, http.StatusForbidden)
	}
	body := wantBody(t, got)
	if body.ErrorCode != "SOME_CODE" {
		t.Errorf("code = %q, want %q", body.ErrorCode, "SOME_CODE")
	}
	if body.ErrorMsg != "nope" {
		t.Errorf("error = %q, want %q", body.ErrorMsg, "nope")
	}
}

// TestMapError_GenericAPIError_EmptyFields belegt, dass mapError auch bei einem defensiv-leeren
// *fileee.APIError (parseAPIError liefert das z. B. bei leerem/nicht-JSON Body, fileee/errors.go)
// niemals ein leeres error/code-Feld ausliefert.
func TestMapError_GenericAPIError_EmptyFields(t *testing.T) {
	apiErr := &fileee.APIError{HTTPStatus: http.StatusForbidden}
	got := mapError(apiErr)
	body := wantBody(t, got)
	if body.ErrorCode == "" {
		t.Errorf("code darf nicht leer sein")
	}
	if body.ErrorMsg == "" {
		t.Errorf("error darf nicht leer sein")
	}
}

func TestMapError_Unknown(t *testing.T) {
	got := mapError(errors.New("boom"))
	if status := wantStatus(t, got); status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if code := wantBody(t, got).ErrorCode; code != "internal_error" {
		t.Errorf("code = %q, want %q", code, "internal_error")
	}
}

// TestStatusError_ErrorInterface prüft die beiden Interface-Methoden von statusError direkt.
// Beide werden sonst nur indirekt über Huma aufgerufen (error bzw. huma.StatusError), sodass eine
// Regression — etwa ein versehentlich zurückgegebener ErrorCode statt der Meldung — in den
// mapError-Tests unbemerkt bliebe: die prüfen die Felder, nicht die Methoden.
func TestStatusError_ErrorInterface(t *testing.T) {
	err := newStatusError(http.StatusBadRequest, "invalid_cursor", "invalid cursor parameter")

	if got := err.Error(); got != "invalid cursor parameter" {
		t.Errorf("Error() = %q, want %q", got, "invalid cursor parameter")
	}
	if got := err.GetStatus(); got != http.StatusBadRequest {
		t.Errorf("GetStatus() = %d, want 400", got)
	}
	// Der Status steht bewusst NICHT im JSON-Body (unexportiertes Feld, siehe statusError-Doku) —
	// er ist bereits der HTTP-Status-Code. Nur error und code gehören in den Body.
	body, mErr := json.Marshal(err)
	if mErr != nil {
		t.Fatalf("statusError serialisieren: %v", mErr)
	}
	if got, want := string(body), `{"error":"invalid cursor parameter","code":"invalid_cursor"}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
