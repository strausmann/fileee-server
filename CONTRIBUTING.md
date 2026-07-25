# Beitragen zu fileee-server

Danke für dein Interesse an diesem Projekt. `fileee-server` ist ein dünner REST-API-Wrapper um
die [`go-fileee`](https://github.com/strausmann/go-fileee)-Core-Lib — er enthält selbst
**keinen** Fileee-Protokoll-Code. Diese Datei beschreibt, wie ein Beitrag (Issue, Pull Request)
reibungslos durch die Qualitäts-Gates dieses Repos kommt.

## Bevor du anfängst

- **ADRs lesen:** Architektur-Entscheidungen zu diesem Server stehen unter
  [`docs/adr/`](docs/adr/) (Nummerierung ab `0008`, siehe
  [`docs/adr/README.md`](docs/adr/README.md)). Die grundlegenden Entscheidungen zur Core-Lib
  (Library-first, Auth-Modell, Rate-Limiting, Domänen-Neutralität, Ausschluss destruktiver
  Lib-Operationen) leben weiterhin im
  [go-fileee-Repo](https://github.com/strausmann/go-fileee/tree/main/docs/adr) und werden hier
  **nicht** dupliziert.
- Dieser Server delegiert jede Fileee-Operation 1:1 an einen bereits eingerichteten
  `*fileee.Client`/`*fileee.ShareClient` aus `go-fileee` — neue Fileee-Protokoll-Logik gehört
  **nicht** hierher, sondern ins go-fileee-Repo.
- Für größere Änderungen erst ein Issue eröffnen und die Richtung abstimmen, bevor viel Code
  geschrieben wird.

## Entwicklungs-Workflow

1. Fork oder Branch anlegen (kein Direkt-Push auf `main`).
2. Änderungen **strikt TDD** umsetzen: zuerst einen fehlschlagenden Test schreiben, dann die
   Implementierung, bis der Test grün ist.
3. Mutations-Pfade (POST/PUT/DELETE-Handler, insbesondere die drei Hard-DELETE-Routen hinter dem
   Destruktiv-Gate `FILEEE_ALLOW_DESTRUCTIVE`) decken mindestens ab: Happy-Path, Error-Path
   (4xx/5xx von go-fileee) und Network-Error/Timeout.
4. Lokal vor dem Commit prüfen:
   ```bash
   go build ./...
   go vet ./...
   go test ./... -race -coverprofile=cover.out
   ./scripts/coverage-gate-strict.sh cover.out cmd/fileee-server/<geänderte-datei>.go:<schwelle>
   ./scripts/doc-coverage.sh
   ```
   Die Coverage-Schwellen pro Datei stehen in
   [`.github/workflows/test.yml`](.github/workflows/test.yml) — sie sind ein **Floor** (an den
   gemessenen Ist-Stand angelehnt), keine aspirationalen Zielwerte. Wird eine bestehende Datei
   geändert, darf ihre Coverage nicht unter die dort hinterlegte Schwelle fallen.
5. **API-Änderungen dokumentieren:** Neue/geänderte Routen, Request-/Response-Structs oder
   Auth-Verhalten im selben PR in `README.md` (Routen-Tabellen) nachziehen — die OpenAPI-Spec
   (`/openapi.json`/`/openapi.yaml`) generiert Huma automatisch aus den Struct-Tags, muss also
   nicht separat gepflegt werden, aber die Struct-Tags selbst müssen stimmen.
6. Falls die Änderung eine Architektur- oder Technologie-Entscheidung enthält: ADR unter
   `docs/adr/` anlegen (nächste freie Nummer nach `0008`) und in
   [`docs/adr/README.md`](docs/adr/README.md) registrieren.

## Commit-Konvention

Commit-Messages folgen [Conventional Commits](https://www.conventionalcommits.org/) und werden
per Husky-Hook + [commitlint](https://commitlint.js.org/) geprüft
(`.commitlintrc.json`, `@commitlint/config-conventional`):

- **Typ:** `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `ci:` — auf Deutsch
  formuliert (z. B. `fix(handlers-documents): cursor-decodierung bei leerem query-param`).
- **Subject in Kleinbuchstaben** (`subject-case`-Regel aus `config-conventional`) — kein
  großgeschriebener Satzanfang.
- **Scope** aus der festen Liste in `.commitlintrc.json` (`server`, `config`, `secrets`,
  `handlers`, `share-proxy`, `resolve`, `watch`, `deploy`, `adr`, `ci`, `deps`, `docs`,
  `release`) — kein neuer Scope ohne Anpassung der Datei.
- **Issue-Referenz:** `Refs #N` oder `Closes #N` in Commit oder PR-Beschreibung.

Der `commit-msg`-Hook läuft automatisch nach `npm install` (installiert Husky via
`"prepare": "husky"` in `package.json`). Committen ohne `npm install` vorher überspringt den
lokalen Hook — der `commitlint`-PR-Check in CI greift trotzdem als Gate.

**Nie `git commit --no-verify` verwenden**, um den Hook zu umgehen.

## Pull Requests

- Ziel-Branch ist `main`. `main` ist geschützt — kein Direkt-Push, nur PRs mit grüner CI.
- CI-Gates, die grün sein müssen:
  - `test.yml` — `go build`, `go vet`, `go test ./... -race`, Coverage-Gate
    (`scripts/coverage-gate-strict.sh`), Doc-Coverage (`scripts/doc-coverage.sh`)
  - `commitlint.yml` — jeder Commit im PR muss der Konvention entsprechen
- PR-Beschreibung nutzt die [Pull-Request-Vorlage](.github/PULL_REQUEST_TEMPLATE.md) — Issue-Bezug,
  Testnachweis und Doku-Sync-Checkbox nicht auslassen.
- Kleine, fokussierte PRs bevorzugt gegenüber großen Sammel-PRs.

## Fragen oder Bugs melden

- **Bug:** [Bug-Report-Vorlage](.github/ISSUE_TEMPLATE/bug_report.md) nutzen.
- **Feature-Wunsch:** [Feature-Request-Vorlage](.github/ISSUE_TEMPLATE/feature_request.md)
  nutzen — bei Wunsch nach neuer Fileee-Protokoll-Abdeckung bitte beachten, dass diese Logik ins
  [go-fileee-Repo](https://github.com/strausmann/go-fileee) gehört, nicht hierher.

## Sicherheit

Das statische API-Token (`FILEEE_SERVER_API_TOKEN` o. ä.) und alle Fileee-/Infisical-Credentials
gehören niemals in Code, Tests, Issues oder Compose-Dateien im Klartext — Platzhalter (`CHANGE_ME`)
in Vorlagen, echte Werte ausschließlich in `.env`/Secret-Backends. Sicherheitsrelevante Funde
bitte nicht als öffentliches Issue melden, sondern den Maintainer direkt kontaktieren.
