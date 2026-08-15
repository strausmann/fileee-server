package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/strausmann/go-fileee/fileee"
)

// defaultDocumentListLimit ist das Default-Limit für GET /v1/documents, wenn der Aufrufer keinen
// (oder einen nicht-positiven) `limit`-Query-Parameter mitschickt. Bewusst ein eigener,
// server-lokaler Wert statt der unexportierten `fileee`-internen Konstante (die Lib exportiert
// ihr Default-Page-Limit nicht) — beide sind zufällig identisch (100), das ist aber keine
// Voraussetzung, nur eine sinnvolle Übereinstimmung.
const defaultDocumentListLimit = 100

// uploadSizeLimit begrenzt den Request-Body von POST /v1/documents auf maxBytes
// (FILEEE_MAX_UPLOAD_SIZE, Config.MaxUploadBytes) — siehe Verdrahtung + Begründung in
// server.go Handler(). maxBytes <= 0 deaktiviert das Limit (kein sinnvoller Konfigurationswert,
// aber defensiv statt eines Panics/Endlos-Limits). http.MaxBytesReader lässt einen nachfolgenden
// r.ParseMultipartForm (huma@v2.35.0 adapters/humago GetMultipartForm) mit
// "http: request body too large" abbrechen, sobald die Grenze überschritten wird.
func uploadSizeLimit(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maxBytes > 0 && r.Method == http.MethodPost && r.URL.Path == "/v1/documents" {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// registerDocumentRoutes registriert alle Dokument-/Seiten-bezogenen Operationen (Task 7 Read +
// Task 8 Write, Design-Spec §4.1/§4.2, docs/superpowers/specs/2026-07-24-fileee-server-design.md
// im homelab-management-Repo) auf der übergebenen Huma-API: Liste/Suche, Einzelabruf, PDF-/Bild-
// Stream, OCR-Tokens, Upload, Update und ZIP-Export. Jeder Handler delegiert direkt an s.fc
// (Core-Lib) und übersetzt Lib-Fehler ausschließlich über mapError (errors.go) — Ausnahme ist der
// Upload-Duplikat-Fall (uploadDuplicateError), der laut Design-Spec §12 zusätzliche Felder (id,
// isDuplicate) braucht, die das generische {error,code}-Schema nicht abdeckt.
func (s *Server) registerDocumentRoutes(api huma.API) {
	registerOperation(api, huma.Operation{
		OperationID: "list-documents",
		Method:      http.MethodGet,
		Path:        "/v1/documents",
		Summary:     "Dokumente auflisten oder per Volltextsuche durchsuchen",
	}, s.handleListDocuments)

	registerOperation(api, huma.Operation{
		OperationID: "upload-document",
		Method:      http.MethodPost,
		Path:        "/v1/documents",
		Summary:     "Neues Dokument hochladen (multipart)",
	}, s.handleUploadDocument)

	registerOperation(api, huma.Operation{
		OperationID: "get-document",
		Method:      http.MethodGet,
		Path:        "/v1/documents/{id}",
		Summary:     "Einzelnes Dokument laden",
	}, s.handleGetDocument)

	registerOperation(api, huma.Operation{
		OperationID: "update-document",
		Method:      http.MethodPut,
		Path:        "/v1/documents/{id}",
		Summary:     "Dokument-Metadaten aktualisieren",
		// SkipValidateBody: fileee.Document ist ein Wire-Typ der Core-Lib OHNE omitempty-Tags
		// (er MUSS 1:1 mit der Fileee-Antwort roundtripen können, siehe MarshalJSON/UnmarshalJSON
		// in fileee/types.go). Humas Default-Schemagenerierung markiert deshalb JEDES Feld ohne
		// omitempty als required — ein Aufrufer, der nur geänderte Metadaten schickt (der
		// eigentliche Zweck von PUT), würde an dieser Fremd-Validierung scheitern, obwohl
		// Documents.Update selbst kein vollständiges Objekt verlangt. Die Core-Lib bleibt trotzdem
		// die einzige Validierungsinstanz (serverseitiges Optimistic-Locking über version).
		SkipValidateBody: true,
	}, s.handleUpdateDocument)

	registerOperation(api, huma.Operation{
		OperationID: "download-document-pdf",
		Method:      http.MethodGet,
		Path:        "/v1/documents/{id}/pdf",
		Summary:     "Original-PDF eines Dokuments herunterladen (Stream)",
	}, s.handleDownloadDocumentPDF)

	registerOperation(api, huma.Operation{
		OperationID: "download-page-image",
		Method:      http.MethodGet,
		Path:        "/v1/pages/{pageId}/image",
		Summary:     "Seitenbild herunterladen (Fallback ohne PDF, Stream)",
	}, s.handleDownloadPageImage)

	registerOperation(api, huma.Operation{
		OperationID: "get-page-ocr",
		Method:      http.MethodGet,
		Path:        "/v1/pages/{pageId}/ocr",
		Summary:     "OCR-Tokens einer Seite laden",
	}, s.handleGetPageOCR)

	registerOperation(api, huma.Operation{
		OperationID: "export-documents-zip",
		Method:      http.MethodPost,
		Path:        "/v1/documents/export-zip",
		Summary:     "Passwortgeschützten ZIP-Export starten (asynchroner Process)",
	}, s.handleExportZip)
}

// listDocumentsInput drives GET /v1/documents. When Query is set, the search branch runs
// (Documents.Search + a Get-hydration per hit — Search only returns document IDs per
// fileee/search.go, Design-Spec §17 "API/Code"); when Query is empty, the page branch runs
// (stateless Start/Limit pagination over Documents.Query, see documentPageCursor). Both branches
// share Limit.
type listDocumentsInput struct {
	Query  string `query:"query" doc:"Full-text search (FULLTEXT/FUZZY via Documents.Search). Setting it activates search mode instead of page mode."`
	Limit  int    `query:"limit" doc:"Max. number of results for this page/search run." default:"100"`
	Cursor string `query:"cursor" doc:"Opaque cursor token from a previous response of this endpoint (only relevant in page mode, i.e. when query is empty). Empty = start a full sync from scratch; an empty cursor in the response likewise means the last page was reached."`
}

// documentListBody is the response body GET /v1/documents shares across both modes (Design-Spec
// §17: unified output {items, cursor, totalRows}). Cursor stays empty in search mode —
// Documents.Search has no page cursor, only Start/Limit pagination.
//
// Items is deliberately []documentResponseBody, NOT []fileee.Document (security-review finding,
// PR #38: fileee.Document.MarshalJSON unconditionally reconstructs the full wire envelope
// {"attributes":{"data":{...}}} including RawExtra on EVERY direct marshal — even as a slice
// element — REGARDLESS of any `json:"-"` tag. A []fileee.Document here would have completely
// bypassed the Issue-#37 gate (includeAttributes + FILEEE_EXPOSE_ATTRIBUTES) for GET
// /v1/documents — every document in every list/search result would have leaked the full
// financial PII ungated. documentListBody therefore uses documentResponseBody exactly like
// get/upload/update-document does, but WITH Attributes==nil (lists get NO opt-in attributes —
// only GET /v1/documents/{id} has the gated path, see handleGetDocument). See also:
// TestDocumentResponseBody_JSONTagsStayInSyncWithFileeeDocument (drift guard) and
// response_body_safety_test.go (structural guardrail against this class of bug).
type documentListBody struct {
	Items     []documentResponseBody `json:"items" doc:"Documents on this page or in this search run. NEVER carries an \"attributes\" field (no opt-in for lists, see the Attributes gate in the README)."`
	Cursor    string                 `json:"cursor" doc:"Opaque follow-up cursor token for the next call in page mode. Always empty in search mode; in page mode ONLY empty once the last page has been reached (see documentPageCursor)."`
	TotalRows int                    `json:"totalRows" doc:"Total count reported by Fileee (search hits, or total documents in page mode)."`
}

// listDocumentsOutput kapselt documentListBody als Huma-Response von GET /v1/documents.
type listDocumentsOutput struct {
	Body documentListBody
}

// mapDocuments projects a []fileee.Document onto []documentResponseBody via newDocumentResponseBody
// — Attributes bleibt für JEDES Element nil (Listen haben keinen Opt-in-Pfad, siehe
// documentListBody-Doku).
func mapDocuments(docs []fileee.Document) []documentResponseBody {
	out := make([]documentResponseBody, 0, len(docs))
	for _, doc := range docs {
		out = append(out, newDocumentResponseBody(doc))
	}
	return out
}

// handleListDocuments implements GET /v1/documents. When Query is set, it searches via
// Documents.Search (returns only hit IDs) and hydrates every hit via Documents.Get to the full
// document (N+1 access pattern — accepted deliberately, see the Search documentation: this
// Fileee API facet only ever returns details via Get). Without Query, it pages through
// Documents.Query using a Start offset decoded from the `cursor` parameter (documentPageCursor)
// and returns the encoded follow-up cursor — see documentPageCursor for why this does NOT (any
// longer) run over Documents.Diff (issue #39).
func (s *Server) handleListDocuments(ctx context.Context, in *listDocumentsInput) (*listDocumentsOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = defaultDocumentListLimit
	}

	if in.Query != "" {
		res, err := s.fc.Documents.Search(ctx, in.Query, fileee.SearchOptions{Limit: limit})
		if err != nil {
			return nil, mapError(err)
		}
		items := make([]fileee.Document, 0, len(res.IDs))
		for _, id := range res.IDs {
			doc, err := s.fc.Documents.Get(ctx, id)
			if err != nil {
				return nil, mapError(err)
			}
			items = append(items, *doc)
		}
		return &listDocumentsOutput{Body: documentListBody{Items: mapDocuments(items), TotalRows: res.TotalRows}}, nil
	}

	start, err := decodeDocumentPageCursor(in.Cursor)
	if err != nil {
		return nil, newStatusError(http.StatusBadRequest, "invalid_cursor", "invalid cursor parameter")
	}
	res, err := s.fc.Documents.Query(ctx, fileee.QueryOptions{Start: start, Limit: limit})
	if err != nil {
		return nil, mapError(err)
	}

	nextStart := start + len(res.Rows)
	var nextCursor string
	if len(res.Rows) > 0 && nextStart < res.TotalRows {
		nextCursor, err = encodeDocumentPageCursor(nextStart)
		if err != nil {
			return nil, mapError(err)
		}
	}
	return &listDocumentsOutput{Body: documentListBody{Items: mapDocuments(res.Rows), Cursor: nextCursor, TotalRows: res.TotalRows}}, nil
}

// documentPageCursor is the opaque pagination state of GET /v1/documents' page branch: a plain
// zero-based offset into Documents.Query — NOT (any longer) a fileee.Cursor over
// Documents.Diff.
//
// Root cause (issue #39, live-reproduced against fileee-api.strausmann.cloud): two consecutive
// calls, where the second one passes back the cursor the first response returned, yielded the
// same first/last document ID instead of the next page. The cause is go-fileee@v0.2.0 itself
// (fileee/service.go, restService[T].Diff): it builds the request body (diffRequestWire) WITHOUT
// ever populating the Start field — every Diff call therefore effectively always sends
// `"start":0`, regardless of how many documents the cursor passed in already lists as known
// (Known). fileee.Cursor itself has no offset/position field at all, only EntityType+Known
// (fileee/query.go) — there is no way, via the public Documents.Diff signature, to force an
// advancing start. Observed live behaviour: localResults does not visibly filter the returned
// `rows` (the same top-N page comes back every time) — it presumably only ever informs
// idsToDelete server-side, not page selection; see also the open verification gap in
// .claude/skills/fileee/references/troubleshooting.md ("localResults delta effect ... not
// confirmed").
//
// Documents.Query, on the other hand, is proven to paginate correctly via Start/Limit — both per
// the API reference and by the fact that go-fileee's own full-export helper
// (restService[T].queryAllPages) relies exclusively on Query for exactly this purpose, never on
// Diff. This page branch does the same: Start advances on every call by the number of rows just
// returned, until either an empty page comes back or the new Start reaches the reported
// TotalRows — at which point the response cursor stays empty as the termination signal.
//
// No more Diff-based delta sync (idsToDelete goes away) — harmless for the full
// document-by-document walk (migration scenario) issue #39 asks for; an incremental change sync
// would need a fix in go-fileee itself first anyway, see the discussion above.
type documentPageCursor struct {
	Start int `json:"start"`
}

// encodeDocumentPageCursor packs a Start offset as an opaque web token — JSON serialization,
// then base64 URL without padding, analogous to the generic encodeCursor(fileee.Cursor) (still
// used by /v1/conversations, see handlers_conversations.go). Callers MUST treat the result as a
// black box (see documentListBody.Cursor) — the server is the only place that ever passes it
// back into decodeDocumentPageCursor.
func encodeDocumentPageCursor(start int) (string, error) {
	b, err := json.Marshal(documentPageCursor{Start: start})
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// decodeDocumentPageCursor unpacks a token produced by encodeDocumentPageCursor into its Start
// offset. An empty string yields Start=0 — the normal case for the very first call without a
// `cursor` parameter (a full sync from scratch). A negative Start is treated as a decode error
// (can only arise from a tampered/foreign token, see TestListDocuments_InvalidCursorReturns400)
// — Documents.Query itself would quietly answer a too-LARGE start with an empty page, that is
// not an error case.
func decodeDocumentPageCursor(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("decode cursor: %w", err)
	}
	var c documentPageCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0, fmt.Errorf("decode cursor: %w", err)
	}
	if c.Start < 0 {
		return 0, fmt.Errorf("decode cursor: negative start %d", c.Start)
	}
	return c.Start, nil
}

