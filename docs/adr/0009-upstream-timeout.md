# ADR-0009: Upstream-Timeout als Server-Middleware mit Routen-Ausnahmeliste

**Status:** proposed
**Datum:** 2026-08-15
**Ersetzt:** —
**Ersetzt durch:** —
**Verwandt:** [go-fileee ADR-0005](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0005-schonender-betrieb-rate-limiting.md), [ADR-0008](0008-fileee-server.md)

## Kontext

Live beobachtet (2026-08-14, während eines Bulk-Reads): nach ~223 authentifizierten Requests in
Folge begannen **alle** authentifizierten Endpunkte (auch triviale GETs wie
`GET /v1/document-types`) unbegrenzt zu hängen — 0 Bytes Response, kein Fehler, auch bei 90s
Client-Timeout kein Ergebnis. `/healthz` antwortete währenddessen durchgehend mit 200 — das
Monitoring bemerkte den Ausfall nicht (siehe `.claude/rules/service-verifikation.md`,
homelab-management-Repo: ein Gesundheitspfad beweist nichts über den echten Endpunkt).

Ursache: `fileee-server` setzte auf keinem Upstream-Call gegen Fileee eine Deadline. go-fileees
`defaultTransport()` setzt zwar `ResponseHeaderTimeout=30s`, aber **bewusst kein**
`http.Client.Timeout` — laut dessen eigener Doku, um große Uploads/ZIP-Exports nicht mittendrin
abzuschneiden (die per Design über lange Zeit streamen). Ein wedged Fileee-Backend legte dadurch
effektiv die gesamte authentifizierte API lahm, ohne dass der Server selbst je einen Fehler warf.

Rate-Limiting und Backoff waren zum Zeitpunkt der Untersuchung bereits vollständig verdrahtet
(`main.go` ruft `fileee.WithRateLimit` für **beide** Clients auf, go-fileees
`rateLimitedTransport` nutzt den Limiter in jedem Roundtrip, `ExponentialBackoff` — ADR-0005 im
go-fileee-Repo — ist Default). Die Lücke war ausschließlich der fehlende Timeout.

