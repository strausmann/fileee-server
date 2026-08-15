# Architecture Decision Records — fileee-server

Diese Registry führt alle Architecture Decision Records (ADRs) für `fileee-server`. ADRs dokumentieren wichtige, langfristig wirkende Entscheidungen — inklusive Kontext und Konsequenzen — damit spätere Sessions und Mitwirkende nachvollziehen können, **warum** etwas so gebaut wurde und nicht anders.

Neues ADR anlegen: Kopiere ein bestehendes ADR aus diesem Verzeichnis als Vorlage (Felder Status/Datum/Lineage/Kontext/Entscheidung/Konsequenzen/Referenzen) nach `docs/adr/NNNN-slug.md` (nächste freie Nummer, vierstellig) und trage es unten in die Tabelle ein.

## Vorgeschichte: ADR-0001–0007 leben im go-fileee-Repo

`fileee-server` ist per Repo-Split aus [`strausmann/go-fileee`](https://github.com/strausmann/go-fileee)
hervorgegangen (Task B2, 2026-07-24). Die grundlegenden ADRs zur Core-Lib —
[ADR-0001 Library-first-Architektur](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0001-library-first-architektur.md),
[ADR-0002 Auth-Modell](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0002-auth-modell-session-cookie-totp.md),
[ADR-0003 Reverse-engineered API-Risiko](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0003-reverse-engineered-api-risiko.md),
[ADR-0004 Test-Strategie](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0004-test-strategie.md),
[ADR-0005 Schonender Betrieb / Rate-Limiting](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0005-schonender-betrieb-rate-limiting.md),
[ADR-0006 Domänen-Neutralität](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0006-domaenen-neutralitaet.md) und
[ADR-0007 Ausschluss destruktiver Operationen](https://github.com/strausmann/go-fileee/blob/main/docs/adr/0007-ausschluss-destruktiver-operationen.md) —
betreffen die Core-Lib und bleiben dort. `fileee-server` konsumiert die Core-Lib nur noch als
versionierte externe Abhängigkeit (`github.com/strausmann/go-fileee`, aktuell `v0.1.0`) und muss
diese ADRs bei Bedarf im go-fileee-Repo nachschlagen — sie werden **nicht** dupliziert.

ADR-0008 selbst ist beim Split mit umgezogen (dieses Repo ist sein „Zuhause", da es den Server
beschreibt) und führt die Nummerierung fort statt neu bei `0001` zu beginnen.

## ADR-Regelwerk

- **Was/Wann:** Ein ADR dokumentiert **jede bedeutsame Architektur-, Technologie- oder Betriebs-Entscheidung** samt Kontext und Konsequenzen — damit spätere Sessions und Mitwirkende nachvollziehen können, warum etwas so entschieden wurde. **Nicht** für Trivialitäten, reine Formatierung oder offensichtliche Umsetzungsdetails.
- **Nummerierung:** fortlaufend `NNNN` (4-stellig), Dateiname `NNNN-kebab-slug.md`. Nummern werden **nie wiederverwendet** — auch nicht für abgelöste/verworfene ADRs. Die Zählung setzt die von go-fileee übernommene Nummer `0008` fort (nächstes neues ADR in diesem Repo: `0010`).
- **Status-Lifecycle:** `proposed` → `accepted` → (`superseded` | `deprecated`). Ein ADR startet als `proposed`, wird nach Freigabe `accepted`, und geht bei Ablösung/Ungültigkeit in einen der Endzustände über.
- **Lineage (beidseitig pflegen):** Die Header-Felder `Ersetzt` / `Ersetzt durch` (vollständige Ablösung → Vorgänger auf `superseded` setzen) und `Verwandt` (Querbezug ohne Ablösung) werden **auf beiden beteiligten ADRs** eingetragen. Verweist ein ADR auf ein ADR im go-fileee-Repo (oder umgekehrt), wird die **volle Cross-Repo-URL** verwendet (siehe ADR-0008-Header), keine relativen Pfade. **Beim Ablösen nur den Header** des alten ADR anfassen (Status + `Ersetzt durch`) — Kontext und Entscheidung des alten ADR werden **nie umgeschrieben** (sie sind ein historisches Protokoll).
- **Registry-Pflicht:** Jedes neue oder im Status geänderte ADR wird **sofort** in die Registry-Tabelle unten eingetragen bzw. aktualisiert (Nr, Titel, Status, Datum). Ein ADR, das nicht in der Registry steht, gilt als „übersehen" und damit als nicht existent.
- **Sprache:** ADRs werden auf **Deutsch** verfasst (echte Umlaute ä ö ü ß), Code/CLI/Bezeichner auf Englisch.

## Registry

| Nr. | Titel | Status | Datum |
|-----|-------|--------|-------|
| [0008](0008-fileee-server.md) | fileee-server — REST-API-Service (Single-Tenant, geguardetes Löschen) | proposed | 2026-07-24 |
| [0009](0009-response-body-registry-aus-echter-registrierung.md) | DTO-Leak-Guardrail wird aus der echten `huma.Register`-Verdrahtung abgeleitet | proposed | 2026-08-15 |

## Status-Werte

| Status | Bedeutung |
|--------|-----------|
| `proposed` | Entwurf, noch nicht final entschieden |
| `accepted` | Gültig, wird umgesetzt/befolgt |
| `superseded` | Durch ein neueres ADR vollständig abgelöst (siehe `Ersetzt durch` im Header) |
| `deprecated` | Nicht mehr gültig, ohne direkten Nachfolger |