// encodeCursor packs a lib cursor (fileee.Cursor) as an opaque web token: JSON serialization,
// then base64 URL without padding. Callers MUST treat the result as a black box — the server is
// the only place that ever passes it back into decodeCursorToken. Still used by GET
// /v1/conversations (handlers_conversations.go); GET /v1/documents has used documentPageCursor
// instead of fileee.Cursor since issue #39 (see its doc comment).
func encodeCursor(c fileee.Cursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// decodeCursorToken decodes a NON-empty cursor token produced by encodeCursor (base64 URL
// without padding, then JSON) — deliberately its own function (rather than part of a
// decodeCursor wrapper, which no longer exists for GET /v1/documents since issue #39) so that
// decodeConversationsCursor (handlers_conversations.go) can reuse the same decoding with its own
// default EntityType ("Conversation" instead of "Document") without duplicating the base64/JSON
// logic.
func decodeCursorToken(s string) (fileee.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return fileee.Cursor{}, fmt.Errorf("cursor dekodieren: %w", err)
	}
	var c fileee.Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return fileee.Cursor{}, fmt.Errorf("cursor dekodieren: %w", err)
	}
	return c, nil
}

// getDocumentInput steuert GET /v1/documents/{id}. IncludeAttributes ist das Issue-#37-Opt-in für
// Fileees automatisch extrahierte Indexierungs-Metadaten (attributes.data, siehe attributes.go) —
// weggelassen/false ändert am Response-Body NICHTS gegenüber dem Vorzustand.
type getDocumentInput struct {
	ID                string `path:"id" doc:"Dokument-ID."`
	IncludeAttributes bool   `query:"includeAttributes" doc:"Opt-in: Fileees automatisch extrahierte Indexierungs-Metadaten (Dokumenttyp, Absender/Empfänger, Tags, Rechnungsdaten, IBAN, Kundennummer, ...) im Response-Body unter \"attributes\" mitliefern. Braucht zusätzlich das serverseitige FILEEE_EXPOSE_ATTRIBUTES-Gate — ohne dieses Gate liefert ein gesetztes includeAttributes=true 403 statt der Metadaten (private Finanz-PII, niemals Default-on, siehe README)." default:"false"`
}

