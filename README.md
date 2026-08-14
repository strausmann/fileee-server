# fileee-server

[![CI](https://github.com/strausmann/fileee-server/actions/workflows/test.yml/badge.svg)](https://github.com/strausmann/fileee-server/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/strausmann/fileee-server)](https://goreportcard.com/report/github.com/strausmann/fileee-server)
[![Release](https://img.shields.io/github/v/release/strausmann/fileee-server)](https://github.com/strausmann/fileee-server/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/strausmann/fileee-server)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> **Baut auf [`github.com/strausmann/go-fileee`](https://github.com/strausmann/go-fileee).**
> `fileee-server` ist der selbst gehostete REST-API-Service rund um die
> [go-fileee](https://github.com/strausmann/go-fileee) Core-Lib — er konsumiert sie als normale,
> versionierte Go-Modul-Abhängigkeit (`github.com/strausmann/go-fileee`, aktuell `v0.2.0`) und
> enthält selbst **keinen** Fileee-Protokoll-Code. Dieses Repo entstand per Split aus `go-fileee`
> (siehe [ADR-0008](docs/adr/0008-fileee-server.md)); die grundlegenden Architektur-Entscheidungen
> zur Core-Lib (Library-first, Auth-Modell, Rate-Limiting, Domänen-Neutralität, Ausschluss
> destruktiver Lib-Operationen) leben weiterhin im
> [go-fileee-Repo](https://github.com/strausmann/go-fileee) unter `docs/adr/`.

## Zweck und Architektur

`fileee-server` ist ein **dünner REST-API-Wrapper** um die `go-fileee`-Core-Lib: ein einzelnes
Go-Binary (Package `main`, `cmd/fileee-server/`), das die Lib-Methoden hinter einem statischen
API-Token als HTTP-API exponiert — gedacht für **N8N-Workflows und CI-Automatisierung**, die
Fileee ansprechen sollen, ohne selbst einen Fileee-Login (Username/Passwort/TOTP) zu kennen. Der
Server selbst enthält keine eigene Geschäftslogik gegen `my.fileee.com` — jede Fileee-Operation
delegiert 1:1 an einen bereits eingerichteten `*fileee.Client` bzw. `*fileee.ShareClient` aus
go-fileee.

- **Framework:** [Huma v2](https://huma.rocks/) (`v2.35.0`) über den `humago`-Adapter auf
  `http.ServeMux` — Request-/Response-Strukturen sind getippte Go-Structs, aus denen Huma zur
  Laufzeit automatisch eine OpenAPI-3.1-Spezifikation generiert (`GET /openapi.json`,
  `GET /openapi.yaml`) samt interaktiver, self-contained Docs-UI (`GET /docs`, kein CDN).
- **Laufzeit-Image:** rootless, distroless (`gcr.io/distroless/static-debian12:nonroot`,
  `uid 65532`), statisches Binary (`CGO_ENABLED=0`) — kein Shell, kein Paketmanager im
  Runtime-Image. Der Container-`HEALTHCHECK` läuft deshalb bewusst nicht per `sh -c curl ...`,
  sondern über ein eigenes Subcommand der Binary selbst (`fileee-server healthcheck`).
- **Infisical-Dual-Mode (optional, kein separater Entrypoint-Wrapper):** Ist
  `SECRET_BACKEND=infisical` gesetzt (oder eine Universal-Auth-Client-ID konfiguriert), mintet die
  Binary beim Start selbst ein Infisical-Token über die im Image mitgelieferte, statische
  `infisical`-CLI (`/infisical`), exportiert die Secrets als Dotenv, merged sie in die
  Prozessumgebung und ersetzt sich per `syscall.Exec` durch sich selbst — der Server wird dabei
  PID 1. Kein `ENTRYPOINT ["infisical", "run", "--", ...]`-Wrapper: `MaybeInjectInfisical`
  (`cmd/fileee-server/secrets.go`) übernimmt das innerhalb von `main()`, bevor die Konfiguration
  geladen wird.
- **Go-Version:** `go 1.25.0` laut `go.mod` (beide Repos, `go-fileee` und `fileee-server`).
- **Sichtbarkeit:** Das Repo ist **öffentlich** auf GitHub
  (`https://github.com/strausmann/fileee-server`), ebenso `go-fileee`. Es gibt (noch) **keinen
  `v1.0.0`-Release** — der aktuellste Tag ist `v0.1.1`.

Die vom Server exponierte `/v1/...`-Oberfläche ist **stabil und OpenAPI-3.1-dokumentiert** — im
Gegensatz zur Core-Lib, deren Methoden sich mit dem internen, reverse-engineerten Fileee-API
jederzeit ändern können.

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

Voraussetzung: **Go 1.25 oder neuer**. Die Abhängigkeit auf die Core-Lib
(`github.com/strausmann/go-fileee v0.2.0`) wird über `go.mod` aufgelöst — kein lokaler Checkout
von `go-fileee` nötig (Ausnahme: Co-Entwicklung an beiden Repos gleichzeitig, siehe
[„Lokale Co-Entwicklung mit go-fileee"](#lokale-co-entwicklung-mit-go-fileee) unten).

## Zwei Auth-Schichten (nicht verwechseln)

`fileee-server` hat **zwei vollständig getrennte** Authentifizierungs-Schichten. Das ist die
häufigste Quelle für Verwirrung — deshalb hier explizit beide gegenübergestellt:

| | Clients → fileee-server | fileee-server → Fileee |
|---|---|---|
| **Was** | Statisches Bearer-Token `FILEEE_API_TOKEN` | Cookie-Session + CSRF-Header + TOTP (go-fileee) |
| **Wer prüft** | `APITokenAuth`-Middleware (`cmd/fileee-server/auth.go`) | Fileee-Server (`my.fileee.com`) |
| **Header** | `X-API-Key: <token>` (Vorrang) **oder** `Authorization: Bearer <token>` | Session-Cookie + `x-xsrf-token` (intern, kein Client-Bezug) |
| **Konfiguriert über** | `FILEEE_API_TOKEN` | `FILEEE_USERNAME`, `FILEEE_PASSWORD`, `FILEEE_TOTP_SEED` (oder Infisical) |
| **Bei Fehlschlag** | HTTP 401, Body immer `{"error":"unauthorized"}`, **nie** der Token-Wert (bewusst 401 statt 403, damit CrowdSecs `http-bruteforce`-Szenario greift) | Automatischer Re-Auth (siehe unten), im Fehlerfall HTTP 502 `upstream_auth` |

1. **Client → fileee-server:** Jeder Request braucht das statische `FILEEE_API_TOKEN`, verglichen
   zeitkonstant per `crypto/subtle.ConstantTimeCompare` (inkl. Fail-closed-Guard: ein leeres
   konfiguriertes Token lehnt IMMER ab, statt versehentlich offen zu sein). Ausnahmen — **kein**
   Token nötig:
   - `GET /healthz`, `GET /openapi.json`, `GET /openapi.yaml` — **immer** ohne Token erreichbar.
   - `GET /docs` — **nur** ohne Token erreichbar, wenn `FILEEE_DOCS_PUBLIC=true` (Default `true`).
     Bei `FILEEE_DOCS_PUBLIC=false` braucht auch die Docs-UI das Token.
2. **fileee-server → Fileee:** die komplette go-fileee-Cookie/TOTP/CSRF-Schicht — Login-Handshake
   (`GET /api/f/start` → `POST /api/f/existent` → `POST /api/f/login`), automatischer Re-Auth bei
   HTTP 403 von Fileee, periodischer Keepalive. Dafür wird einmalig beim Boot ein `*fileee.Client`
   (authentifizierte Routen) und ein zweiter, credential-loser `*fileee.ShareClient` (anonymer
   Share-Proxy, siehe unten) aufgebaut. Details zum Auth-Handshake und den Fehlertypen:
   [go-fileee `docs/API.md`](https://github.com/strausmann/go-fileee/blob/main/docs/API.md).

## Konfiguration (Umgebungsvariablen)

Alle Werte werden ausschließlich über `LoadConfig` (`cmd/fileee-server/config.go`) gelesen — kein
Feld wird an anderer Stelle direkt aus `os.Getenv` bezogen (einzige Ausnahme: der
`healthcheck`-Subcommand-Zweig in `main.go`, der bewusst vor jeder Config-Validierung läuft, siehe
[„Howto — Entwickler"](#howto--entwickler) unten).

| Variable | Zweck | Default | Pflicht | Secret |
|---|---|---|---|---|
| `FILEEE_USERNAME` | Fileee-Login-Benutzername | – | Ja | Ja |
| `FILEEE_PASSWORD` | Fileee-Login-Passwort | – | Ja | Ja |
| `FILEEE_TOTP_SEED` | Base32-TOTP-Seed für Zwei-Faktor-Konten | leer | Nein (nur bei 2FA-Konten) | Ja |
| `FILEEE_API_TOKEN` | Statisches Bearer-Token, mit dem sich Clients gegen den Server authentifizieren (`X-API-Key`- oder `Bearer`-Header) | – | Ja | Ja |
| `FILEEE_ALLOW_DESTRUCTIVE` | Schaltet die drei Hard-DELETE-Routen frei (siehe Destruktiv-Gate unten) | `false` | Nein | Nein |
| `FILEEE_EXPOSE_ATTRIBUTES` | Schaltet die Ausgabe von Fileees automatisch extrahierten Indexierungs-Metadaten (`attributes.data`) über `GET /v1/documents/{id}?includeAttributes=true` frei (siehe Attributes-Gate unten) | `false` | Nein | Nein |
| `FILEEE_LISTEN_ADDR` | Adresse, auf der der HTTP-Server lauscht | `:8080` | Nein | Nein |
| `FILEEE_SESSION_PATH` | Pfad, unter dem die Fileee-Session persistiert wird | `/home/nonroot/session.json` | Nein | Nein (Dateiinhalt ist sensibel, Rechte `0600`) |
| `FILEEE_KEEPALIVE_INTERVAL` | Intervall des Session-Keepalive | `15m` | Nein | Nein |
| `FILEEE_WAIT_TIMEOUT` | Default-Wartezeit von `POST /v1/processes/{id}/wait`, falls kein `?timeout=` mitgeschickt wird | `60s` | Nein | Nein |
| `FILEEE_WAIT_MAX` | Obergrenze, auf die jedes angeforderte Wait-Timeout gedeckelt wird | `300s` | Nein | Nein |
| `FILEEE_RATE_RPS` | Erlaubte Request-Rate/Sekunde gegen die Fileee-API (gilt für den authentifizierten Client UND den anonymen ShareClient) | `1` | Nein | Nein |
| `FILEEE_RATE_BURST` | Burst-Größe des Token-Buckets | `3` | Nein | Nein |
| `FILEEE_TRUSTED_PROXIES` | Kommagetrennte IPs/CIDRs vertrauenswürdiger Reverse-Proxies (Access-Log/Client-IP-Ermittlung) | leer | Nein | Nein |
| `FILEEE_CLIENT_IP_HEADERS` | Kommagetrennte Header-Prüfreihenfolge zur Client-IP-Ermittlung | `CF-Connecting-IP,X-Real-IP,X-Forwarded-For` | Nein | Nein |
| `FILEEE_DOCS_PUBLIC` | Ob `/docs` (Doku-UI) ohne API-Token erreichbar ist | `true` | Nein | Nein |
| `FILEEE_MAX_UPLOAD_SIZE` | Max. Body-Größe von `POST /v1/documents` in Bytes | `33554432` (32 MiB) | Nein | Nein |
| `FILEEE_WEBHOOK_URL` | Ziel-URL für ausgehende Webhook-Benachrichtigungen des Änderungs-Watchers | leer (Webhooks deaktiviert) | Nein | Nein |
| `FILEEE_WEBHOOK_SECRET` | Signiert ausgehende Webhook-Payloads (HMAC-SHA256, Header `X-Fileee-Signature`) | leer | Nein | Ja |
| `FILEEE_WATCH_INTERVAL` | Polling-Intervall des Änderungs-Watchers (`0` = deaktiviert) | `0` (Watcher deaktiviert) | Nein | Nein |
| `FILEEE_USER_AGENT` | Überschreibt den User-Agent gegen Fileee | leer (Core-Lib-Default) | Nein | Nein |
| `FILEEE_LOG_LEVEL` | Log-Level des strukturierten Loggers (`slog`) | `info` | Nein | Nein |

**22 `FILEEE_*`-Variablen insgesamt** (3 davon Pflicht: `FILEEE_USERNAME`, `FILEEE_PASSWORD`,
`FILEEE_API_TOKEN`).

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
Beschreibung: OpenAPI 3.1 unter `/openapi.json`/`/openapi.yaml`, interaktive Docs unter `/docs`.
Insgesamt **44 Huma-Operationen + 1 Plain-Mux-Route** (`/healthz`) = **45 HTTP-Routen**
(die 3 Destruktiv-Routen sind nur bedingt registriert, siehe unten).

### Dokumente/Seiten (8)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| GET | `/v1/documents` | Liste bzw. Volltextsuche (`?q=`) | Nein |
| POST | `/v1/documents` | Upload (multipart), Server erkennt Duplikate | Ja |
| GET | `/v1/documents/{id}` | Einzelabruf, optional `?includeAttributes=true` (siehe Attributes-Gate unten) | Nein |
| PUT | `/v1/documents/{id}` | Metadaten ändern (Optimistic Locking über `version`) | Ja |
| GET | `/v1/documents/{id}/pdf` | Original-PDF als Stream | Nein |
| GET | `/v1/pages/{pageId}/image` | Seiten-Bild-Fallback als Stream | Nein |
| GET | `/v1/pages/{pageId}/ocr` | OCR-Tokens (Text + Bounding-Box) einer eigenen Seite | Nein |
| POST | `/v1/documents/export-zip` | Passwortgeschützter ZIP-Export (asynchron, über Prozess) | Ja (async) |

### Stammdaten, Boxen, Reminders, Contacts (14)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| GET | `/v1/tags` | Tags auflisten | Nein |
| GET | `/v1/companies` | Firmen auflisten | Nein |
| GET | `/v1/contacts` | Kontakte auflisten | Nein |
| GET | `/v1/document-types` | Dokumenttypen auflisten | Nein |
| GET | `/v1/document-type-schemes` | Dokumenttyp-Schemas auflisten | Nein |
| GET | `/v1/reminders` | Erinnerungen auflisten | Nein |
| GET | `/v1/boxes` | FileeeBoxen auflisten | Nein |
| GET | `/v1/boxes/{id}` | Einzelne FileeeBox abrufen | Nein |
| POST | `/v1/boxes/{boxId}/documents/{docId}` | Dokument in eine Box einheften | Ja (kein Destruktiv-Gate) |
| DELETE | `/v1/boxes/{boxId}/documents/{docId}` | Dokument aus einer Box aushängen | Ja (kein Destruktiv-Gate) |
| POST | `/v1/reminders` | Erinnerung erstellen | Ja |
| PUT | `/v1/reminders/{id}` | Erinnerung aktualisieren | Ja |
| POST | `/v1/contacts` | Kontakt erstellen | Ja |
| PUT | `/v1/contacts/{id}` | Kontakt aktualisieren | Ja |

### Freigabe + Prozesse (4)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| POST | `/v1/share` | Freigabe-Link erzeugen | Ja |
| POST | `/v1/documents/{id}/unshare` | Freigabe widerrufen | Ja |
| GET | `/v1/processes/{id}` | Prozess-Status abfragen (Polling) | Nein |
| POST | `/v1/processes/{id}/wait` | Blockierend auf Prozess-Abschluss warten (gedeckelt auf `FILEEE_WAIT_MAX`) | Nein (blockierend) |

### Share-Proxy, anonym — ohne Fileee-Login (4)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| POST | `/v1/share-objects/{token}` | Freigabe-Token auflösen | Nein |
| GET | `/v1/share-objects/{token}/pages/{pageId}/image` | Seiten-Bild einer Freigabe | Nein |
| GET | `/v1/share-objects/{token}/pages/{pageId}/ocr` | OCR-Tokens einer freigegebenen Seite | Nein |
| GET | `/v1/share-objects/{token}/documents/{docId}/pdf` | PDF eines freigegebenen Dokuments | Nein |

### Unified Resolver (1)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| POST | `/v1/resolve` | Ein Fileee-Link rein (`{url}`), ein einheitliches Dokument raus — erkennt intern vs. anonym automatisch (optional `?include=ocr`) | Nein |

### Konversationen (10)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| GET | `/v1/conversations` | Konversationen auflisten (Diff-Sync) | Nein |
| GET | `/v1/conversations/invitations` | Ausstehende Einladungen auflisten | Nein |
| POST | `/v1/conversations/invitations/accept/{token}` | Einladung annehmen (Pfadform bewusst `accept/{token}`, siehe Hinweis unten) | Ja |
| GET | `/v1/conversations/{id}` | Einzelne Konversation abrufen | Nein |
| GET | `/v1/documents/{id}/conversations` | Konversationen, in denen ein Dokument geteilt ist | Nein |
| POST | `/v1/conversations/{id}/messages` | Chat-Nachricht senden | Ja |
| POST | `/v1/conversations/{id}/documents/{docId}` | Dokument in Konversation teilen | Ja |
| DELETE | `/v1/conversations/{id}/documents/{docId}` | Dokument aus Konversation entfernen | Ja (kein Destruktiv-Gate) |
| POST | `/v1/conversations/{id}/participants` | Teilnehmer hinzufügen | Ja |
| DELETE | `/v1/conversations/{id}/participants/{participantId}` | Teilnehmer entfernen | Ja |

> **Pfad-Sonderfall** (`accept-conversation-invitation`): Die Route lautet
> `/v1/conversations/invitations/accept/{token}` — **nicht** `.../{token}/accept`. Grund: Go 1.22+
> `http.ServeMux` hätte sonst einen Pattern-Konflikt mit `/v1/conversations/{id}/documents/{docId}`
> (beide 5-Segment-Pfade wären an mehreren Positionen unterschiedlich dominant und die
> Registrierung würde zur Laufzeit panicen) — empirisch verifiziert
> (`handlers_conversations.go:42-60`).

### Destruktiv, Hard-DELETE — **nur wenn `FILEEE_ALLOW_DESTRUCTIVE=true`** (3)

| Methode | Pfad | Beschreibung | Mutation? |
|---|---|---|---|
| DELETE | `/v1/documents/{id}` | Dokument unwiderruflich löschen | Ja, **Hard-DELETE** |
| DELETE | `/v1/contacts/{id}` | Kontakt unwiderruflich löschen | Ja, **Hard-DELETE** |
| DELETE | `/v1/reminders/{id}` | Erinnerung unwiderruflich löschen | Ja, **Hard-DELETE** |

### Sonstiges

| Methode | Pfad | Beschreibung | Auth |
|---|---|---|---|
| GET | `/healthz` | Liveness-Check, **kein** Fileee-Roundtrip (registriert direkt auf dem `http.ServeMux`, nicht als Huma-Operation) | Kein Token nötig |
| GET | `/openapi.json`, `/openapi.yaml` | Maschinenlesbare API-Beschreibung | Kein Token nötig |
| GET | `/docs` | Interaktive Docs-UI | Kein Token nötig NUR wenn `FILEEE_DOCS_PUBLIC=true` |

### Destruktiv-Gate — exaktes Verhalten

`FILEEE_ALLOW_DESTRUCTIVE` (Default `false`) steuert **ausschließlich**, ob die drei
Hard-DELETE-Routen oben überhaupt registriert werden — **normale Schreib-Operationen und
Soft-artige Zuordnungsänderungen laufen unabhängig davon**: Uploads, Updates, Reminder-/
Kontakt-Anlage, Box-Einheften/-Aushängen und Konversations-Schreibaktionen funktionieren immer,
auch bei `FILEEE_ALLOW_DESTRUCTIVE=false`. Das Flag gated **ausschließlich** die drei echten,
unwiderruflichen Hard-DELETEs — das ist der Standardfall, kein Sonderfall.

- Bei `false` sind die drei DELETE-Pfade dem `http.ServeMux` für das DELETE-Verb komplett
  unbekannt — **kein** „Route existiert, lehnt aber mit 403 ab"-Zwischenzustand. Da GET/PUT auf
  denselben drei Pfaden bereits registriert sind, liefert Go 1.22+ `http.ServeMux` in diesem Fall
  **405 Method Not Allowed** statt 404.
- Jede tatsächlich ausgeführte Destruktiv-Operation wird **vor** dem Löschversuch als
  strukturierte Audit-Log-Zeile auf **Warn-Level** protokolliert.
- **Nicht** unter das Gate fallend (ausdrücklich als „kein Destruktiv-Gate" dokumentiert, da keine
  Dokument-Löschung, nur eine Zuordnung wird zurückgenommen): `DELETE
  /v1/boxes/{boxId}/documents/{docId}` (Box-Aushängen) und `DELETE
  /v1/conversations/{id}/documents/{docId}` (Chat-Dokument-Entfernen).

Hintergrund und Abwägung: [ADR-0008](docs/adr/0008-fileee-server.md) sowie das nur in `go-fileee`
liegende
[ADR-0007](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0007-ausschluss-destruktiver-operationen.md)
(Ausschluss destruktiver Lib-Operationen — durch ADR-0008 für den Server verfeinert, nicht
abgelöst).

### Attributes-Gate (`FILEEE_EXPOSE_ATTRIBUTES`) — exaktes Verhalten

Fileee klassifiziert und indexiert jedes Dokument automatisch (Dokumenttyp, Absender/Empfänger,
Tags, Rechnungsdatum, Rechnungsbetrag, IBAN, Kundennummer, …) — dieser Fund liegt intern unter
`attributes.data` (go-fileee: `fileee.Document.Attributes`) und wurde bis inkl. der letzten Version
**grundsätzlich nicht** über die API ausgeliefert (nur `status`, `uploadAttribute`, `pages`,
`sharedSpaceIds`). `GET /v1/documents/{id}?includeAttributes=true` liefert diese Metadaten jetzt
optional mit — als eigenes, typisiertes `attributes`-Objekt (Absender-/Empfänger-**IDs**,
Rechnungsnummer, Rechnungs-/Ausstellungs-/Fälligkeitsdatum, Beträge, Bankverbindung, Kundennummer,
Zahlungsreferenz, Tags, …), **nicht** ein roher Passthrough der Fileee-internen Wire-Struktur.

**`attributes.data` ist private Finanz-PII** (Rechnungsbeträge, IBAN, Kundennummer, Absender). Die
Ausgabe ist deshalb — analog zum Destruktiv-Gate — **zweifach** opt-in:

1. **Betreiber-seitig:** `FILEEE_EXPOSE_ATTRIBUTES=true` muss beim Serverstart gesetzt sein.
2. **Aufrufer-seitig:** der Query-Parameter `?includeAttributes=true` muss auf dem einzelnen Request
   mitgeschickt werden.

Fehlt **eine** der beiden Zustimmungen, bleibt das Verhalten **exakt** wie zuvor — kein
`attributes`-Feld im Body. Ist NUR der Parameter gesetzt (Gate aus), antwortet der Server
**explizit mit 403** (`{"error":"attribute exposure disabled; set FILEEE_EXPOSE_ATTRIBUTES=true to
enable","code":"attributes_disabled"}`) statt den Parameter still zu ignorieren — der Aufrufer soll
erkennen, dass er PII angefordert hat, die der Betreiber nicht freigeschaltet hat.

**Migrations-Einsatzzweck (Fileee → DMS, z. B. Paperless-ngx):** die drei wichtigsten Felder für
eine metadatentragende Migration sind alle enthalten — `senderId` (→ Correspondent), `invoiceId`
(→ z. B. ASN/Custom-Field, **nicht** zu verwechseln mit der separaten `customerId`) und
`invoiceDate` (→ z. B. `created`). Vollständige Feldliste: `cmd/fileee-server/attributes.go`
(`documentAttributesBody`).

| includeAttributes | FILEEE_EXPOSE_ATTRIBUTES | Ergebnis |
|---|---|---|
| weggelassen/`false` | egal | unverändert — kein `attributes`-Feld |
| `true` | `false` (Default) | **403** `attributes_disabled` |
| `true` | `true` | 200, `attributes`-Feld gesetzt |

### Interaktive Doku (`/docs`)

Der Server generiert seine OpenAPI-3.1-Spezifikation zur Laufzeit aus den getippten
Request-/Response-Strukturen ([Huma](https://huma.rocks/)) und stellt dazu eine self-contained
Docs-UI unter `GET /docs` bereit (kein CDN, passt zu CSP-freiem Betrieb). Maschinenlesbar:
`GET /openapi.json` / `GET /openapi.yaml`. Ob `/docs` ohne API-Token erreichbar ist, steuert
`FILEEE_DOCS_PUBLIC` (Default `true`).

## Fehler → HTTP-Mapping

`mapError` (`cmd/fileee-server/errors.go`) übersetzt jeden Fehler der Core-Lib
(`github.com/strausmann/go-fileee/fileee`) **ausschließlich** über `errors.Is`/`errors.As` gegen
die von der Lib exportierten Sentinel-Fehler bzw. Typen — **nie** über Fehlertext-Matching.

| Lib-Fehler | HTTP-Status | `code` | `Retry-After` |
|---|---|---|---|
| `fileee.ErrDuplicateDocument` | 409 Conflict | `duplicate` | – |
| `fileee.ErrUnsupportedFileType` | 415 Unsupported Media Type | `unsupported_file_type` | – |
| `fileee.ErrRateLimited` | 429 Too Many Requests | `rate_limited` | **Keine** — die Lib führt keinen Sekundenwert mit (Backoff bereits ausgeschöpft) |
| `*fileee.BlockedError` | 503 Service Unavailable | `blocked` | Ja, `SecondsBlocked` aus dem Fehler |
| `fileee.ErrNotFound` | 404 Not Found | `not_found` | – |
| `fileee.ErrSessionExpired` | 502 Bad Gateway | `upstream_auth` | – (Re-Auth nach Session-Ablauf endgültig fehlgeschlagen) |
| sonstiger `*fileee.APIError` | Pass-Through `apiErr.HTTPStatus` | `apiErr.Code` (Fallback `api_error`) | – |
| alles andere | 500 Internal Server Error | `internal_error` | – |

Response-Body-Schema (eigener Typ `statusError`, `errors.go`): **nur** `{"error": "...", "code":
"..."}` — bewusst **nicht** Humas RFC-9457-`ErrorModel`, damit N8N-Workflows und
CI-Automatisierung ein stabiles, minimales Fehlerformat auswerten können. Ausnahme: ein
Upload-Duplikat (`POST /v1/documents`) liefert zusätzlich `id` und `isDuplicate: true`.

## Howto — Server-Nutzer/Betreiber

### Deployment

```bash
# Bearer-Token für Clients generieren (64 Hex-Zeichen)
openssl rand -hex 32

# Session-Volume für den nonroot-User (uid 65532) vorbereiten
sudo mkdir -p /pfad/zur/session && sudo chown 65532:65532 /pfad/zur/session

# Compose-Template kopieren, Platzhalter befüllen, starten
docker compose -f deploy/compose.plain.yaml up -d
```

Secrets (Fileee-Zugangsdaten, `FILEEE_API_TOKEN`, ggf. Infisical-Machine-Identity) **niemals**
Klartext in eine committete `.env`-Datei schreiben — produktiv über einen Secret-Manager
(Vaultwarden/Infisical) injizieren. Im Infisical-Dual-Mode übernimmt die Binary das selbst (siehe
[„Zwei Auth-Schichten"](#zwei-auth-schichten-nicht-verwechseln) und die
`SECRET_BACKEND=infisical`-Variablen oben) — es ist **kein** externer `infisical run`-Wrapper um
den Container nötig.

### Beispiel-Request

```bash
curl -sS https://fileee.example.com/v1/documents \
  -H "Authorization: Bearer ${FILEEE_API_TOKEN}"
```

Äquivalent mit `X-API-Key` (hat Vorrang, falls beide Header gesetzt sind):

```bash
curl -sS https://fileee.example.com/v1/documents \
  -H "X-API-Key: ${FILEEE_API_TOKEN}"
```

### Healthcheck

`GET /healthz` beantwortet ein festes `{"status":"ok"}` mit HTTP 200 — **ohne** Roundtrip gegen
Fileee (reine Prozess-Liveness, kein Readiness-Check gegen die Fileee-Session). Braucht **kein**
Token. Im Container läuft derselbe Check über das eingebaute Subcommand (kein Shell im
distroless-Image):

```bash
/fileee-server healthcheck   # Exit-Code 0 bei 2xx, sonst 1
```

Docker-`HEALTHCHECK` in `deploy/Dockerfile` nutzt genau diesen Aufruf mit `--interval=30s
--timeout=5s --start-period=5s --retries=3`.

### Subcommands

Die Binary kennt zwei Subcommands; ohne Argument startet der Server.

| Aufruf | Wirkung | Exit-Code |
|---|---|---|
| `fileee-server` | startet den Server | — |
| `fileee-server healthcheck` | GET auf `127.0.0.1:<port>/healthz` | `0` bei 2xx, sonst `1` |
| `fileee-server version` | gibt die Version aus und beendet sich | `0` |
| alles andere | Fehlermeldung nach stderr | `2` |

Kein Subcommand nimmt Parameter — `fileee-server version foo` ist ein Fehler mit Exit-Code `2`,
kein stillschweigend ignoriertes `foo`.

`version` ist im distroless-Image der einzige Weg, die Version ohne laufenden Server
abzufragen — es gibt keine Shell:

```bash
docker run --rm ghcr.io/strausmann/fileee-server:latest version
```

Ein unbekanntes Argument ist **bewusst** ein Fehler statt still ignoriert zu werden: Sonst
startet ein Tippfehler wie `healthcheckk` einen zweiten Serverprozess, statt fehlzuschlagen,
und der Container-`HEALTHCHECK` läuft in den Timeout statt sauber Exit 1 zu liefern.

### Rate-Limit und Trusted-Proxies hinter einem Reverse Proxy

- `FILEEE_RATE_RPS`/`FILEEE_RATE_BURST` begrenzen die Requests **gegen Fileee**, nicht die
  Requests, die Clients gegen `fileee-server` stellen — Default 1 req/s, Burst 3, gilt für den
  authentifizierten Client **und** den anonymen `ShareClient` gleichermaßen.
- Läuft der Server hinter einem Reverse Proxy (Traefik, Pangolin, nginx), MUSS
  `FILEEE_TRUSTED_PROXIES` (CIDR-Liste) gesetzt werden, damit die Client-IP-Ermittlung aus
  `FILEEE_CLIENT_IP_HEADERS` (Default `CF-Connecting-IP,X-Real-IP,X-Forwarded-For`) nicht spoofbar
  ist — die Header werden **nur** berücksichtigt, wenn die TCP-Quell-IP in einem der
  konfigurierten CIDRs liegt. Ohne diese Konfiguration protokolliert der Server die
  Proxy-IP statt der echten Client-IP.

### Logs

Zwei getrennte Log-Ströme:

- **Access-Log** (NGINX-`combined`-Format) auf **stdout** — erlaubt CrowdSec, den vorhandenen
  `crowdsecurity/nginx`-Parser (`http-probing`, `http-bruteforce`, `http-crawl-non-statics`) ohne
  eigenen Custom-Parser zu nutzen.
- **App-/Audit-Log** (strukturiertes JSON via `slog`) auf **stderr** — Level über
  `FILEEE_LOG_LEVEL` (`debug`/`info`/`warn`/`error`, Default `info`). Jede tatsächlich ausgeführte
  Destruktiv-Operation (Hard-DELETE) wird hier zusätzlich auf `warn`-Level protokolliert, bevor der
  Löschversuch überhaupt startet.

### CrowdSec-Anbindung aktivieren

> **Die Datei `deploy/crowdsec/acquis.d/fileee-server.yaml` im Repo ist ein Template, kein
> aktiver Zustand.** Solange die beiden folgenden Schritte auf dem Ziel-Node nicht ausgeführt
> sind, liest niemand den Access-Log — es gibt keine Acquisition, keinen Parser und keine
> Alerts. Der bewusste 401-statt-403-Entwurf der Auth-Middleware bleibt dann wirkungslos.

**Schritt 1 — Acquisition ausrollen.** Das Verzeichnis existiert auf einem frischen Node
oft noch nicht:

```bash
sudo mkdir -p /etc/crowdsec/acquis.d
sudo cp deploy/crowdsec/acquis.d/fileee-server.yaml /etc/crowdsec/acquis.d/
```

Läuft CrowdSec selbst im Container, gehört die Datei stattdessen in dessen `acquis.d`-Mount.
Voraussetzungen: der CrowdSec-Agent braucht Zugriff auf den Docker-Socket
(`/var/run/docker.sock`, Default von `docker_host`), der Container muss den Log-Driver
`json-file` nutzen, und `container_name` in der Datei muss zum `container_name` der
verwendeten `deploy/compose.*.yaml` passen (alle drei Templates setzen `fileee-server`).

**Schritt 2 — Collection installieren.** Ohne `crowdsecurity/nginx` gibt es keinen Parser für
das `combined`-Format:

```bash
sudo cscli collections install crowdsecurity/nginx
sudo systemctl reload crowdsec
```

Im Container-Setup stattdessen `crowdsecurity/nginx` zu `CROWDSEC_COLLECTIONS` ergänzen und den
Agent neu starten.

**Schritt 3 — Verifikation (Pflicht).** Ohne diesen Nachweis ist unklar, ob die Anbindung
wirklich greift:

```bash
cscli collections list | grep nginx     # muss crowdsecurity/nginx zeigen
cscli metrics                           # Acquisition des Containers: lines_parsed > 0, nicht nur lines_read

# Positivtest: 401-Burst gegen eine geschützte Route
for i in $(seq 1 20); do
  curl -s -o /dev/null -H "X-API-Key: falsch" https://fileee.example.com/v1/documents
done
cscli alerts list && cscli decisions list   # muss einen Ban zeigen
```

`lines_read > 0` bei `lines_parsed = 0` bedeutet: die Acquisition greift, aber der Parser
fehlt oder bekommt die falschen Zeilen — dann Schritt 2 prüfen und ob wirklich nur **stdout**
eingelesen wird (`follow_stderr: false`, siehe unten).

**Warum `follow_stderr: false` nicht optional ist:** `follow_stderr` steht bei CrowdSec per
Default auf `true`. Da fileee-server stdout und stderr für zwei verschiedene Formate nutzt
(siehe oben), bekäme der `nginx`-Parser sonst das JSON-App-Log mit und erzeugt reine
Parse-Fehler.

**Fallback,** falls die `source: docker`-Acquisition nicht parst: den Access-Log zusätzlich in
eine Datei schreiben und per `source: file` mit `filenames:` und `labels: {type: nginx}`
einlesen.

**Wenn später weitere Dienste dazukommen:** CrowdSec kann Container per Docker-Label selbst
entdecken (`use_container_labels: true` in der Acquisition, dann `crowdsec.enable=true` und
`crowdsec.labels.type=nginx` am jeweiligen Container). Das spart pro Dienst eine eigene
`acquis.d`-Datei. Achtung dabei: Eine 401-basierte Bruteforce-Erkennung darf **nicht** auf
Dienste ausgeweitet werden, die einen OAuth-Discovery-Flow bedienen — der beginnt zwingend mit
einem 401 (`WWW-Authenticate: Bearer resource_metadata=…`) und würde false-positive Bans
erzeugen. Für fileee-server mit reiner `X-API-Key`-/Bearer-Auth ist das unkritisch.

## Howto — Entwickler

### Lokal bauen und testen

```bash
GOTOOLCHAIN=auto go build ./...
GOTOOLCHAIN=auto go vet ./...
GOTOOLCHAIN=auto go test ./... -race -count=1
gofmt -l .   # muss leer bleiben
```

`GOTOOLCHAIN=auto` lässt Go bei Bedarf automatisch eine neuere Toolchain nachladen, falls die
lokal installierte Go-Version älter als die in `go.mod` geforderte `go 1.25.0` ist. In CI läuft
`go build`/`go vet` mit `CGO_ENABLED=0` (passend zum späteren distroless-Runtime-Image), nur die
Test-Stufe mit `-race` aktiviert `CGO_ENABLED=1` (der Race-Detector braucht cgo).

### Projektstruktur

Alles liegt flach in `cmd/fileee-server/` (Package `main`, kein `internal/`-Split):

| Datei | Zuständigkeit |
|---|---|
| `main.go` | Einstiegspunkt, Boot-Reihenfolge, Graceful-Shutdown, `healthcheck`-Subcommand |
| `config.go` | `LoadConfig` — einzige Stelle, die `FILEEE_*`-Env-Variablen liest |
| `secrets.go` | Infisical-Dual-Mode (`MaybeInjectInfisical`, `syscall.Exec`-Re-Exec) |
| `server.go` | `Server`-Struct, `Handler()` (Middleware-Kette + Routen-Registrierung), `/healthz` |
| `api.go` | Huma-API-Konfiguration (OpenAPI/Docs-UI) |
| `auth.go` | `APITokenAuth`-Middleware (Client → Server) |
| `accesslog.go` | NGINX-`combined`-Access-Log-Middleware, Trusted-Proxy-Client-IP-Ermittlung |
| `errors.go` | `mapError`, `statusError`-Typ (Lib-Fehler → HTTP) |
| `handlers_documents.go` | Dokumente/Seiten/OCR/Export-ZIP |
| `handlers_entities.go` | Tags/Companies/Contacts/DocumentTypes/Schemes/Reminders/Boxes |
| `handlers_share.go` | Freigabe erzeugen/widerrufen, Prozesse, anonymer Share-Proxy |
| `handlers_conversations.go` | Konversationen/Chat/Teilnehmer/Einladungen |
| `handlers_destructive.go` | Die drei Hard-DELETE-Routen (nur bei `FILEEE_ALLOW_DESTRUCTIVE=true`) |
| `resolve.go` | Unified Resolver (`POST /v1/resolve`) |
| `watch.go` | Änderungs-Watcher (Poller → Webhook) |

Jede `*.go`-Datei hat eine `*_test.go`-Schwesterdatei.

### Neue Route hinzufügen (Huma-Operation-Muster)

Jede Route ist eine `huma.Register`-Registrierung mit einem typisierten Input- und
Output-Struct. Minimalbeispiel (angelehnt an `handlers_entities.go`):

```go
type getThingInput struct {
    ID string `path:"id" doc:"Beschreibung des Pfad-Parameters."`
}

type getThingOutput struct {
    Body fileee.Thing
}

func (s *Server) registerThingRoutes(api huma.API) {
    huma.Register(api, huma.Operation{
        OperationID: "get-thing",
        Method:      http.MethodGet,
        Path:        "/v1/things/{id}",
        Summary:     "Kurzbeschreibung für die OpenAPI-Doku",
    }, s.handleGetThing)
}

func (s *Server) handleGetThing(ctx context.Context, in *getThingInput) (*getThingOutput, error) {
    thing, err := s.fc.Things.Get(ctx, in.ID)
    if err != nil {
        return nil, mapError(err) // NIE den rohen Lib-Fehler direkt zurückgeben
    }
    return &getThingOutput{Body: *thing}, nil
}
```

Danach die neue `register*Routes`-Methode in `server.go` (`Handler()`) aufrufen. Konventionen:

- **Jeder** Fehler von der Core-Lib läuft durch `mapError` (siehe
  [„Fehler → HTTP-Mapping"](#fehler--http-mapping)) — nie den rohen `error` direkt zurückgeben.
- Destruktive Routen registrieren sich **nur** bedingt, nach dem Muster in
  `handlers_destructive.go` (`if s.cfg.AllowDestructive { ... }` in `server.go`).
- Neue Routen brauchen einen Test in der zugehörigen `handlers_*_test.go` (Happy-Path +
  Fehler-Pfad über einen `httptest`-Mock des Fileee-Backends) — siehe
  `.claude/rules/test-coverage-pflicht.md` im homelab-management-Repo für die Coverage-Pflicht;
  dieses Repo setzt sie über `scripts/coverage-gate-strict.sh` in CI durch (siehe
  `.github/workflows/test.yml`, harte Schwellen pro Datei).
- Zwei-Segment-Pfade mit gemeinsamem Präfix können in Go 1.22+ `http.ServeMux` zu
  Registrierungs-Panics führen (siehe der `accept/{token}`-Sonderfall oben) — bei mehrdeutigen
  Pfad-Mustern vorher lokal `go build ./... && go test ./cmd/fileee-server/...` laufen lassen,
  ein Konflikt zeigt sich sofort als Panic beim Serverstart bzw. beim ersten betroffenen Test.

## Sicherheit

- **Credentials** (Fileee-Username/-Passwort/-TOTP-Seed, `FILEEE_API_TOKEN`, Infisical-Machine-Identity) gehören **ausschließlich** in einen Secret-Manager (Vaultwarden/Infisical) — niemals in Code, Fixtures oder Commits.
- Die **Session-Datei** (`FILEEE_SESSION_PATH`) ist ein Secret (Dateirechte `0600`), wird nie geloggt oder committed.
- Der Server läuft als **rootless, distroless** Container (`gcr.io/distroless/static-debian12:nonroot`, `uid 65532`, statisches Binary, `CGO_ENABLED=0`) — keine Shell, kein Paketmanager im Laufzeit-Image.
- Zugriffs-Logs im NGINX-`combined`-Format auf stdout erlauben CrowdSec, den vorhandenen `crowdsecurity/nginx`-Parser (`http-probing`, `http-bruteforce`, `http-crawl-non-statics`) ohne Custom-Parser zu nutzen. Die Client-IP wird nur aus Reverse-Proxy-Headern übernommen, wenn die TCP-Quelle in `FILEEE_TRUSTED_PROXIES` liegt. **Die Anbindung ist nicht automatisch aktiv** — sie muss auf dem Ziel-Node eingerichtet und verifiziert werden, siehe [„CrowdSec-Anbindung aktivieren"](#crowdsec-anbindung-aktivieren).
- Details zur schonenden Fileee-Nutzung (Rate-Limiting/Backoff, geteilt über alle Handler): [go-fileee ADR-0005](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0005-schonender-betrieb-rate-limiting.md).

## Lokale Co-Entwicklung mit go-fileee

Für Änderungen, die **beide** Repos gleichzeitig betreffen (z. B. eine neue Lib-Methode vor ihrem
`go-fileee`-Release nutzen), lokal ein Go-Workspace anlegen — **niemals committen**:

```bash
go work init && go work use . ../go-fileee
```

`go.work`/`go.work.sum` sind in `.gitignore` gelistet. Das committete `go.mod` bleibt immer auf
einen echten, veröffentlichten `go-fileee`-Tag gepinnt (aktuell `v0.2.0`), **ohne** `replace`-
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
