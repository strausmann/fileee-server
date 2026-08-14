package main

import (
	"time"

	"github.com/strausmann/go-fileee/fileee"
)

// documentAttributesBody is the typed, PII-carrying projection of go-fileee's
// fileee.DocumentAttributes (the wire path `attributes.data`, see fileee/types.go) that
// GET /v1/documents/{id} MAY expose behind the `includeAttributes` opt-in query parameter (see
// getDocumentInput) AND the FILEEE_EXPOSE_ATTRIBUTES gate (Config.ExposeAttributes). It is a
// deliberate, field-for-field mapping (see mapDocumentAttributes) — NOT a raw passthrough of
// fileee.DocumentAttributes.RawExtra (Issue #37: "kein roher Passthrough-Dump"). Unmapped/unknown
// extraction keys (RawExtra) stay server-internal and are never surfaced through this type.
//
// Every field carries `omitempty`: Fileee only ever populates the subset of attributes it managed
// to extract from a given document, so an empty/zero Go value here means "Fileee did not extract
// this field" — not "the field is unexpectedly missing from the schema".
//
// For a Fileee → DMS metadata-carrying migration (Issue #37 motivation), three fields matter most
// and are ALL modeled by go-fileee's DocumentAttributes and therefore exposed here — no gap found
// on the go-fileee side for this feature: SenderID (→ e.g. Paperless-ngx correspondent), InvoiceID
// (→ e.g. Paperless-ngx ASN/custom field; do NOT confuse with CustomerID, a separate field), and
// InvoiceDate (→ e.g. Paperless-ngx `created`). Fileee itself derives a document's title from
// sender + recognized invoice number — Title, SenderID and InvoiceID are therefore related but
// distinct fields, all exposed independently below.
type documentAttributesBody struct {
	Title            string              `json:"title,omitempty" doc:"Von Fileee erkannter/gesetzter Dokumenttitel (von Fileee typischerweise aus Absender + Rechnungsnummer gebildet)."`
	DocumentType     string              `json:"documentType,omitempty" doc:"Fileees Auto-Klassifizierung (documentTypeId), z. B. \"bill\"."`
	SenderID         string              `json:"senderId,omitempty" doc:"ID des erkannten Absenders (Company oder Contact — siehe GET /v1/companies, GET /v1/contacts). Migrations-relevant: → z. B. Paperless-ngx correspondent."`
	ReceiverID       string              `json:"receiverId,omitempty" doc:"ID des erkannten Empfängers (Company oder Contact)."`
	TagIDs           []string            `json:"tagIds,omitempty" doc:"IDs der dem Dokument zugewiesenen Tags (siehe GET /v1/tags)."`
	InvoiceID        string              `json:"invoiceId,omitempty" doc:"Extrahierte Rechnungsnummer (NICHT die Kundennummer, siehe customerId). Migrations-relevant: → z. B. Paperless-ngx ASN/Custom-Field."`
	InvoiceDate      *time.Time          `json:"invoiceDate,omitempty" doc:"Extrahiertes Rechnungsdatum. Migrations-relevant: → z. B. Paperless-ngx created."`
	IssueDate        *time.Time          `json:"issueDate,omitempty" doc:"Extrahiertes Ausstellungsdatum."`
	InvoiceDueDate   *time.Time          `json:"invoiceDueDate,omitempty" doc:"Extrahiertes Fälligkeitsdatum."`
	Amount           *fileee.Money       `json:"amount,omitempty" doc:"Extrahierter Rechnungsbetrag."`
	GrossIncome      *fileee.Money       `json:"grossIncome,omitempty" doc:"Extrahierter Bruttobetrag."`
	NetIncome        *fileee.Money       `json:"netIncome,omitempty" doc:"Extrahierter Nettobetrag."`
	CustomerID       string              `json:"customerId,omitempty" doc:"Extrahierte Kundennummer."`
	BankAccount      *fileee.BankAccount `json:"bankAccount,omitempty" doc:"Extrahierte Bankverbindung (IBAN/BIC/Bank/Kontoinhaber)."`
	PaymentReference string              `json:"paymentReference,omitempty" doc:"Extrahierter Zahlungs-/Verwendungszweck."`
	Payed            *bool               `json:"payed,omitempty" doc:"Ob das Dokument als bezahlt markiert ist."`
	ContentLanguage  string              `json:"contentLanguage,omitempty" doc:"Von Fileee erkannte Dokumentsprache."`
	TotalPageCount   int                 `json:"totalPageCount,omitempty" doc:"Von Fileee erkannte Gesamtseitenzahl."`
	MaxPageNr        int                 `json:"maxPageNr,omitempty" doc:"Höchste von Fileee erkannte Seitenzahl."`
	Read             *bool               `json:"read,omitempty" doc:"Ob das Dokument als gelesen markiert ist."`
	Reviewed         *bool               `json:"reviewed,omitempty" doc:"Ob das Dokument als geprüft markiert ist."`
	Secured          *bool               `json:"secured,omitempty" doc:"Ob das Dokument als gesichert markiert ist."`
}