// getDocumentOutput ist der Response-Body von GET/POST/PUT /v1/documents/{id}: das vollständige
// fileee.Document (inkl. Pages) plus dem optionalen, Issue-#37-gegateten "attributes"-Feld — siehe
// documentResponseBody (attributes.go).
type getDocumentOutput struct {
	Body documentResponseBody
}

// handleGetDocument implementiert GET /v1/documents/{id} — ein dünner Durchgriff auf
// Documents.Get, ergänzt um das Issue-#37-Opt-in für attributes.data. Das Opt-in braucht ZWEI
// unabhängige Zustimmungen: den Aufrufer-seitigen Query-Parameter (in.IncludeAttributes) UND das
// Betreiber-seitige FILEEE_EXPOSE_ATTRIBUTES-Gate (s.cfg.ExposeAttributes) — fehlt eine der
// beiden, bleibt das Verhalten unverändert (kein "attributes"-Feld), fehlt NUR das Gate bei
// gesetztem Parameter, antwortet der Handler explizit mit 403 statt den Parameter still zu
// ignorieren (der Aufrufer soll erkennen, dass er PII angefordert hat, die der Betreiber nicht
// freigeschaltet hat — kein leises Weglassen).
func (s *Server) handleGetDocument(ctx context.Context, in *getDocumentInput) (*getDocumentOutput, error) {
	if in.IncludeAttributes && !s.cfg.ExposeAttributes {
		return nil, newStatusError(http.StatusForbidden, "attributes_disabled",
			"attribute exposure disabled; set FILEEE_EXPOSE_ATTRIBUTES=true to enable")
	}

	doc, err := s.fc.Documents.Get(ctx, in.ID)
	if err != nil {
		return nil, mapError(err)
	}

	body := newDocumentResponseBody(*doc)
	if in.IncludeAttributes && s.cfg.ExposeAttributes {
		attrs := mapDocumentAttributes(doc.Attributes)
		body.Attributes = &attrs
	}
	return &getDocumentOutput{Body: body}, nil
}

