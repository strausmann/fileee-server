# ADR-0010: DTO-Leak-Guardrail wird aus der echten `huma.Register`-Verdrahtung abgeleitet

**Status:** proposed
**Datum:** 2026-08-15
**Ersetzt:** —
**Ersetzt durch:** —
**Verwandt:** `.claude/rules/api-response-dto-boundary.md` (homelab-management-Repo), [ADR-0009](0009-upstream-timeout.md)

## Kontext

Im Review von PR #42 (Gegenprobe, empirisch belegt gegen `huma@v2.39.1`
`SchemaLinkTransformer.Transform` in `transforms.go`) kam eine strukturelle Lücke im
PII-Leak-Sicherheitsnetz zutage — kein Defekt in #42 selbst, aber die Tests versprachen mehr
Schutz, als sie liefern. Es ist der zweite Beinahe-Fehler dieser Klasse nach Issue #37/PR #38.

**Zwei Probleme:**

1. **HTTP-Regressionstests sind blind für Top-Level-Marshaler-Leaks.** `huma.DefaultConfig`
   registriert `SchemaLinkTransformer`, der für JEDEN Response aus dem TOP-LEVEL-Body-Typ einen
   frischen, methodenlosen Klon via `reflect.StructOf` baut (um `$schema` zu injizieren). Ein
   methodenloser Klon implementiert `json.Marshaler` nicht — ein `MarshalJSON` (das PII anhängt)
   feuert nie, wenn der getaintete Typ der Top-Level-Body ist. Bei VERSCHACHTELTEN Typen
   (`documentListBody.Items []documentResponseBody`) feuert `MarshalJSON` weiterhin (der Klon ist
   nur eine Ebene tief). Per Gegenprobe verifiziert: `handleGetCompany` so gepatcht, dass es
   `fileee.Company` direkt (Top-Level) zurückgibt → `TestGetCompany_NeverLeaksAttributes` bleibt
   GRÜN, fängt den Leak NICHT.
2. **Der strukturelle Guardrail (`response_body_safety_test.go`,
   `TestNoFileeeMarshalerTypeInAnyResponseBody`) prüfte eine handgepflegte
   `registeredResponseBodyTypes`-Liste**, nicht die echte Verdrahtung. Ein künftiges „Aufräumen",
   das `fileee.Company` direkt in `getCompanyOutput.Body` inlined, würde durch BEIDE Tests
   rutschen (der HTTP-Test maskiert es wie oben beschrieben, der Guardrail prüft die alte
   Handliste statt des tatsächlichen Handler-Rückgabetyps). Manuelle-Listen-Drift war schon bei
   Issue #37/#38 der Beinahe-Treffer.

Vollständiger Issue-Text: [#43](https://github.com/strausmann/fileee-server/issues/43).

## Entscheidung

1. **`registerOperation[I, O any]` ersetzt `huma.Register` als EINZIGEN sanktionierten
   Registrierungsweg.** `operation_registry.go` definiert einen Drop-in-Wrapper mit identischem
   Verhalten (delegiert 1:1 an `huma.Register`), der zusätzlich — nur wenn ein Test-Hook
   (`operationBodyTypeRecorder`, im Normalbetrieb `nil`, keine Laufzeitkosten) gesetzt ist — den
   Go-Typ des Response-Bodys jeder Operation meldet. Alle 42 bestehenden `huma.Register`-Aufrufe
   (6 Dateien: `handlers_documents.go`, `handlers_entities.go`, `handlers_share.go`,
   `handlers_conversations.go`, `handlers_destructive.go`, `resolve.go`) wurden mechanisch auf
   `registerOperation` umgestellt.
2. **`registeredResponseBodyTypesFromRealServer` (response_body_safety_test.go) ersetzt die
   handgepflegte Liste.** Der Test baut einen echten `*Server` (mit `AllowDestructive:true`, damit
   auch die drei bedingt registrierten Hard-DELETE-Routen erfasst werden), liest während dessen
   Aufbau über den Recorder-Hook exakt die Typen ab, die tatsächlich registriert wurden, und füttert
   damit denselben Type-Walk (`findFileeeMarshalerTypes`) wie zuvor. Eine neue oder geänderte Route
   ist damit automatisch erfasst — nichts mehr zu pflegen.
3. **Gegenprobe als Abnahmekriterium, nicht als Dauertest.** Vor dem Commit wurde `fileee.Company`
   probeweise als Top-Level-Body von `handleGetCompany` inlined: der neue, registrierungs-
   abgeleitete Guardrail ging korrekt ROT (`TestNoFileeeMarshalerTypeInAnyResponseBody/fileee.Company`),
   während `TestGetCompany_NeverLeaksAttributes` erwartungsgemäß GRÜN blieb (Beleg für Problem 1
   oben). Der Patch wurde danach vollständig zurückgesetzt — er bleibt kein Teil des Codebase, nur
   der Nachweis, dass der neue Guardrail wirkt.
4. **Irreführender Doc-Kommentar korrigiert.** `TestGetCompany_NeverLeaksAttributes`
   (`pii_leak_regression_test.go`) behauptete implizit „regression-proof" für den Top-Level-Fall.
   Der Kommentar beschreibt jetzt explizit, was der Test beweist (aktueller Output sauber) und was
   NICHT (kein eigenständiger Schutz gegen eine künftige Top-Level-Regression — das übernimmt der
   strukturelle Guardrail).

## Konsequenzen

**Positiv:**
- Eine künftige Route, die versehentlich einen go-fileee-Marshaler-Typ direkt (auch als
  Top-Level-Body) zurückgibt, wird beim nächsten `go test ./...` gefangen — unabhängig davon, ob
  jemand daran gedacht hat, eine Liste zu pflegen.
- Die Gegenprobe ist empirisch, nicht behauptet: der Beweis, dass der neue Mechanismus wirkt (und
  der alte HTTP-Test allein es nicht getan hätte), liegt im PR-Testnachweis.
- `registerOperation` ist für alle 42 bestehenden Aufrufstellen ein reiner Drop-in — keine
  Verhaltensänderung an der eigentlichen API, nur an der Test-Infrastruktur.

**Negativ / bewusst in Kauf genommen:**
- Jede KÜNFTIGE Route MUSS über `registerOperation` (nicht `huma.Register` direkt) registriert
  werden, sonst greift der Guardrail für sie nicht — eine Konvention, die nirgends vom Compiler
  erzwungen wird (beide Funktionen haben identische Signaturform). Ein Code-Review-Punkt, kein
  automatischer Schutz.
- Der Recorder-Hook (`operationBodyTypeRecorder`) ist globaler, veränderlicher Package-Zustand —
  bewusst auf `nil` im Normalbetrieb beschränkt und nur für die Dauer eines einzelnen
  `Handler()`-Aufbaus innerhalb EINES Tests gesetzt (sequentielle Testausführung in diesem Paket,
  kein `t.Parallel()` — verifiziert vor dieser Entscheidung). Bei künftiger Parallelisierung der
  Testsuite müsste dieser Mechanismus überarbeitet werden.

## Referenzen

- Issue: [#43](https://github.com/strausmann/fileee-server/issues/43)
- Vorgänger-Vorfall: Issue #37 / PR #38 (documentListBody-Leak, Ursprung des gesamten
  DTO-Boundary-Sicherheitsnetzes)
- `.claude/rules/api-response-dto-boundary.md` (homelab-management-Repo) — verlangt genau diesen
  strukturellen, aus der echten Registrierung abgeleiteten Guardrail
- [ADR-0008](0008-fileee-server.md) (fileee-server-Grundarchitektur)
