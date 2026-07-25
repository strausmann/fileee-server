package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// fakeSessionChecker implementiert fileeeSessionChecker als Test-Double von *fileee.Client:
// EnsureSessionFunc/UserIDFunc sind injizierbar, ensureSessionCalled/userIDCalled dokumentieren,
// OB die jeweilige Methode aufgerufen wurde (z. B. um zu prüfen, dass UserID NICHT aufgerufen
// wird, wenn EnsureSession bereits fehlschlägt).
type fakeSessionChecker struct {
	ensureSessionErr  error
	userID            string
	userIDErr         error
	ensureSessionCall bool
	userIDCall        bool
}

func (f *fakeSessionChecker) EnsureSession(ctx context.Context) error {
	f.ensureSessionCall = true
	return f.ensureSessionErr
}

func (f *fakeSessionChecker) UserID(ctx context.Context) (string, error) {
	f.userIDCall = true
	return f.userID, f.userIDErr
}

// TestRunBootSelfcheck_DisabledIsNoOp prüft den Default-Zustand (FILEEE_BOOT_SELFCHECK=false):
// weder EnsureSession noch UserID werden aufgerufen — der Boot-Selbsttest ist standardmäßig aus,
// um das reverse-engineered Fileee nicht mit zusätzlichem Boot-Traffic zu belasten.
func TestRunBootSelfcheck_DisabledIsNoOp(t *testing.T) {
	checker := &fakeSessionChecker{}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	runBootSelfcheck(context.Background(), false, checker, log)

	if checker.ensureSessionCall || checker.userIDCall {
		t.Fatal("bei enabled=false dürfen weder EnsureSession noch UserID aufgerufen werden")
	}
	if buf.Len() != 0 {
		t.Errorf("bei enabled=false erwartet kein Log, war:\n%s", buf.String())
	}
}

// TestRunBootSelfcheck_HappyPath prüft den Erfolgsfall: beide Aufrufe gelingen, ein
// Erfolgs-Log wird geschrieben, und die User-ID selbst (PII) taucht NICHT im Log auf.
func TestRunBootSelfcheck_HappyPath(t *testing.T) {
	checker := &fakeSessionChecker{userID: "user-secret-id-42"}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	runBootSelfcheck(context.Background(), true, checker, log)

	if !checker.ensureSessionCall || !checker.userIDCall {
		t.Fatal("bei enabled=true müssen EnsureSession UND UserID aufgerufen werden")
	}
	out := buf.String()
	if !strings.Contains(out, "boot-selfcheck erfolgreich") {
		t.Errorf("erwartet Erfolgs-Log, war:\n%s", out)
	}
	if strings.Contains(out, "user-secret-id-42") {
		t.Errorf("Log darf die User-ID (PII) nicht enthalten:\n%s", out)
	}
}

// TestRunBootSelfcheck_EnsureSessionError prüft, dass ein EnsureSession-Fehler geloggt wird UND
// UserID danach NICHT mehr aufgerufen wird (kein sinnloser zweiter Call auf eine Session, die
// bereits als ungültig erkannt wurde).
func TestRunBootSelfcheck_EnsureSessionError(t *testing.T) {
	wantErr := errors.New("network unreachable")
	checker := &fakeSessionChecker{ensureSessionErr: wantErr}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	runBootSelfcheck(context.Background(), true, checker, log)

	if !checker.ensureSessionCall {
		t.Fatal("EnsureSession muss aufgerufen werden")
	}
	if checker.userIDCall {
		t.Fatal("UserID darf nach EnsureSession-Fehler nicht aufgerufen werden")
	}
	if !strings.Contains(buf.String(), "boot-selfcheck fehlgeschlagen") {
		t.Errorf("erwartet Fehler-Log, war:\n%s", buf.String())
	}
}

// TestRunBootSelfcheck_UserIDError prüft, dass ein UserID-Fehler (EnsureSession erfolgreich,
// aber die anschließende Leseoperation schlägt fehl) ebenfalls als Fehlschlag geloggt wird.
func TestRunBootSelfcheck_UserIDError(t *testing.T) {
	wantErr := errors.New("unexpected response")
	checker := &fakeSessionChecker{userIDErr: wantErr}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	runBootSelfcheck(context.Background(), true, checker, log)

	if !checker.ensureSessionCall || !checker.userIDCall {
		t.Fatal("beide Aufrufe müssen stattfinden")
	}
	if !strings.Contains(buf.String(), "boot-selfcheck fehlgeschlagen") {
		t.Errorf("erwartet Fehler-Log, war:\n%s", buf.String())
	}
}