// downloadDocumentPDFInput steuert GET /v1/documents/{id}/pdf.
type downloadDocumentPDFInput struct {
	ID   string         `path:"id" doc:"Dokument-ID."`
	Mode fileee.PDFMode `query:"mode" doc:"download (Originaldatei) oder print (druckoptimierte Fassung)." default:"download"`
}

// handleDownloadDocumentPDF implementiert GET /v1/documents/{id}/pdf als Stream — der PDF-Body
// wird NIE vollständig in den RAM gepuffert: io.Copy kopiert direkt vom Lib-eigenen
// io.ReadCloser (Documents.DownloadPDF) auf den Huma-BodyWriter (Design-Spec §13
// "Streaming-Download ohne RAM-Puffer").
func (s *Server) handleDownloadDocumentPDF(ctx context.Context, in *downloadDocumentPDFInput) (*huma.StreamResponse, error) {
	mode := in.Mode
	if mode == "" {
		mode = fileee.PDFModeDownload
	}
	rc, err := s.fc.Documents.DownloadPDF(ctx, in.ID, mode)
	if err != nil {
		return nil, mapError(err)
	}
	return &huma.StreamResponse{
		Body: func(sctx huma.Context) {
			defer rc.Close()
			sctx.SetHeader("Content-Type", "application/pdf")
			if _, err := io.Copy(sctx.BodyWriter(), rc); err != nil {
				s.log.Error("pdf stream copy fehlgeschlagen", "document_id", in.ID, "error", err)
			}
		},
	}, nil
}