Vollständiger Issue-Text: [#44](https://github.com/strausmann/fileee-server/issues/44).

## Entscheidung

1. **Deadline auf Server-Ebene, nicht in go-fileee.** `fileee-server` setzt eine
   `context.WithTimeout`-Deadline auf dem eingehenden Request-Context, bevor er an `s.fc`/`s.sc`
   delegiert wird (`FILEEE_UPSTREAM_TIMEOUT`, Default `30s`) — als HTTP-Middleware
   (`UpstreamTimeout`, `upstream_timeout.go`), nicht als Änderung an go-fileees `http.Client`.
   CONTRIBUTING.md verbietet neue Fileee-Protokoll-Logik im Server-Repo; ein request-lebenszyklus-
   bezogenes Timeout ist dagegen genuin Server-Zuständigkeit (der Server kennt seine eigenen
   Antwortzeit-Erwartungen gegenüber seinen Clients, die Lib kennt sie nicht).

2. **Ausnahmeliste statt eines pauschalen Timeouts für alle Routen.** Dieselbe Begründung, aus der
   go-fileee selbst kein `http.Client.Timeout` setzt, gilt für einen Teil der Server-Routen:
   - `POST /v1/processes/{id}/wait` — blockiert AUSDRÜCKLICH bis zu `FILEEE_WAIT_MAX` (bis zu
     300s, Design-Spec §4.4) — das ist keine Hänger-Situation, sondern die vom Handler selbst
     bereits gedeckelte Wartesemantik.
   - `POST /v1/documents` (Upload) und `POST /v1/documents/export-zip` (ZIP-Export).
   - `GET .../pdf` und `GET .../image` — Voll-PDFs und Seitenbilder, direkt UND über den anonymen
     Share-Proxy (`/v1/share-objects/{token}/...`).

   Alle übrigen Routen (insbesondere die schlanken JSON-GETs/-POSTs, deren Hängen Issue #44
   überhaupt erst auslöste) unterliegen der Deadline.

3. **`0` deaktiviert die Deadline vollständig, statt einen Minimalwert zu erzwingen** — konsistent
   mit `WatchInterval` (`0` = Watcher deaktiviert). Wichtiger Nebeneffekt: `Config{}`-Literale (wie
   sie die überwiegende Mehrheit der bestehenden Handler-Tests direkt bauen, ohne `LoadConfig` zu
   durchlaufen) bleiben dadurch unverändert unbegrenzt — nur ein echter Serverstart über
   `LoadConfig` aktiviert den 30s-Default. Ohne diesen Zero-heißt-aus-Mechanismus hätte JEDER
   bestehende Handler-Test, der `Config{}` ohne explizites `UpstreamTimeout` baut, durch eine
   sofort abgelaufene 0s-Deadline gebrochen.

4. **`context.DeadlineExceeded` → HTTP 504 `upstream_timeout`** (`mapError`, `errors.go`) statt in
   den bisherigen 500-Default-Fall zu fallen — `errors.Is` findet den Sentinel über die
   `%w`-Unwrap-Kette von go-fileees eigenen Fehler-Wraps UND über `net/url.Error.Unwrap`, ohne
   dass go-fileee dafür etwas Eigenes exportieren muss.

## Konsequenzen

**Positiv:**
- Ein wedged Fileee-Backend legt nicht mehr die gesamte API lahm — betroffene Requests scheitern
  innerhalb einer festen, konfigurierbaren Frist mit einem eindeutigen, maschinenlesbaren Fehler
  (`504 upstream_timeout`) statt unbegrenzt zu hängen.
- Große Uploads/Exports/Downloads bleiben unangetastet — die Ausnahmeliste verhindert, dass ein zu
  kurzes Timeout genau die Operationen bricht, für die go-fileee bewusst kein pauschales Timeout
  vorsieht.
- Bestehende Tests und Deployments ohne `LoadConfig` (z. B. `Config{}`-Literale) sind durch den
  Zero-heißt-aus-Default nicht betroffen — keine stille Verhaltensänderung für Code, der das Feld
  nicht kennt.

**Negativ / bewusst in Kauf genommen:**
- Die Ausnahmeliste ist eine **Pfad-/Methoden-Matching-Middleware** (`isUpstreamTimeoutExempt`),
  keine pro-Operation deklarierte Eigenschaft in der Huma-Registrierung — neue Routen mit
  intrinsisch langer Laufzeit müssen der Liste manuell hinzugefügt werden, sonst fallen sie
  unter die 30s-Default-Deadline. Ein Test (`TestIsUpstreamTimeoutExempt`) deckt die aktuelle
  Liste ab, verhindert aber nicht, dass eine künftige Route vergessen wird.
- `30s` ist ein pragmatischer Default (Issue-Vorschlag), keine empirisch hergeleitete Schwelle —
  kann bei Bedarf über `FILEEE_UPSTREAM_TIMEOUT` angepasst werden.
- Ein optionaler Circuit-Breaker (Issue #44, Punkt 2) und ein vertiefter „deep"-Health-Check
  (Punkt 3) sind bewusst NICHT Teil dieser Entscheidung — im Issue selbst als optional markiert,
  Rate-Limiting/Backoff decken das eskalierende Verhalten bereits ab (siehe Kontext).

## Referenzen

- Issue: [#44](https://github.com/strausmann/fileee-server/issues/44)
- [go-fileee ADR-0005](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0005-schonender-betrieb-rate-limiting.md)
  (Rate-Limiting/Backoff — bereits verdrahtet, unverändert durch dieses ADR)
- [ADR-0008](0008-fileee-server.md) (fileee-server-Grundarchitektur)
- `.claude/rules/service-verifikation.md` (homelab-management-Repo) — Gesundheitspfad beweist
  nichts über den echten Endpunkt, Auslöser der Untersuchung
