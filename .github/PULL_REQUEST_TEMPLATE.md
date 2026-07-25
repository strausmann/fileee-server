## Beschreibung

Was ändert dieser PR und warum?

## Bezug

Refs #… <!-- oder: Closes #… -->

## Checkliste

- [ ] **TDD strict eingehalten** — Test zuerst geschrieben, dann Implementierung.
- [ ] Mutations-Pfade (Handler mit POST/PUT/DELETE, insbesondere die Destruktiv-Gate-Routen)
      decken Happy-Path, Error-Path (4xx/5xx) und Network-Error/Timeout ab, falls zutreffend.
- [ ] `go build ./...`, `go vet ./...` und `go test ./... -race` laufen lokal grün.
- [ ] `./scripts/coverage-gate-strict.sh` besteht für geänderte Dateien (Schwellen aus
      `.github/workflows/test.yml`).
- [ ] `./scripts/doc-coverage.sh` meldet 0 undokumentierte Exports.
- [ ] **API-Änderungen dokumentiert** — Routen-Tabellen in `README.md` nachgezogen, Request-/
      Response-Structs mit korrekten Huma-Tags (OpenAPI generiert sich daraus automatisch).
- [ ] Neues ADR angelegt und in `docs/adr/README.md` registriert, falls eine Architektur-/
      Technologie-Entscheidung enthalten ist.
- [ ] Commit-Messages sind Conventional-Commits-konform (Kleinbuchstaben-Subject, gültiger
      Scope aus `.commitlintrc.json`).
- [ ] Keine Secrets/API-Token/Credentials im Klartext in Code, Tests oder Compose-Dateien.

## Testnachweis

Wie wurde die Änderung verifiziert (Testlauf-Output, manueller Request gegen `/v1/...`)?