// downloadPageImageInput steuert GET /v1/pages/{pageId}/image. Version wird 1:1 an
// Documents.DownloadPageImage durchgereicht (die Lib-Signatur verlangt sie zwingend) — der
// Aufrufer MUSS den zuletzt aus dem übergeordneten Dokument gelesenen Wert mitschicken
// (Document.Pages[i].ImageVersion), NIE einen zwischengespeicherten (siehe Page-Doku in
// fileee/types.go). Dieser dünne Passthrough-Endpunkt selbst prüft die Frische NICHT — ein
// künftiger Unified-Resolver (Design-Spec §4.1a, spätere Task) kann diesen Wert serverseitig
// vorab aus einem frisch geladenen Dokument beziehen und braucht dafür keine Änderung an dieser
// Route.
type downloadPageImageInput struct {
	PageID  string           `path:"pageId" doc:"Seiten-ID."`
	Size    fileee.ImageSize `query:"size" doc:"Bildgröße (smedium/medium)." default:"medium"`
	Version int64            `query:"v" doc:"Aktuelle imageVersion der Seite (aus Document.Pages), NIE zwischenspeichern."`
}

// handleDownloadPageImage implementiert GET /v1/pages/{pageId}/image als Stream (Fallback-Weg
// ohne PDF, analog handleDownloadDocumentPDF ohne RAM-Puffer).
func (s *Server) handleDownloadPageImage(ctx context.Context, in *downloadPageImageInput) (*huma.StreamResponse, error) {
	size := in.Size
	if size == "" {
		size = fileee.ImageSizeMedium
	}
	rc, err := s.fc.Documents.DownloadPageImage(ctx, in.PageID, size, in.Version)
	if err != nil {
		return nil, mapError(err)
	}
	return &huma.StreamResponse{
		Body: func(sctx huma.Context) {
			defer rc.Close()
			sctx.SetHeader("Content-Type", "image/jpeg")
			if _, err := io.Copy(sctx.BodyWriter(), rc); err != nil {
				s.log.Error("page-image stream copy fehlgeschlagen", "page_id", in.PageID, "error", err)
			}
		},
	}, nil
}

