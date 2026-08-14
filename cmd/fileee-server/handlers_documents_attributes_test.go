package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fullAttributesDocumentJSON is the mock-Fileee fixture reused by every test in this file: a
// Document whose attributes.data covers ALL fields mapDocumentAttributes maps (attributes.go),
// wire-encoded per go-fileee's documented attribute-wrapper form (fileee/types_test.go
// TestDocumentAttributesUnmarshalGruppenAttributeMitVerschachteltenUnterfeldern — group attributes
// nest a full attribute-wrapper per sub-field; simple attributes carry {"value":...,"type":...}).
const fullAttributesDocumentJSON = `{
	"id": "doc-attrs", "version": 1, "status": "CLASSIFIED", "type": "Document",
	"pages": [{"id": "page-1", "imageVersion": 1, "contentVersion": 1}],
	"uploadAttribute": {"originalFileName": "invoice.pdf", "newUpload": true},
	"sharedSpaceIds": [],
	"attributes": {
		"data": {
			"title": {"value": "Rechnung Elektrizität", "type": "TEXT"},
			"documentTypeId": {"value": "bill", "type": "TEXT"},
			"senderId": {"value": "company-42", "type": "TEXT"},
			"receiverId": {"value": "contact-7", "type": "TEXT"},
			"tagIds": {"value": ["tag-1", "tag-2"], "type": "LIST", "containedType": "TEXT"},
			"invoiceId": {"value": "RE-2026-001", "type": "TEXT"},
			"invoiceDate": {"value": "2026-07-15", "type": "DATE"},
			"issueDate": {"value": "2026-07-10", "type": "DATE"},
			"invoiceDueDate": {"value": "2026-08-01", "type": "DATE"},
			"amount": {"type": "COMPOSED", "attributeGroup": "COMPOSED", "data": {
				"currency": {"type": "ENUMERATION", "value": "EURO"},
				"value": {"type": "DOUBLE", "value": 148.75}
			}},
			"grossIncome": {"type": "COMPOSED", "attributeGroup": "COMPOSED", "data": {
				"currency": {"type": "ENUMERATION", "value": "EURO"},
				"value": {"type": "DOUBLE", "value": 177.02}
			}},
			"netIncome": {"type": "COMPOSED", "attributeGroup": "COMPOSED", "data": {
				"currency": {"type": "ENUMERATION", "value": "EURO"},
				"value": {"type": "DOUBLE", "value": 148.75}
			}},
			"customerId": {"value": "KD-9911", "type": "TEXT"},
			"bankAccount1": {"type": "COMPOSED", "attributeGroup": "COMPOSED", "data": {
				"iban": {"type": "TEXT", "value": "DE02120300000000202051"},
				"bic": {"type": "TEXT", "value": "BYLADEM1001"},
				"bank": {"type": "TEXT", "value": "Sparkasse"},
				"account_holder": {"type": "TEXT", "value": "Max Mustermann"}
			}},
			"paymentReference": {"value": "RE-2026-001/KD-9911", "type": "TEXT"},
			"payed": {"value": false, "type": "BOOLEAN"},
			"contentLanguage": {"value": "de", "type": "TEXT"},
			"totalPageCount": {"value": 2, "type": "INTEGER"},
			"maxPageNr": {"value": 2, "type": "INTEGER"},
			"read": {"value": true, "type": "BOOLEAN"},
			"reviewed": {"value": false, "type": "BOOLEAN"},
			"secured": {"value": false, "type": "BOOLEAN"}
		}
	}
}`