// mapDocumentAttributes projects go-fileee's fileee.DocumentAttributes (attributes.data) onto the
// server's own typed, PII-aware response shape (documentAttributesBody). It is a deliberate
// field-for-field mapping — a.RawExtra (unmapped/unknown extraction keys) is intentionally NEVER
// surfaced (Issue #37: "kein roher Passthrough-Dump").
func mapDocumentAttributes(a fileee.DocumentAttributes) documentAttributesBody {
	return documentAttributesBody{
		Title:            a.Title,
		DocumentType:     a.DocumentTypeID,
		SenderID:         a.SenderID,
		ReceiverID:       a.ReceiverID,
		TagIDs:           a.TagIDs,
		InvoiceID:        a.InvoiceID,
		InvoiceDate:      a.InvoiceDate,
		IssueDate:        a.IssueDate,
		InvoiceDueDate:   a.InvoiceDueDate,
		Amount:           a.Amount,
		GrossIncome:      a.GrossIncome,
		NetIncome:        a.NetIncome,
		CustomerID:       a.CustomerID,
		BankAccount:      a.BankAccount1,
		PaymentReference: a.PaymentReference,
		Payed:            a.Payed,
		ContentLanguage:  a.ContentLanguage,
		TotalPageCount:   a.TotalPageCount,
		MaxPageNr:        a.MaxPageNr,
		Read:             a.Read,
		Reviewed:         a.Reviewed,
		Secured:          a.Secured,
	}
}

// documentResponseBody is the response body shared by GET/POST/PUT /v1/documents/{id}
// (get-document, upload-document, update-document — see handlers_documents.go). Its fields
// deliberately MIRROR fileee.Document field-for-field (same JSON tags, no embedding) plus the
// server's own Attributes field — see newDocumentResponseBody for the mapping and the important
// "why not embed fileee.Document" note below.
//
// WHY NOT `fileee.Document` embedded anonymously (tried first, reverted — Issue #37 PR review):
// fileee.Document carries its own `MarshalJSON`/`UnmarshalJSON` (fileee/types.go) that reconstructs
// the wire envelope `{"attributes":{"data":{...}}}` from its otherwise `json:"-"`-tagged Attributes
// field. Huma's `SchemaLinkTransformer` (huma v2.39.1, transforms.go) clones EVERY response type
// via `reflect.StructOf` to inject a `$schema` field — and `reflect.StructOf` panics for a struct
// whose FIRST field is an anonymous field with methods only when that anonymous field is NOT
// actually first in the CLONED type ("reflect: embedded type with methods not implemented if type
// is not first field") — which it never is, because huma always prepends its own synthetic
// `$schema` field ahead of the caller's fields. Huma catches the panic and silently falls back to
// marshaling the ORIGINAL (non-cloned) struct value directly — which DOES still have the promoted
// `Document.MarshalJSON` method, and that method knows NOTHING about a sibling `Attributes` field
// on an outer wrapper struct. Empirically verified (probe test against a real httptest.Server
// through the full Handler() pipeline, not just direct json.Marshal in a unit test): an anonymous-
// embedding design silently ignored this server's own `Attributes` field and instead leaked
// fileee's RAW, unmapped wire envelope (`attributes.data.<key>.value`, including any `RawExtra`)
// straight through — the opposite of the PII-gated, typed-projection contract Issue #37 requires.
//
// Mirroring the fields explicitly (no embedding, no custom Marshaler on this type) sidesteps the
// whole SchemaLinkTransformer/reflect.StructOf interaction entirely: plain tagged fields marshal
// identically whether or not huma's clone succeeds, which is exactly why this is also the SAME
// mechanism that already made the pre-Issue-#37 code's `Attributes DocumentAttributes json:"-"`
// field invisible by default (fileee.Document itself has no anonymous fields, so its own
// schema-link clone never panics and plain tag-based marshaling already applied there).
//
// Attributes stays nil (and is therefore omitted from the JSON body via `omitempty`) unless the
// caller requested it AND the operator enabled the PII gate — see handleGetDocument. Upload/Update
// never populate it (Issue #37 scope is GET /v1/documents/{id} only).
type documentResponseBody struct {
	ID               string                      `json:"id"`
	Version          int64                       `json:"version"`
	Created          time.Time                   `json:"created"`
	Modified         time.Time                   `json:"modified"`
	Deleted          bool                        `json:"deleted"`
	Status           fileee.PublicDocumentStatus `json:"status"`
	Type             string                      `json:"type"`
	Pages            []fileee.Page               `json:"pages"`
	UploadAttribute  fileee.UploadAttribute      `json:"uploadAttribute"`
	SharedSpaceIDs   []string                    `json:"sharedSpaceIds"`
	ShareInformation fileee.ShareInformation     `json:"shareInformation"`
	ForbiddenActions []fileee.DocumentAction     `json:"forbiddenActions"`
	Attributes       *documentAttributesBody     `json:"attributes,omitempty" doc:"Fileees automatisch extrahierte Indexierungs-Metadaten (attributes.data) — NUR gesetzt bei GET /v1/documents/{id}?includeAttributes=true UND aktiviertem FILEEE_EXPOSE_ATTRIBUTES-Gate. Enthält private Finanz-PII (Rechnungsdaten, IBAN, Kundennummer, Absender/Empfänger, ...) — siehe README-Abschnitt \"Attributes-Gate\"."`
}

// newDocumentResponseBody copies every fileee.Document field (SAME json tags, see
// documentResponseBody) into a documentResponseBody with Attributes left nil (caller sets it
// explicitly — see handleGetDocument). doc.Attributes (the go-fileee-internal, `json:"-"`-tagged
// DocumentAttributes value) is intentionally NOT copied anywhere onto documentResponseBody — the
// ONLY sanctioned path from Fileee's raw extraction data to a client is mapDocumentAttributes.
func newDocumentResponseBody(doc fileee.Document) documentResponseBody {
	return documentResponseBody{
		ID:               doc.ID,
		Version:          doc.Version,
		Created:          doc.Created,
		Modified:         doc.Modified,
		Deleted:          doc.Deleted,
		Status:           doc.Status,
		Type:             doc.Type,
		Pages:            doc.Pages,
		UploadAttribute:  doc.UploadAttribute,
		SharedSpaceIDs:   doc.SharedSpaceIDs,
		ShareInformation: doc.ShareInformation,
		ForbiddenActions: doc.ForbiddenActions,
	}
}