// getPageOCRInput steuert GET /v1/pages/{pageId}/ocr.
type getPageOCRInput struct {
	PageID string `path:"pageId" doc:"Seiten-ID."`
}

// getPageOCROutput ist der Response-Body von GET /v1/pages/{pageId}/ocr: die flache Liste der
// erkannten Text-Tokens mit Bounding-Box (fileee.OCRToken) — Grundlage einer möglichen
// Fileee→Paperless-ngx-Migration (siehe fileee/ocr.go).
type getPageOCROutput struct {
	Body []fileee.OCRToken
}

// handleGetPageOCR implementiert GET /v1/pages/{pageId}/ocr — dünner Durchgriff auf
// Documents.PageOCR (authentifizierter Pfad; der anonyme Share-Pfad über ShareClient.
// SharedPageOCR ist Scope einer späteren Task, Design-Spec §4.1 Share-Proxy-Routen).
func (s *Server) handleGetPageOCR(ctx context.Context, in *getPageOCRInput) (*getPageOCROutput, error) {
	toks, err := s.fc.Documents.PageOCR(ctx, in.PageID)
	if err != nil {
		return nil, mapError(err)
	}
	return &getPageOCROutput{Body: toks}, nil
}

// uploadDocumentInput steuert POST /v1/documents (multipart, Design-Spec §4.2). Huma dekodiert das
// Formular über huma.MultipartFormFiles[T] (huma@v2.35.0 formdata.go) — das "file"-Feld ist
// Pflicht (required:"true"), "title" ist ein optionales Textfeld (form:"title", kein Datei-Feld).
// contentType:"application/octet-stream" auf File akzeptiert JEDEN Dateityp (huma@v2.35.0
// MimeTypeValidator.Validate behandelt "application/octet-stream" als Wildcard) — die eigentliche
// Dateityp-Prüfung übernimmt Fileee selbst (ErrUnsupportedFileType, 415, mapError).
type uploadDocumentInput struct {
	RawBody huma.MultipartFormFiles[struct {
		File  huma.FormFile `form:"file" contentType:"application/octet-stream" required:"true" doc:"Hochzuladende Datei (beliebiger Dateityp; Fileee lehnt nicht unterstützte Typen serverseitig mit 415 ab)."`
		Title string        `form:"title" required:"false" doc:"Optionaler Dokumenttitel. Fallback: Dateiname der hochgeladenen Datei."`
	}]
}

// uploadDuplicateError signalisiert 409 bei einem Upload-Duplikat (Design-Spec §12: "Upload-
// Duplikat (ErrDuplicateDocument) → 409 {error:"duplicate", id, isDuplicate:true}"). Anders als das
// generische {error,code}-Schema aus errors.go (mapError) MUSS diese Antwort zusätzlich die id des
// bereits bestehenden Dokuments sowie isDuplicate:true enthalten, damit der Aufrufer sofort mit der
// bestehenden id weiterarbeiten kann, statt sie separat nachfragen zu müssen — deshalb ein
// eigenständiger, lokaler Fehlertyp statt eines mapError-Zweigs.
type uploadDuplicateError struct {
	// ErrorMsg ist Feld "error" im JSON-Body — analog zu statusError (errors.go).
	ErrorMsg string `json:"error"`
	// ErrorCode ist Feld "code" im JSON-Body — hier immer "duplicate".
	ErrorCode string `json:"code"`
	// ID ist die id des bereits bestehenden Dokuments (nicht die client-generierte id des Uploads).
	ID string `json:"id"`
	// IsDuplicate ist immer true — Design-Spec §12 verlangt das Feld explizit im Body.
	IsDuplicate bool `json:"isDuplicate"`
}

// Error liefert die menschenlesbare Fehlermeldung und erfüllt damit das error-Interface.
func (e *uploadDuplicateError) Error() string { return e.ErrorMsg }

// GetStatus liefert immer 409 und erfüllt damit huma.StatusError (siehe statusError.GetStatus,
// errors.go, für den zugrunde liegenden Huma-Mechanismus).
func (e *uploadDuplicateError) GetStatus() int { return http.StatusConflict }