// doGetDocument issues an authenticated GET /v1/documents/{id} against ts, optionally with
// ?includeAttributes=true, and returns the response (caller closes the body).
func doGetDocument(t *testing.T, ts string, id string, includeAttributes bool) *http.Response {
	t.Helper()
	url := ts + "/v1/documents/" + id
	if includeAttributes {
		url += "?includeAttributes=true"
	}
	req := newAuthedRequest(t, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// TestGetDocument_BackendError_StillMapped stellt sicher, dass die Issue-#37-Umstellung von
// handleGetDocument (Gate-Check + newDocumentResponseBody statt der reinen fileee.Document-
// Rückgabe) den bestehenden Fehler-Pfad NICHT verändert hat: ein 404 vom Fileee-Backend läuft
// weiterhin unverändert durch mapError (fileee.ErrNotFound → 404/"not_found", errors.go).
func TestGetDocument_BackendError_StillMapped(t *testing.T) {
	routes := map[string]mockRoute{
		"GET /api/documents/rest/doc-missing": {
			Status: http.StatusNotFound,
			Body:   []byte(`{"apiError":"NOT_FOUND","errorMessage":"document not found"}`),
		},
	}
	_, ts := newTestServer(t, routes)

	resp := doGetDocument(t, ts.URL, "doc-missing", false)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 404, body=%s", resp.StatusCode, body)
	}
	var errBody statusErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("Body als {error,code} dekodieren: %v", err)
	}
	if errBody.Code != "not_found" {
		t.Fatalf("code = %q, want not_found", errBody.Code)
	}
}

// TestGetDocument_IncludeAttributes_GateOff_Returns403 ist der erste TDD-Pflichtfall aus dem
// Issue-#37-Brief: Gate aus (Default, FILEEE_EXPOSE_ATTRIBUTES unset) + includeAttributes=true →
// 403, OHNE dass überhaupt ein Fileee-Upstream-Roundtrip versucht wird (routes bewusst leer/nil —
// ein nicht registrierter Mock-Pfad würde den Test mit einem Verbindungsfehler statt einem
// aussagekräftigen Assert scheitern lassen, käme der Handler doch bis zu Documents.Get durch).
func TestGetDocument_IncludeAttributes_GateOff_Returns403(t *testing.T) {
	_, ts := newTestServer(t, nil)

	resp := doGetDocument(t, ts.URL, "doc-attrs", true)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 403, body=%s", resp.StatusCode, body)
	}

	var errBody statusErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("Body als {error,code} dekodieren: %v", err)
	}
	if errBody.Code != "attributes_disabled" {
		t.Fatalf("code = %q, want %q", errBody.Code, "attributes_disabled")
	}
}

