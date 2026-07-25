---
name: Feature-Request
about: Eine neue Funktion für fileee-server vorschlagen
title: "feat: "
labels: enhancement
---

## Problem / Motivation

Welches Problem löst der Vorschlag? Welche Route/welcher Anwendungsfall fehlt aktuell?

## Vorschlag

Wie könnte die Lösung aussehen (neue Route, geändertes Response-Format, neue Config-Option)?

## Einordnung: Server oder Core-Lib?

`fileee-server` ist ein dünner REST-Wrapper und delegiert jede Fileee-Operation 1:1 an
`go-fileee`. Bitte kurz einordnen:

- [ ] Der Vorschlag betrifft nur den Server (Routing, Auth, Response-Format, Deployment) —
      passt in dieses Repo.
- [ ] Der Vorschlag braucht neue Fileee-Protokoll-Abdeckung (neue Entity, neuer Endpunkt bei
      `my.fileee.com`) — gehört ins
      [go-fileee-Repo](https://github.com/strausmann/go-fileee), nicht hierher.

## Alternativen

Welche Alternativen wurden erwogen?

## Zusätzlicher Kontext

Links, verwandte Issues, Beispiele aus anderen APIs.
