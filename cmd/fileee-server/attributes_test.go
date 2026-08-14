package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/strausmann/go-fileee/fileee"
)

// TestMapDocumentAttributes_MapsAllFields ist der Mapping-Einheitstest aus dem Issue-#37-Brief:
// go-fileee-Extraktionsfelder (fileee.DocumentAttributes) -> Response-Shape
// (documentAttributesBody), Feld für Feld — table-driven über EIN vollständig befülltes Attribut-
// Set, damit ein vergessenes Mapping-Feld sofort auffällt.
func TestMapDocumentAttributes_MapsAllFields(t *testing.T) {
	invoiceDate := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	issueDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	dueDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	payedFalse := false
	readTrue := true
	reviewedFalse := false
	securedFalse := false

	in := fileee.DocumentAttributes{
		Title:            "Rechnung Elektrizität",
		DocumentTypeID:   "bill",
		SenderID:         "company-42",
		ReceiverID:       "contact-7",
		TagIDs:           []string{"tag-1", "tag-2"},
		InvoiceID:        "RE-2026-001",
		InvoiceDate:      &invoiceDate,
		IssueDate:        &issueDate,
		InvoiceDueDate:   &dueDate,
		Amount:           &fileee.Money{Value: 148.75, Currency: "EURO"},
		GrossIncome:      &fileee.Money{Value: 177.02, Currency: "EURO"},
		NetIncome:        &fileee.Money{Value: 148.75, Currency: "EURO"},
		CustomerID:       "KD-9911",
		BankAccount1:     &fileee.BankAccount{IBAN: "DE02120300000000202051", BIC: "BYLADEM1001", Bank: "Sparkasse", AccountHolder: "Max Mustermann"},
		PaymentReference: "RE-2026-001/KD-9911",
		Payed:            &payedFalse,
		ContentLanguage:  "de",
		TotalPageCount:   2,
		MaxPageNr:        2,
		Read:             &readTrue,
		Reviewed:         &reviewedFalse,
		Secured:          &securedFalse,
		// RawExtra MUSS NIEMALS im Mapping landen (Issue #37: "kein roher Passthrough-Dump") —
		// ein unbekannter Schlüssel hier darf in der Ausgabe an keiner Stelle auftauchen.
		RawExtra: map[string]json.RawMessage{"someUnmappedKey": json.RawMessage(`{"value":"secret-internal"}`)},
	}

	got := mapDocumentAttributes(in)

	if got.Title != in.Title {
		t.Errorf("Title = %q, want %q", got.Title, in.Title)
	}
	if got.DocumentType != in.DocumentTypeID {
		t.Errorf("DocumentType = %q, want %q (aus DocumentTypeID)", got.DocumentType, in.DocumentTypeID)
	}
	if got.SenderID != in.SenderID {
		t.Errorf("SenderID = %q, want %q", got.SenderID, in.SenderID)
	}
	if got.ReceiverID != in.ReceiverID {
		t.Errorf("ReceiverID = %q, want %q", got.ReceiverID, in.ReceiverID)
	}
	if len(got.TagIDs) != 2 || got.TagIDs[0] != "tag-1" || got.TagIDs[1] != "tag-2" {
		t.Errorf("TagIDs = %v, want [tag-1 tag-2]", got.TagIDs)
	}
	if got.InvoiceID != in.InvoiceID {
		t.Errorf("InvoiceID = %q, want %q", got.InvoiceID, in.InvoiceID)
	}
	if got.InvoiceDate == nil || !got.InvoiceDate.Equal(invoiceDate) {
		t.Errorf("InvoiceDate = %v, want %v", got.InvoiceDate, invoiceDate)
	}
	if got.IssueDate == nil || !got.IssueDate.Equal(issueDate) {
		t.Errorf("IssueDate = %v, want %v", got.IssueDate, issueDate)
	}
	if got.InvoiceDueDate == nil || !got.InvoiceDueDate.Equal(dueDate) {
		t.Errorf("InvoiceDueDate = %v, want %v", got.InvoiceDueDate, dueDate)
	}
	if got.Amount == nil || *got.Amount != *in.Amount {
		t.Errorf("Amount = %v, want %v", got.Amount, in.Amount)
	}
	if got.GrossIncome == nil || *got.GrossIncome != *in.GrossIncome {
		t.Errorf("GrossIncome = %v, want %v", got.GrossIncome, in.GrossIncome)
	}
	if got.NetIncome == nil || *got.NetIncome != *in.NetIncome {
		t.Errorf("NetIncome = %v, want %v", got.NetIncome, in.NetIncome)
	}
	if got.CustomerID != in.CustomerID {
		t.Errorf("CustomerID = %q, want %q", got.CustomerID, in.CustomerID)
	}
	if got.BankAccount == nil || *got.BankAccount != *in.BankAccount1 {
		t.Errorf("BankAccount = %v, want %v (aus BankAccount1)", got.BankAccount, in.BankAccount1)
	}
	if got.PaymentReference != in.PaymentReference {
		t.Errorf("PaymentReference = %q, want %q", got.PaymentReference, in.PaymentReference)
	}
	if got.Payed == nil || *got.Payed != *in.Payed {
		t.Errorf("Payed = %v, want %v", got.Payed, in.Payed)
	}
	if got.ContentLanguage != in.ContentLanguage {
		t.Errorf("ContentLanguage = %q, want %q", got.ContentLanguage, in.ContentLanguage)
	}
	if got.TotalPageCount != in.TotalPageCount {
		t.Errorf("TotalPageCount = %d, want %d", got.TotalPageCount, in.TotalPageCount)
	}
	if got.MaxPageNr != in.MaxPageNr {
		t.Errorf("MaxPageNr = %d, want %d", got.MaxPageNr, in.MaxPageNr)
	}
	if got.Read == nil || *got.Read != *in.Read {
		t.Errorf("Read = %v, want %v", got.Read, in.Read)
	}
	if got.Reviewed == nil || *got.Reviewed != *in.Reviewed {
		t.Errorf("Reviewed = %v, want %v", got.Reviewed, in.Reviewed)
	}
	if got.Secured == nil || *got.Secured != *in.Secured {
		t.Errorf("Secured = %v, want %v", got.Secured, in.Secured)
	}

	// mapDocumentAttributes hat kein RawExtra-Feld im Zieltyp — der Compiler verhindert bereits ein
	// versehentliches Durchreichen. Zusätzlich: Marshal darf NIRGENDS den RawExtra-Wert enthalten.
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if bytesContains(b, "secret-internal") || bytesContains(b, "someUnmappedKey") {
		t.Fatalf("Marshal(got) enthält RawExtra-Daten (Passthrough-Leak): %s", b)
	}
}