// statusErrorBody spiegelt das {error,code}-Schema aus errors.go (statusError) für Test-Decodes —
// eigenständig definiert, da statusError selbst unexportierte Felder hat und aus package main
// heraus (hier: package main, aber andere Datei) trotzdem nicht direkt als Decode-Ziel taugt (die
// JSON-Feldnamen sind exportiert, das reicht für einen eigenen, lokalen Decode-Typ).
type statusErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// TestGetDocument_IncludeAttributes_GateOn_ReturnsAllMappedFields ist der zweite TDD-Pflichtfall:
// Gate an + includeAttributes=true → 200, "attributes" ist gesetzt und enthält JEDES von
// mapDocumentAttributes gemappte Feld mit dem aus fullAttributesDocumentJSON erwarteten Wert
// (Mapping-Verifikation Ende-zu-Ende über den echten HTTP-Pfad, nicht nur die Go-Einheit in
// attributes_test.go).
func TestGetDocument_IncludeAttributes_GateOn_ReturnsAllMappedFields(t *testing.T) {
	routes := map[string]mockRoute{
		"GET /api/documents/rest/doc-attrs": {Status: http.StatusOK, Body: []byte(fullAttributesDocumentJSON)},
	}
	cfg := Config{ExposeAttributes: true}
	_, ts := newTestServerWithConfig(t, cfg, routes)

	resp := doGetDocument(t, ts.URL, "doc-attrs", true)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}

	var out getDocumentOutput
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	if err := json.Unmarshal(body, &out.Body); err != nil {
		t.Fatalf("Body als documentResponseBody dekodieren: %v\nbody=%s", err, body)
	}

	if out.Body.ID != "doc-attrs" {
		t.Errorf("id = %q, want doc-attrs (bestehende Top-Level-Felder unverändert)", out.Body.ID)
	}

	attrs := out.Body.Attributes
	if attrs == nil {
		t.Fatalf("attributes ist nil, obwohl includeAttributes=true + Gate an: body=%s", body)
	}

	// Betreiber-priorisierte Kernfelder für eine Fileee→Paperless-Migration (Absender,
	// Rechnungsnummer, Rechnungsdatum) — explizit einzeln geprüft, nicht nur implizit über die
	// Vollständigkeitsprüfung unten.
	if attrs.SenderID != "company-42" {
		t.Errorf("SenderID (Absender) = %q, want company-42", attrs.SenderID)
	}
	if attrs.InvoiceID != "RE-2026-001" {
		t.Errorf("InvoiceID (Rechnungsnummer) = %q, want RE-2026-001", attrs.InvoiceID)
	}
	if attrs.InvoiceDate == nil || attrs.InvoiceDate.Format("2006-01-02") != "2026-07-15" {
		t.Errorf("InvoiceDate (Rechnungsdatum) = %v, want 2026-07-15", attrs.InvoiceDate)
	}
	// Rechnungsnummer und Kundennummer dürfen NIEMALS verwechselt werden (Betreiber-Vorgabe) —
	// beide gleichzeitig auf ihren jeweils EIGENEN erwarteten Wert prüfen.
	if attrs.CustomerID != "KD-9911" {
		t.Errorf("CustomerID (Kundennummer) = %q, want KD-9911 (nicht die Rechnungsnummer)", attrs.CustomerID)
	}
	if attrs.InvoiceID == attrs.CustomerID {
		t.Fatalf("InvoiceID und CustomerID sind identisch (%q) — Verwechslungsgefahr nicht ausgeschlossen", attrs.InvoiceID)
	}

	// Die übrigen gemappten Felder — Rest der Vollständigkeitsprüfung.
	if attrs.Title != "Rechnung Elektrizität" {
		t.Errorf("Title = %q, want \"Rechnung Elektrizität\"", attrs.Title)
	}
	if attrs.DocumentType != "bill" {
		t.Errorf("DocumentType = %q, want bill", attrs.DocumentType)
	}
	if attrs.ReceiverID != "contact-7" {
		t.Errorf("ReceiverID = %q, want contact-7", attrs.ReceiverID)
	}
	if len(attrs.TagIDs) != 2 || attrs.TagIDs[0] != "tag-1" || attrs.TagIDs[1] != "tag-2" {
		t.Errorf("TagIDs = %v, want [tag-1 tag-2]", attrs.TagIDs)
	}
	if attrs.IssueDate == nil || attrs.IssueDate.Format("2006-01-02") != "2026-07-10" {
		t.Errorf("IssueDate = %v, want 2026-07-10", attrs.IssueDate)
	}
	if attrs.InvoiceDueDate == nil || attrs.InvoiceDueDate.Format("2006-01-02") != "2026-08-01" {
		t.Errorf("InvoiceDueDate = %v, want 2026-08-01", attrs.InvoiceDueDate)
	}
	if attrs.Amount == nil || attrs.Amount.Value != 148.75 || attrs.Amount.Currency != "EURO" {
		t.Errorf("Amount = %v, want {148.75 EURO}", attrs.Amount)
	}
	if attrs.GrossIncome == nil || attrs.GrossIncome.Value != 177.02 {
		t.Errorf("GrossIncome = %v, want Value=177.02", attrs.GrossIncome)
	}
	if attrs.NetIncome == nil || attrs.NetIncome.Value != 148.75 {
		t.Errorf("NetIncome = %v, want Value=148.75", attrs.NetIncome)
	}
	if attrs.BankAccount == nil || attrs.BankAccount.IBAN != "DE02120300000000202051" || attrs.BankAccount.BIC != "BYLADEM1001" {
		t.Errorf("BankAccount = %v, want IBAN=DE02120300000000202051 BIC=BYLADEM1001", attrs.BankAccount)
	}
	if attrs.PaymentReference != "RE-2026-001/KD-9911" {
		t.Errorf("PaymentReference = %q, want RE-2026-001/KD-9911", attrs.PaymentReference)
	}
	if attrs.Payed == nil || *attrs.Payed != false {
		t.Errorf("Payed = %v, want false", attrs.Payed)
	}
	if attrs.ContentLanguage != "de" {
		t.Errorf("ContentLanguage = %q, want de", attrs.ContentLanguage)
	}
	if attrs.TotalPageCount != 2 {
		t.Errorf("TotalPageCount = %d, want 2", attrs.TotalPageCount)
	}
	if attrs.MaxPageNr != 2 {
		t.Errorf("MaxPageNr = %d, want 2", attrs.MaxPageNr)
	}
	if attrs.Read == nil || *attrs.Read != true {
		t.Errorf("Read = %v, want true", attrs.Read)
	}
	if attrs.Reviewed == nil || *attrs.Reviewed != false {
		t.Errorf("Reviewed = %v, want false", attrs.Reviewed)
	}
	if attrs.Secured == nil || *attrs.Secured != false {
		t.Errorf("Secured = %v, want false", attrs.Secured)
	}
}

