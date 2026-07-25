# fileee-server

[![CI](https://github.com/strausmann/fileee-server/actions/workflows/test.yml/badge.svg)](https://github.com/strausmann/fileee-server/actions/workflows/test.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/strausmann/fileee-server)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> **Baut auf [`github.com/strausmann/go-fileee`](https://github.com/strausmann/go-fileee).**
> `fileee-server` ist der selbst gehostete REST-API-Service rund um die
> [go-fileee](https://github.com/strausmann/go-fileee) Core-Lib — er konsumiert sie als normale,
> versionierte Go-Modul-Abhängigkeit (`github.com/strausmann/go-fileee`, aktuell `v0.1.0`) und
> enthält selbst **keinen** Fileee-Protokoll-Code. Dieses Repo entstand per Split aus `go-fileee`
> (siehe [ADR-0008](docs/adr/0008-fileee-server.md)); die grundlegenden Architektur-Entscheidungen
> zur Core-Lib (Library-first, Auth-Modell, Rate-Limiting, Domänen-Neutralität, Ausschluss
> destruktiver Lib-Operationen) leben weiterhin im
> [go-fileee-Repo](https://github.com/strausmann/go-fileee) unter `docs/adr/`.

`fileee-server` exponiert die Core-Lib hinter einem statischen API-Token als REST-API —
gedacht für **N8N-Workflows und CI-Automatisierung**, die Fileee ansprechen sollen, ohne selbst
einen Fileee-Login (Username/Passwort/TOTP) zu kennen. Die vom Server exponierte `/v1/...`-
Oberfläche ist **stabil und OpenAPI-3.1-dokumentiert** (`GET /openapi.json`, interaktive Docs
unter `GET /docs`) — im Gegensatz zur Core-Lib, deren Methoden sich mit dem internen,
reverse-engineerten Fileee-API jederzeit ändern können.

> **Status:** In Entwicklung — privat. Noch keine stabile Version, kein `v1.0.0`-Release.

## Quickstart

```bash
# Einmalig: Session-Volume für den nonroot-User des Containers vorbereiten (uid 65532,
# distroless-Image ohne Shell — siehe Kommentar in deploy/compose.plain.yaml).
sudo mkdir -p <host-pfad-fuer-session> && sudo chown 65532:65532 <host-pfad-fuer-session>

# .env / Compose-Platzhalter ("CHANGE_ME") mit echten Werten befüllen, dann:
docker compose -f deploy/compose.plain.yaml up
```

`deploy/compose.plain.yaml` ist ein **Referenz-Template** (echtes GitOps-Deployment folgt in
`infrastructure/docker/fileee-server`, siehe `.claude/rules/infrastructure-as-code-governance.md`
im homelab-management-Repo). Zwei Konfigurationsmodi stehen zur Wahl:

- **ENV-Modus** (`SECRET_BACKEND=env`, Default): alle Werte — inklusive Secrets — kommen direkt aus
  den unten dokumentierten `FILEEE_*`-Umgebungsvariablen.
- **Infisical-Dual-Mode** (`SECRET_BACKEND=infisical`): die Binary mintet beim Start selbst ein
  Infisical-Token (`infisical login --method=universal-auth`), exportiert die Secrets
  (`infisical export --format=dotenv`), merged sie in die Prozessumgebung und ersetzt sich per
  `syscall.Exec` durch sich selbst (`fileee-server` wird dabei PID 1 — Signal-Forwarding ist damit
  gegenstandslos). In diesem Modus entfallen die `FILEEE_USERNAME`/`FILEEE_PASSWORD`/
  `FILEEE_API_TOKEN`/`FILEEE_TOTP_SEED`-ENV-Variablen; sie werden stattdessen aus Infisical
  bezogen. Beispiel-Umschaltung: siehe auskommentierten Block in `deploy/compose.plain.yaml`.

Drei Compose-Referenz-Templates liegen unter [`deploy/`](deploy/):

| Datei | Szenario |
|---|---|
| `deploy/compose.plain.yaml` | Ohne Reverse Proxy — direkt per Port oder im internen Docker-Netz erreichbar. |
| `deploy/compose.pangolin.yaml` | Öffentlich über Pangolin, **bewusst ohne SSO** (reiner Maschinen-Endpunkt, kein Browser-UI). |
| `deploy/compose.traefik.yaml` | Hinter Traefik als Reverse Proxy. |

## Installation aus Quellcode

```bash
git clone https://github.com/strausmann/fileee-server.git
cd fileee-server
go build ./cmd/fileee-server
```

Voraussetzung: Go 1.23 oder neuer. Die Abhängigkeit auf die Core-Lib
(`github.com/strausmann/go-fileee v0.1.0`) wird über `go.mod` aufgelöst — kein lokaler Checkout
von `go-fileee` nötig (Ausnahme: Co-Entwicklung an beiden Repos gleichzeitig, siehe
[„Lokale Co-Entwicklung mit go-fileee"](#lokale-co-entwicklung-mit-go-fileee) unten).

## Konfiguration (Umgebungsvariablen)

Alle Werte werden ausschließlich über `LoadConfig` (`cmd/fileee-server/config.go`) gelesen — kein
Feld wird an anderer Stelle direkt aus `os.Getenv` bezogen.

| Variable | Zweck | Default | Pflicht | Secret |
|---|---|---|---|---|
| `FILEEE_USERNAME` | Fileee-Login-Benutzername | – | Ja | Ja |
| `FILEEE_PASSWORD` | Fileee-Login-Passwort | – | Ja | Ja |
| `FILEEE_TOTP_SEED` | Base32-TOTP-Seed für Zwei-Faktor-Konten | leer | Nein (nur bei 2FA-Konten) | Ja |
| `FILEEE_API_TOKEN` | Statisches Bearer-Token, mit dem sich Clients gegen den Server authentifizieren (`X-API-Key`- oder `Bearer`-Header) | – | Ja | Ja |
| `FILEEE_ALLOW_DESTRUCTIVE` | Schaltet die drei Hard-DELETE-Routen frei (siehe Destruktiv-Gate unten) | `false` | Nein | Nein |
| `FILEEE_LISTEN_ADDR` | Adresse, auf der der HTTP-Server lauscht | `:8080` | Nein | Nein |
| `FILEEE_SESSION_PATH` | Pfad, unter dem die Fileee-Session persistiert wird | `/home/nonroot/session.json` | Nein | Nein (Dateiinhalt ist sensibel, Rechte `0600`) |
| `FILEEE_KEEPALIVE_INTERVAL` | Intervall des Session-Keepalive | `15m` | Nein | Nein |
| `FILEEE_WAIT_TIMEOUT` | Default-Wartezeit von `POST /v1/processes/{id}/wait`, falls kein `?timeout=` mitgeschickt wird | `60s` | Nein | Nein |
| `FILEEE_WAIT_MAX` | Obergrenze, auf die jedes angeforderte Wait-Timeout gedeckelt wird | `300s` | Nein | Nein |
| `FILEEE_RATE_RPS` | Erlaubte Request-Rate/Sekunde gegen die Fileee-API | `1` | Nein | Nein |
| `FILEEE_RATE_BURST` | Burst-Größe des Token-Buckets | `3` | Nein | Nein |
| `FILEEE_TRUSTED_PROXIES` | Kommagetrennte IPs/CIDRs vertrauenswürdiger Reverse-Proxies (Access-Log/Client-IP-Ermittlung) | leer | Nein | Nein |
| `FILEEE_CLIENT_IP_HEADERS` | Kommagetrennte Header-Prüfreihenfolge zur Client-IP-Ermittlung | `CF-Connecting-IP,X-Real-IP,X-Forwarded-For` | Nein | Nein |
| `FILEEE_DOCS_PUBLIC` | Ob `/docs` (Doku-UI) ohne API-Token erreichbar ist | `true` | Nein | Nein |
| `FILEEE_MAX_UPLOAD_SIZE` | Max. Body-Größe von `POST /v1/documents` in Bytes | `33554432` (32 MiB) | Nein | Nein |
| `FILEEE_WEBHOOK_URL` | Ziel-URL für ausgehende Webhook-Benachrichtigungen | leer (Webhooks deaktiviert) | Nein | Nein |
| `FILEEE_WEBHOOK_SECRET` | Signiert ausgehende Webhook-Payloads | leer | Nein | Ja |
| `FILEEE_WATCH_INTERVAL` | Polling-Intervall des Änderungs-Watchers | `0` (Watcher deaktiviert) | Nein | Nein |
| `FILEEE_USER_AGENT` | Überschreibt den User-Agent gegen Fileee | leer (Core-Lib-Default) | Nein | Nein |
| `FILEEE_LOG_LEVEL` | Log-Level des strukturierten Loggers (`slog`) | `info` | Nein | Nein |

**Secret-Backend / Infisical-Dual-Mode** (`cmd/fileee-server/secrets.go`, optional — nur relevant, wenn `SECRET_BACKEND=infisical` gesetzt ist oder eine Universal-Auth-Client-ID vorliegt):

| Variable | Zweck | Default | Pflicht (im Infisical-Modus) | Secret |
|---|---|---|---|---|
| `SECRET_BACKEND` | `env` (Default) oder `infisical` | `env` | Nein | Nein |
| `INFISICAL_UNIVERSAL_AUTH_CLIENT_ID` | Machine-Identity Client-ID | – | Ja | Ja |
| `INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET` | Machine-Identity Client-Secret | – | Ja | Ja |
| `INFISICAL_DOMAIN` | Self-hosted Infisical-URL, **mit** `/api` (z. B. `https://secretsmanager.strausmann.cloud/api`) | – | Ja | Nein |
| `INFISICAL_PROJECT_ID` | Ziel-Projekt-ID | – | Ja | Nein |
| `INFISICAL_ENV` | Ziel-Environment (`dev`/`staging`/`prod`) | – | Ja (CLI-Default wäre sonst `dev` — für `prod` fatal, siehe `.claude/rules/secret-environment-awareness.md` im homelab-management-Repo) | Nein |
| `INFISICAL_PATH` | Secret-Pfad/Folder innerhalb des Projekts | `/` | Nein | Nein |

## Endpunkt-Übersicht

Alle Routen liegen unter `/v1/...` (Ausnahme `/healthz`). Vollständige, maschinenlesbare
Beschreibung: OpenAPI 3.1 unter `/openapi.json`/`/openapi.yaml`, interaktive Docs unter `/docs`
(`FILEEE_DOCS_PUBLIC` steuert, ob `/docs` ohne Token erreichbar ist).

| Gruppe | Routen |
|---|---|
| **Dokumente/Seiten** (Read, PDF-/Bild-Streams, OCR) | `GET /v1/documents` (Liste/Volltextsuche), `GET /v1/documents/{id}`, `GET /v1/documents/{id}/pdf`, `GET /v1/pages/{pageId}/image`, `GET /v1/pages/{pageId}/ocr` |
| **Stammdaten** (Tags/Companies/Contacts/Document-Types/Schemes/Reminders/Boxes) | `GET /v1/tags`, `GET /v1/companies`, `GET /v1/contacts`, `GET /v1/document-types`, `GET /v1/document-type-schemes`, `GET /v1/reminders`, `GET /v1/boxes`, `GET /v1/boxes/{id}` |
| **Write** (Upload/Update/Share/Unshare/Box/Reminders/Contacts/Export-ZIP/Processes/Wait) | `POST /v1/documents` (Upload, multipart), `PUT /v1/documents/{id}`, `POST /v1/share`, `POST /v1/documents/{id}/unshare`, `POST` bzw. `DELETE /v1/boxes/{boxId}/documents/{docId}` (Einheften/Aushängen, kein Destruktiv-Gate), `POST /v1/reminders`, `PUT /v1/reminders/{id}`, `POST /v1/contacts`, `PUT /v1/contacts/{id}`, `POST /v1/documents/export-zip`, `GET /v1/processes/{id}` (Poll), `POST /v1/processes/{id}/wait` (blockierend, auf `FILEEE_WAIT_MAX` gedeckelt) |
| **Share-Proxy** (anonym, ohne Fileee-Login, `/v1/share-objects/...`) | `POST /v1/share-objects/{token}` (auflösen), `GET /v1/share-objects/{token}/pages/{pageId}/image`, `GET /v1/share-objects/{token}/pages/{pageId}/ocr`, `GET /v1/share-objects/{token}/documents/{docId}/pdf` |
| **Resolver** (ein Link rein, ein einheitliches Dokument raus) | `POST /v1/resolve {url}` — erkennt intern vs. anonym per `?include=ocr` |
| **Konversationen** (Chat, Teilnehmer, Einladungen) | `GET /v1/conversations`, `GET /v1/conversations/{id}`, `GET /v1/documents/{id}/conversations`, `POST /v1/conversations/{id}/messages`, `POST`/`DELETE /v1/conversations/{id}/documents/{docId}` (kein Destruktiv-Gate), `POST /v1/conversations/{id}/participants`, `DELETE /v1/conversations/{id}/participants/{participantId}`, `GET /v1/conversations/invitations`, `POST /v1/conversations/invitations/accept/{token}` (Annahme-Pfad bewusst `.../accept/{token}`, nicht `.../{token}/accept` — vermeidet einen Go-`ServeMux`-Pattern-Konflikt mit der Dokument-Teilen-Route) |
| **Destruktiv (Hard-DELETE)** | `DELETE /v1/documents/{id}`, `DELETE /v1/contacts/{id}`, `DELETE /v1/reminders/{id}` |
| **Sonstiges** | `GET /healthz` (Liveness, kein Auth nötig, kein Fileee-Roundtrip) |

### Destruktiv-Gate

Die drei Hard-DELETE-Routen (`DELETE /v1/documents/{id}`, `DELETE /v1/contacts/{id}`,
`DELETE /v1/reminders/{id}`) werden nur registriert, wenn `FILEEE_ALLOW_DESTRUCTIVE=true` gesetzt
ist. Bleibt das Flag `false`, ist der DELETE-Pfad dem Server für das DELETE-Verb komplett
unbekannt — da GET/PUT auf denselben Pfaden weiterhin registriert sind, antwortet der Server dann
mit **405 Method Not Allowed** statt 404. Jede tatsächlich ausgeführte Destruktiv-Operation wird
zusätzlich vor dem Löschversuch als Audit-Log-Zeile protokolliert. Das Aushängen aus einer Box
(`DELETE /v1/boxes/{boxId}/documents/{docId}`) und das Entfernen aus einer Konversation
(`DELETE /v1/conversations/{id}/documents/{docId}`) fallen **nicht** unter dieses Gate — beides
löscht kein Dokument, sondern nimmt nur eine Zuordnung zurück. Hintergrund und Abwägung:
[ADR-0008](docs/adr/0008-fileee-server.md) sowie das nur in `go-fileee` liegende
[ADR-0007](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0007-ausschluss-destruktiver-operationen.md)
(Ausschluss destruktiver Lib-Operationen — durch ADR-0008 für den Server verfeinert, nicht
abgelöst).

### Auth

Jeder Request (außer `/healthz`, `/openapi.json`, `/openapi.yaml` sowie `/docs` bei
`FILEEE_DOCS_PUBLIC=true`) braucht das statische `FILEEE_API_TOKEN` als `X-API-Key`- oder
`Bearer`-Header. Details zum Betrieb hinter Pangolin (bewusst ohne SSO) siehe
`deploy/compose.pangolin.yaml`.

### Interaktive Doku (`/docs`)

Der Server generiert seine OpenAPI-3.1-Spezifikation zur Laufzeit aus den getippten
Request-/Response-Strukturen ([Huma](https://huma.rocks/)) und stellt dazu eine self-contained
Docs-UI unter `GET /docs` bereit (kein CDN, passt zu CSP-freiem Betrieb). Maschinenlesbar:
`GET /openapi.json` / `GET /openapi.yaml`. Ob `/docs` ohne API-Token erreichbar ist, steuert
`FILEEE_DOCS_PUBLIC` (Default `true`).

## Sicherheit

- **Credentials** (Fileee-Username/-Passwort/-TOTP-Seed, `FILEEE_API_TOKEN`, Infisical-Machine-Identity) gehören **ausschließlich** in einen Secret-Manager (Vaultwarden/Infisical) — niemals in Code, Fixtures oder Commits.
- Die **Session-Datei** (`FILEEE_SESSION_PATH`) ist ein Secret (Dateirechte `0600`), wird nie geloggt oder committed.
- Der Server läuft als **rootless, distroless** Container (`gcr.io/distroless/static-debian12:nonroot`, `uid 65532`, statisches Binary, `CGO_ENABLED=0`) — keine Shell, kein Paketmanager im Laufzeit-Image.
- Zugriffs-Logs im NGINX-`combined`-Format auf stdout erlauben CrowdSec, den vorhandenen `crowdsecurity/nginx`-Parser (`http-probing`, `http-bruteforce`, `http-crawl-non-statics`) ohne Custom-Parser zu nutzen. Die Client-IP wird nur aus Reverse-Proxy-Headern übernommen, wenn die TCP-Quelle in `FILEEE_TRUSTED_PROXIES` liegt.
- Details zur schonenden Fileee-Nutzung (Rate-Limiting/Backoff, geteilt über alle Handler): [go-fileee ADR-0005](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0005-schonender-betrieb-rate-limiting.md).

## Lokale Co-Entwicklung mit go-fileee

Für Änderungen, die **beide** Repos gleichzeitig betreffen (z. B. eine neue Lib-Methode vor ihrem
`go-fileee`-Release nutzen), lokal ein Go-Workspace anlegen — **niemals committen**:

```bash
go work init && go work use . ../go-fileee
```

`go.work`/`go.work.sum` sind in `.gitignore` gelistet. Das committete `go.mod` bleibt immer auf
einen echten, veröffentlichten `go-fileee`-Tag gepinnt (aktuell `v0.1.0`), **ohne** `replace`-
Direktive.

## Dokumentation

- [`docs/adr/`](docs/adr/) — Architecture Decision Records dieses Repos (ADR-0008 und Folgende); die grundlegenden Lib-ADRs 0001–0007 liegen im [go-fileee-Repo](https://github.com/strausmann/go-fileee/tree/main/docs/adr)
- [go-fileee `docs/API.md`](https://github.com/strausmann/go-fileee/blob/main/docs/API.md) — vollständige Fileee-API-Referenz (Endpunkte, Auth-Ablauf, Datenmodell der Core-Lib)
- [go-fileee auf pkg.go.dev](https://pkg.go.dev/github.com/strausmann/go-fileee/fileee) — Core-Lib-Methodenreferenz

## Disclaimer

`fileee-server` exponiert Funktionalität, die letztlich auf einer Rekonstruktion des internen
Protokolls der Fileee-Web-App (`my.fileee.com`) beruht — siehe
[go-fileee](https://github.com/strausmann/go-fileee) für den vollständigen Disclaimer. Fileee kann
das interne API jederzeit ohne Ankündigung ändern; dieser Server kann dadurch brechen. Nutzung
ausschließlich für das eigene Fileee-Konto vorgesehen, keine Gewähr für Vollständigkeit,
Korrektheit oder Dauerhaftigkeit der Funktionalität.

## Lizenz

[MIT](LICENSE) — Copyright © 2026 Björn Strausmann
