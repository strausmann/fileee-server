---
name: Bug-Report
about: Ein Problem mit fileee-server melden
title: "bug: "
labels: bug
---

## Beschreibung

Kurze Beschreibung des Problems.

## Reproduktion

Schritte, um das Verhalten zu reproduzieren (Route, Request-Body/Query, erwarteter vs.
tatsächlicher HTTP-Status):

1. …
2. …

**Keine echten Credentials, API-Token oder personenbezogene Daten (PII) posten** — vor dem
Anhängen von Requests/Responses entfernen bzw. durch synthetische Werte ersetzen.

## Erwartetes Verhalten

Was hätte passieren sollen?

## Tatsächliches Verhalten

Was ist stattdessen passiert? Fehlermeldung/Response-Body/Stacktrace falls vorhanden.

## Umgebung

- fileee-server-Version/Tag: `<z. B. v0.2.0>`
- go-fileee-Version (siehe `go.mod`): `<z. B. v0.1.1>`
- Betriebsart: [ ] Binary direkt [ ] Docker-Compose (`deploy/compose.plain.yaml`)
  [ ] Traefik-Setup [ ] Pangolin-Setup
- Secret-Backend: [ ] `.env` [ ] Infisical-Dual-Mode (`SECRET_BACKEND=infisical`)
- Go-Version: `go version`

## Zusätzlicher Kontext

Alles Weitere, das beim Debuggen hilft (Logs mit maskierten Secrets, `docker compose logs`).
