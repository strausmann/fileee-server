package main

import (
	"context"
	"log/slog"

	"github.com/strausmann/go-fileee/fileee"
)

// fileeeSessionChecker ist die schmale Schnittstelle, die runBootSelfcheck von seinem client
// braucht — erfüllt strukturell von *fileee.Client (siehe Compile-Time-Assertion unten), und in
// Tests von fakeSessionChecker (selfcheck_test.go) ohne echten Fileee-Account/-Netzwerkzugriff.
type fileeeSessionChecker interface {
	EnsureSession(ctx context.Context) error
	UserID(ctx context.Context) (string, error)
}

// Compile-Time-Assertion: *fileee.Client muss fileeeSessionChecker weiterhin erfüllen. Bricht
// absichtlich den Build, falls sich die Core-Lib-Signaturen von EnsureSession/UserID künftig
// ändern — dann sofort sichtbar hier, statt erst zur Laufzeit in main().
var _ fileeeSessionChecker = (*fileee.Client)(nil)

// runBootSelfcheck führt — wenn enabled (FILEEE_BOOT_SELFCHECK, Default false, siehe config.go)
// — GENAU einen leichten Lese-Roundtrip gegen Fileee aus: EnsureSession (stellt eine gültige
// Session sicher, re-authentifiziert bei Bedarf) gefolgt von UserID (liest die eigene
// Fileee-User-ID der Session, siehe fileee/auth.go — kein zusätzlicher Netzwerk-Request nötig,
// wenn die Session bereits userId-Cookie/JWT trägt). Ist enabled false, ist der Aufruf ein
// reines No-Op (kein Client-Call, kein Log) — der Boot-Selbsttest bleibt standardmäßig aus, um
// das reverse-engineered, undokumentierte Fileee-API nicht mit zusätzlichem Boot-Traffic zu
// belasten (siehe fileee-Skill).
//
// Ergebnis wird als reines Erfolg/Fehlschlag geloggt — NIEMALS die tatsächliche User-ID (PII)
// oder irgendein Secret-Wert, nur ob der Roundtrip geklappt hat und im Fehlerfall die
// Fehlermeldung des jeweiligen Aufrufs.
func runBootSelfcheck(ctx context.Context, enabled bool, client fileeeSessionChecker, log *slog.Logger) {
	if !enabled {
		return
	}

	if err := client.EnsureSession(ctx); err != nil {
		log.Warn("boot-selfcheck fehlgeschlagen (EnsureSession)", "error", err)
		return
	}

	if _, err := client.UserID(ctx); err != nil {
		log.Warn("boot-selfcheck fehlgeschlagen (UserID-Abfrage)", "error", err)
		return
	}

	log.Info("boot-selfcheck erfolgreich")
}
