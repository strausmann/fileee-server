package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fullAttributesCompanyJSON is the mock-Fileee fixture for the list-companies regression test: a
// Company whose attributes.data carries the same class of PII fileee.CompanyAttributes models
// (IBANs, VAT IDs, emails) — analogous to fullAttributesDocumentJSON
// (handlers_documents_attributes_test.go).
const fullAttributesCompanyJSON = `{
	"id": "company-1", "version": 1, "created": "2026-01-01T00:00:00Z", "modified": "2026-01-01T00:00:00Z",
	"deleted": false, "companyName": "ACME GmbH", "contactType": "COMPANY", "contactStatus": "MANAGED",
	"documentCounter": 3, "connected": true, "fromUserDb": false, "hasLogo": false,
	"attributes": {
		"data": {
			"ibans": {"value": ["DE02120300000000202051"], "type": "LIST", "containedType": "TEXT"},
			"vatIds": {"value": ["DE123456789"], "type": "LIST", "containedType": "TEXT"},
			"emails": {"value": ["billing@acme.example"], "type": "LIST", "containedType": "TEXT"},
			"phoneNumbers": {"value": ["+49 40 1234567"], "type": "LIST", "containedType": "TEXT"}
		}
	}
}`

// TestListDocuments_NeverLeaksAttributes_EvenWithFullUpstreamData is the permanent regression test
// for the CRITICAL security-review finding on PR #38 (see registeredResponseBodyTypes doc comment,
// response_body_safety_test.go): GET /v1/documents (diff/sync mode, no query param) used to return
// []fileee.Document directly, and fileee.Document.MarshalJSON unconditionally re-attaches the full
// attributes.data wire envelope for every element, regardless of the includeAttributes gate that
// only ever applied to GET /v1/documents/{id}. This test feeds the mock-Fileee backend a document
// with EVERY mapped attribute field populated (fullAttributesDocumentJSON,
// handlers_documents_attributes_test.go) through the diff endpoint, with NEITHER the caller opt-in
// NOR the FILEEE_EXPOSE_ATTRIBUTES gate set (the worst case: default server config), and asserts
// the response contains no "attributes" key and none of the known PII values.
func TestListDocuments_NeverLeaksAttributes_EvenWithFullUpstreamData(t *testing.T) {
	routes := map[string]mockRoute{
		"POST /api/documents/rest/diff": {
			Status: http.StatusOK,
			Body:   []byte(`{"rows":[` + fullAttributesDocumentJSON + `],"idsToDelete":[],"totalRows":1,"nextCursor":{}}`),
		},
	}
	_, ts := newTestServer(t, routes) // Default Config: ExposeAttributes=false (zero value)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/documents", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/documents: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}

	assertNoAttributesKeyInItems(t, body)
	assertNoKnownPIIValues(t, body)

	// Positive control: the fixture's non-PII, always-public fields must still round-trip — proves
	// the assertions above are testing "PII absent", not "response empty/broken".
	var decoded documentListBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Body als documentListBody dekodieren: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "doc-attrs" {
		t.Fatalf("Items = %+v, want genau ein Dokument mit id=doc-attrs", decoded.Items)
	}
}

// TestListDocuments_WithQuery_NeverLeaksAttributes is the same regression test for the OTHER branch
// of handleListDocuments (Search+Get-hydration, `?query=` set) — the diff branch and the search
// branch share documentListBody but build Items independently (handlers_documents.go), so both
// need their own coverage; a fix to one branch alone would have left this one leaking.
func TestListDocuments_WithQuery_NeverLeaksAttributes(t *testing.T) {
	routes := map[string]mockRoute{
		"POST /api/documents/rest/query": {
			Status: http.StatusOK,
			Body:   []byte(`{"rows":["doc-attrs"],"totalRows":1}`),
		},
		"GET /api/documents/rest/doc-attrs": {
			Status: http.StatusOK,
			Body:   []byte(fullAttributesDocumentJSON),
		},
	}
	_, ts := newTestServer(t, routes)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/documents?query=rechnung", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/documents?query=rechnung: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}

	assertNoAttributesKeyInItems(t, body)
	assertNoKnownPIIValues(t, body)
}

// TestListCompanies_NeverLeaksAttributes is the permanent regression test for the second finding
// from the same security audit (pre-existing, unrelated to Issue #37): GET /v1/companies used
// entityListBody[fileee.Company] directly — same MarshalJSON-bypasses-json-dash-tag mechanism as
// fileee.Document, leaking fileee.CompanyAttributes (IBANs, VAT IDs, emails, phone numbers,
// websites, German tax IDs) on every call, completely ungated (there never was an opt-in for
// company attributes — they were simply always meant to stay internal, per the `json:"-"` tag that
// Company.MarshalJSON silently overrode).
func TestListCompanies_NeverLeaksAttributes(t *testing.T) {
	routes := map[string]mockRoute{
		"POST /api/companies/rest/query": {
			Status: http.StatusOK,
			Body:   []byte(`{"rows":[` + fullAttributesCompanyJSON + `],"totalRows":1}`),
		},
	}
	_, ts := newTestServer(t, routes)

	req := newAuthedRequest(t, http.MethodGet, ts.URL+"/v1/companies", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/companies: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Body lesen: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}

	assertNoAttributesKeyInItems(t, body)
	for _, pii := range []string{"DE02120300000000202051", "DE123456789", "billing@acme.example", "+49 40 1234567"} {
		if strings.Contains(string(body), pii) {
			t.Fatalf("LEAK: company PII %q present in GET /v1/companies response: %s", pii, body)
		}
	}

	var decoded companyListBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Body als companyListBody dekodieren: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "company-1" || decoded.Items[0].CompanyName != "ACME GmbH" {
		t.Fatalf("Items = %+v, want genau eine Firma id=company-1 companyName=\"ACME GmbH\"", decoded.Items)
	}
}

// assertNoAttributesKeyInItems decodes body as a generic {"items":[...]} shape and fails if ANY
// item object carries an "attributes" key — used by the list regression tests above (documents AND
// companies share this exact failure mode).
func assertNoAttributesKeyInItems(t *testing.T, body []byte) {
	t.Helper()
	var generic struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatalf("Re-Unmarshal in generische {items:[...]}-Form: %v", err)
	}
	if len(generic.Items) == 0 {
		t.Fatal("Items ist leer — Test-Fixture liefert nichts zu prüfen (Testaufbau prüfen)")
	}
	for i, item := range generic.Items {
		if _, ok := item["attributes"]; ok {
			t.Fatalf(`LEAK: items[%d] enthält "attributes": %s`, i, body)
		}
	}
}

// assertNoKnownPIIValues fails if body contains any of the specific PII values carried by
// fullAttributesDocumentJSON (handlers_documents_attributes_test.go) — a value-level check in
// addition to the structural "no attributes key" check, in case a future change routes the same
// data out under a different key name.
func assertNoKnownPIIValues(t *testing.T, body []byte) {
	t.Helper()
	for _, pii := range []string{
		"DE02120300000000202051", // IBAN
		"BYLADEM1001",            // BIC
		"KD-9911",                // Kundennummer
		"RE-2026-001",            // Rechnungsnummer
		"company-42",             // SenderID
	} {
		if strings.Contains(string(body), pii) {
			t.Fatalf("LEAK: document PII %q present in response: %s", pii, body)
		}
	}
}