// TestGetDocument_IncludeAttributes_GateOn_NoParam_AttributesAbsent prüft den dritten
// TDD-Pflichtfall: Gate an, aber KEIN includeAttributes-Parameter → 200, KEIN "attributes"-Feld im
// Body. Das Gate allein schaltet nichts frei — es braucht IMMER zusätzlich den expliziten
// Aufrufer-Parameter.
func TestGetDocument_IncludeAttributes_GateOn_NoParam_AttributesAbsent(t *testing.T) {
	routes := map[string]mockRoute{
		"GET /api/documents/rest/doc-attrs": {Status: http.StatusOK, Body: []byte(fullAttributesDocumentJSON)},
	}
	cfg := Config{ExposeAttributes: true}
	_, ts := newTestServerWithConfig(t, cfg, routes)

	resp := doGetDocument(t, ts.URL, "doc-attrs", false)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	assertNoAttributesKey(t, body)
}

// TestGetDocument_Default_Unchanged ist der vierte TDD-Pflichtfall: weder Parameter noch Gate
// gesetzt (der bisherige Aufrufstil vor Issue #37) → 200, Body unverändert — insbesondere KEIN
// "attributes"-Feld, obwohl der Mock-Fileee dieselbe, vollständig befüllte attributes.data liefert
// wie in den Gate-on-Tests. Der Default-Fall darf sich durch dieses Feature in KEINER Weise
// geändert haben.
func TestGetDocument_Default_Unchanged(t *testing.T) {
	routes := map[string]mockRoute{
		"GET /api/documents/rest/doc-attrs": {Status: http.StatusOK, Body: []byte(fullAttributesDocumentJSON)},
	}
	_, ts := newTestServer(t, routes)

	resp := doGetDocument(t, ts.URL, "doc-attrs", false)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	assertNoAttributesKey(t, body)

	var back map[string]any
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("Re-Unmarshal in map: %v", err)
	}
	for _, field := range []string{"id", "version", "status", "pages", "uploadAttribute", "sharedSpaceIds"} {
		if _, ok := back[field]; !ok {
			t.Errorf("Body fehlt bestehendes Feld %q — Default-Verhalten hat sich geändert: %s", field, body)
		}
	}
}

// assertNoAttributesKey scheitert, wenn body ein Top-Level-JSON-Feld "attributes" enthält.
func assertNoAttributesKey(t *testing.T, body []byte) {
	t.Helper()
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &asMap); err != nil {
		t.Fatalf("Re-Unmarshal in map: %v", err)
	}
	if _, ok := asMap["attributes"]; ok {
		t.Fatalf(`Body enthält "attributes", obwohl es nicht angefordert/freigeschaltet wurde: %s`, body)
	}
}

// TestOpenAPIJSON_ReflectsAttributesOptIn ist die Issue-#37-Akzeptanzkriterium-Prüfung "OpenAPI
// reflektiert die neue Opt-in-Form": /openapi.json muss sowohl den includeAttributes-Query-
// Parameter der get-document-Operation als auch die documentAttributesBody-Feldnamen im
// referenzierten Schema enthalten.
func TestOpenAPIJSON_ReflectsAttributesOptIn(t *testing.T) {
	_, ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	spec := string(body)

	if !strings.Contains(spec, "includeAttributes") {
		t.Errorf("OpenAPI enthält keinen includeAttributes-Parameter: fehlt in %s", "/openapi.json")
	}
	for _, want := range []string{`"documentType"`, `"senderId"`, `"invoiceId"`, `"invoiceDate"`, `"bankAccount"`} {
		if !strings.Contains(spec, want) {
			t.Errorf("OpenAPI-Schema enthält kein Feld %s", want)
		}
	}
}