// newUploadDuplicateError baut den 409-Fehler aus der id des von Documents.Upload gemeldeten,
// bereits bestehenden Dokuments.
func newUploadDuplicateError(existingID string) *uploadDuplicateError {
	return &uploadDuplicateError{
		ErrorMsg:    "document already exists",
		ErrorCode:   "duplicate",
		ID:          existingID,
		IsDuplicate: true,
	}
}

// handleUploadDocument implementiert POST /v1/documents. Sie delegiert an Documents.Upload
// (client-generierte id, serverseitige Duplikaterkennung — siehe fileee/documents.go) und behandelt
// den Duplikat-Fall gesondert (uploadDuplicateError statt mapError), weil dessen Response-Body
// zusätzliche Felder braucht (Design-Spec §12). Jeder andere Fehler läuft weiterhin über mapError.
func (s *Server) handleUploadDocument(ctx context.Context, in *uploadDocumentInput) (*getDocumentOutput, error) {
	data := in.RawBody.Data()
	defer data.File.Close()

	title := data.Title
	if title == "" {
		title = data.File.Filename
	}

	res, err := s.fc.Documents.Upload(ctx, data.File, fileee.UploadMetadata{Title: title})
	if err != nil {
		if errors.Is(err, fileee.ErrDuplicateDocument) && res != nil && res.Document != nil {
			return nil, newUploadDuplicateError(res.Document.ID)
		}
		return nil, mapError(err)
	}
	return &getDocumentOutput{Body: newDocumentResponseBody(*res.Document)}, nil
}

// updateDocumentInput steuert PUT /v1/documents/{id}. Die Pfad-id ist maßgeblich — sie überschreibt
// ein eventuell abweichendes Body.ID, damit ein Aufrufer nicht versehentlich ein anderes Dokument
// als das in der URL adressierte ändert.
type updateDocumentInput struct {
	ID   string          `path:"id" doc:"Dokument-ID."`
	Body fileee.Document `doc:"Vollständiges, aktualisiertes Dokument (Optimistic Locking über version, siehe Documents.Update)."`
}

// handleUpdateDocument implementiert PUT /v1/documents/{id} — dünner Durchgriff auf
// Documents.Update.
func (s *Server) handleUpdateDocument(ctx context.Context, in *updateDocumentInput) (*getDocumentOutput, error) {
	doc := in.Body
	doc.ID = in.ID
	updated, err := s.fc.Documents.Update(ctx, &doc)
	if err != nil {
		return nil, mapError(err)
	}
	return &getDocumentOutput{Body: newDocumentResponseBody(*updated)}, nil
}

// exportZipRequest ist der Body von POST /v1/documents/export-zip (Design-Spec §4.2). Eine leere
// DocumentIDs-Liste exportiert ALLE Dokumente des Kontos (Documents.ExportAll).
type exportZipRequest struct {
	DocumentIDs []string `json:"documentIds,omitempty" doc:"Zu exportierende Dokument-IDs. Leer/weggelassen = alle Dokumente."`
	ZipPassword string   `json:"zipPassword" doc:"Passwort, mit dem die erzeugte ZIP-Datei geschützt wird."`
}

// exportZipInput steuert POST /v1/documents/export-zip.
type exportZipInput struct {
	Body exportZipRequest
}

// exportZipOutput ist der Response-Body von POST /v1/documents/export-zip: der gestartete
// asynchrone Vorgang (fileee.Process) — sein Fortschritt wird über GET /v1/processes/{id} bzw.
// POST /v1/processes/{id}/wait abgefragt (handlers_share.go).
type exportZipOutput struct {
	Body fileee.Process
}

// handleExportZip implementiert POST /v1/documents/export-zip. Eine leere/weggelassene
// documentIds-Liste läuft über Documents.ExportAll (alle Dokumente), eine nicht-leere Liste über
// Documents.ExportZIP (Teilmenge) — beide liefern denselben Process-Typ.
func (s *Server) handleExportZip(ctx context.Context, in *exportZipInput) (*exportZipOutput, error) {
	var proc *fileee.Process
	var err error
	if len(in.Body.DocumentIDs) == 0 {
		proc, err = s.fc.Documents.ExportAll(ctx, in.Body.ZipPassword)
	} else {
		proc, err = s.fc.Documents.ExportZIP(ctx, in.Body.DocumentIDs, in.Body.ZipPassword)
	}
	if err != nil {
		return nil, mapError(err)
	}
	return &exportZipOutput{Body: *proc}, nil
}