// TestMapDocumentAttributes_EmptyInputYieldsEmptyOutput prüft den Kehrfall: ein leeres
// fileee.DocumentAttributes (Fileee hat für dieses Dokument nichts extrahiert) liefert ein
// vollständig leeres documentAttributesBody — kein Feld wird mit Datenmüll befüllt.
func TestMapDocumentAttributes_EmptyInputYieldsEmptyOutput(t *testing.T) {
	got := mapDocumentAttributes(fileee.DocumentAttributes{})
	want := documentAttributesBody{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mapDocumentAttributes(leer) = %+v, want %+v", got, want)
	}
}

// bytesContains ist ein winziger strings.Contains-Ersatz für []byte, um in diesem Testfile keinen
// zusätzlichen Import (strings) nur für einen einzelnen Aufruf zu brauchen.
func bytesContains(haystack []byte, needle string) bool {
	return string(haystack) != "" && (len(needle) == 0 || indexOf(string(haystack), needle) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestDocumentResponseBody_AttributesOmittedWhenNil prüft die Default-unverändert-Garantie auf
// JSON-Ebene: ein via newDocumentResponseBody gebautes documentResponseBody OHNE gesetztes
// Attributes-Feld marshalt OHNE ein "attributes"-Feld im Body — selbst wenn das zugrundeliegende
// fileee.Document (die Quelle von newDocumentResponseBody) intern Attributes-Daten trägt.
// newDocumentResponseBody kopiert doc.Attributes bewusst NIRGENDS (siehe deren Doku-Kommentar) —
// der einzige sanktionierte Weg von Fileees Rohdaten zum Client ist mapDocumentAttributes.
func TestDocumentResponseBody_AttributesOmittedWhenNil(t *testing.T) {
	doc := fileee.Document{
		ID:     "doc-1",
		Status: fileee.PublicDocumentStatus("CLASSIFIED"),
		Attributes: fileee.DocumentAttributes{
			Title: "sollte nie im JSON auftauchen",
		},
	}
	body := newDocumentResponseBody(doc)

	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(b, &asMap); err != nil {
		t.Fatalf("Re-Unmarshal in map: %v", err)
	}
	if _, ok := asMap["attributes"]; ok {
		t.Fatalf(`Body enthält "attributes", obwohl Attributes nil ist: %s`, b)
	}
	if bytesContains(b, "sollte nie im JSON auftauchen") {
		t.Fatalf("Body enthält das interne fileee.Document.Attributes.Title trotz json:\"-\": %s", b)
	}
	var back map[string]any
	_ = json.Unmarshal(b, &back)
	if back["id"] != "doc-1" {
		t.Fatalf(`Body["id"] = %v, want "doc-1" (bestehende Top-Level-Felder müssen weiter durchgereicht werden)`, back["id"])
	}
}

// TestDocumentResponseBody_AttributesPresentWhenSet ist der Kehrfall: ein gesetztes
// Attributes-Feld erscheint unter dem JSON-Key "attributes", mit den gemappten Werten.
func TestDocumentResponseBody_AttributesPresentWhenSet(t *testing.T) {
	attrs := mapDocumentAttributes(fileee.DocumentAttributes{Title: "Testtitel", DocumentTypeID: "bill"})
	body := newDocumentResponseBody(fileee.Document{ID: "doc-2"})
	body.Attributes = &attrs

	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(b, &asMap); err != nil {
		t.Fatalf("Re-Unmarshal in map: %v", err)
	}
	rawAttrs, ok := asMap["attributes"]
	if !ok {
		t.Fatalf(`Body enthält kein "attributes"-Feld, obwohl Attributes gesetzt ist: %s`, b)
	}
	var decoded documentAttributesBody
	if err := json.Unmarshal(rawAttrs, &decoded); err != nil {
		t.Fatalf("attributes als documentAttributesBody dekodieren: %v", err)
	}
	if decoded.Title != "Testtitel" || decoded.DocumentType != "bill" {
		t.Fatalf("decoded attributes = %+v, want Title=Testtitel DocumentType=bill", decoded)
	}
}

// TestDocumentResponseBody_JSONTagsStayInSyncWithFileeeDocument is a drift guard for the
// deliberate field-mirroring design (see documentResponseBody's "WHY NOT embed" doc comment): it
// compares the JSON tag SET of fileee.Document (excluding its internal, always-suppressed
// Attributes field) against documentResponseBody (excluding the server's own Attributes field). A
// future go-fileee release that adds/renames/removes a Document field would otherwise silently
// stop round-tripping through documentResponseBody without any test failing.
func TestDocumentResponseBody_JSONTagsStayInSyncWithFileeeDocument(t *testing.T) {
	docTags := jsonTagSet(t, reflect.TypeOf(fileee.Document{}), "Attributes")
	bodyTags := jsonTagSet(t, reflect.TypeOf(documentResponseBody{}), "Attributes")

	if !reflect.DeepEqual(docTags, bodyTags) {
		t.Fatalf("JSON-Tag-Mengen weichen ab — go-fileee.Document hat sich geändert, "+
			"documentResponseBody (attributes.go) muss nachgezogen werden.\nfileee.Document: %v\ndocumentResponseBody: %v",
			docTags, bodyTags)
	}
}

// jsonTagSet returns the set of `json:"..."` tag names (the part before any comma, e.g. "id" for
// `json:"id,omitempty"`) of every EXPORTED field of typ, skipping fields whose Go name is listed
// in skip.
func jsonTagSet(t *testing.T, typ reflect.Type, skip ...string) map[string]bool {
	t.Helper()
	skipSet := map[string]bool{}
	for _, s := range skip {
		skipSet[s] = true
	}
	out := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() || skipSet[f.Name] {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		out[name] = true
	}
	return out
}
