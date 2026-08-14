package main

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/strausmann/go-fileee/fileee"
)

// emptyInput ist der Input-Typ für Read-Endpunkte ohne Pfad-/Query-Parameter (die Stammdaten-
// Listen dieser Datei). Huma akzeptiert einen Zeiger auf ein feldloses struct als "kein Input"
// (siehe huma@v2.35.0 huma_test.go, Testfall "response-stream").
type emptyInput struct{}

// entityListBody ist der einheitliche Response-Body der generischen Stammdaten-Listen (Tags,
// Contacts, DocumentTypes, DocumentTypeSchemes, Reminders) — sie teilen sich alle dieselbe
// Query/Diff/Get-Konvention (fileee.ReadService[T], fileee/service.go) und liefern hier deshalb
// dieselbe {items, totalRows}-Form. Ein Diff-Cursor ist für diese Ressourcen (Stand dieser Task)
// bewusst NICHT exponiert — anders als bei /v1/documents gibt es dafür noch keinen dokumentierten
// Bedarf; die erste Query-Seite (Default-Limit 100 aus fileee.QueryOptions.toWire) genügt für den
// aktuellen Read-Scope. Auch von listBoxesOutput wiederverwendet (Boxes.List kennt kein
// Query/TotalRows-Ergebnis wie die generischen ReadServices — TotalRows wird dort aus len(Items)
// abgeleitet).
//
// Companies nutzt DIESEN generischen Typ NICHT (mehr) — siehe companyListBody/handleListCompanies:
// fileee.Company hat wie fileee.Document eine eigene MarshalJSON, die unabhängig vom `json:"-"`-Tag
// immer die volle attributes.data (IBANs, VAT-IDs, E-Mails, Telefonnummern, …) mit ausliefert.
// entityListBody[T] setzt T direkt und ungefiltert in Items ein — für jeden Typ mit eigener
// MarshalJSON wäre das derselbe Leak wie bei documentListBody (siehe dessen Doku). Ein T-Element
// OHNE eigene MarshalJSON (Tag/Contact/DocumentType/DocumentTypeScheme/Reminder/Box — alle
// verifiziert, siehe response_body_safety_test.go) ist dagegen unbedenklich.
type entityListBody[T any] struct {
	Items     []T `json:"items" doc:"Erste Seite der Ressource (Default-Limit 100)."`
	TotalRows int `json:"totalRows" doc:"Von Fileee gemeldete (bzw. bei Boxes aus der Listenlänge abgeleitete) Gesamtzahl."`
}

// entityListOutput kapselt entityListBody[T] als Huma-Response.
type entityListOutput[T any] struct {
	Body entityListBody[T]
}

// registerEntityListRoute registriert eine parameterlose GET-Liste für einen generischen
// fileee.ReadService[T]. NUR für T OHNE eigene MarshalJSON verwenden (siehe entityListBody-Doku) —
// Tags/Contacts/DocumentTypes/DocumentTypeSchemes/Reminders teilen sich exakt diese Signatur
// (fileee/service.go ReadService[T].Query) — query wird deshalb direkt als Methodenwert übergeben
// (z.B. s.fc.Tags.Query), ganz ohne Wrapper-Closure.
func registerEntityListRoute[T any](api huma.API, operationID, path string, query func(ctx context.Context, opts fileee.QueryOptions) (*fileee.QueryResult[T], error)) {
	huma.Register(api, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodGet,
		Path:        path,
	}, func(ctx context.Context, in *emptyInput) (*entityListOutput[T], error) {
		res, err := query(ctx, fileee.QueryOptions{})
		if err != nil {
			return nil, mapError(err)
		}
		return &entityListOutput[T]{Body: entityListBody[T]{Items: res.Rows, TotalRows: res.TotalRows}}, nil
	})
}

// companyResponseBody mirrors fileee.Company's PUBLIC fields (same JSON tags, no embedding) —
// exactly the same pattern and rationale as documentResponseBody/newDocumentResponseBody
// (attributes.go). fileee.Company carries its own MarshalJSON (fileee/types.go) that — like
// fileee.Document — ALWAYS reconstructs the wire envelope {"attributes":{"data":{...}}} from its
// otherwise `json:"-"`-tagged Attributes field (fileee.CompanyAttributes: IBANs, VAT IDs, emails,
// phone numbers, websites, German tax IDs, plus any RawExtra).
//
// Found during the Issue #37 security review (PR #38): GET /v1/companies used
// entityListBody[fileee.Company] directly, so EVERY company in EVERY response leaked its full,
// UNGATED attributes.data — no opt-in, no FILEEE_EXPOSE_ATTRIBUTES gate, unrelated to (and
// pre-dating) Issue #37's own Document-attributes gate. companyResponseBody/
// newCompanyResponseBody close that leak the same way newDocumentResponseBody does: explicit field
// mirroring, Attributes never copied anywhere. There is currently no opt-in/gate design for
// company attributes (out of scope here) — they are simply never exposed, matching the field's own
// `json:"-"` tag, which fileee.Company.MarshalJSON was silently overriding.
type companyResponseBody struct {
	ID              string `json:"id"`
	Version         int64  `json:"version"`
	Created         string `json:"created"`
	Modified        string `json:"modified"`
	Deleted         bool   `json:"deleted"`
	CompanyName     string `json:"companyName"`
	ContactType     string `json:"contactType"`
	ContactStatus   string `json:"contactStatus"`
	DocumentCounter int    `json:"documentCounter"`
	Connected       bool   `json:"connected"`
	FromUserDB      bool   `json:"fromUserDb"`
	HasLogo         bool   `json:"hasLogo"`
}

// newCompanyResponseBody copies every fileee.Company field EXCEPT Attributes (SAME json tags, see
// companyResponseBody) — analogous to newDocumentResponseBody.
func newCompanyResponseBody(c fileee.Company) companyResponseBody {
	return companyResponseBody{
		ID:              c.ID,
		Version:         c.Version,
		Created:         c.Created,
		Modified:        c.Modified,
		Deleted:         c.Deleted,
		CompanyName:     c.CompanyName,
		ContactType:     c.ContactType,
		ContactStatus:   c.ContactStatus,
		DocumentCounter: c.DocumentCounter,
		Connected:       c.Connected,
		FromUserDB:      c.FromUserDB,
		HasLogo:         c.HasLogo,
	}
}

// companyListBody is the response body of GET /v1/companies — same {items, totalRows} shape as
// entityListBody[T], but with a dedicated, mapped Items type instead of the generic T (see
// companyResponseBody doc comment for why fileee.Company itself can never appear here).
type companyListBody struct {
	Items     []companyResponseBody `json:"items" doc:"Erste Seite der Firmen (Default-Limit 100). Enthält NIE ein \"attributes\"-Feld."`
	TotalRows int                   `json:"totalRows" doc:"Von Fileee gemeldete Gesamtzahl."`
}

// listCompaniesOutput kapselt companyListBody als Huma-Response von GET /v1/companies.
type listCompaniesOutput struct {
	Body companyListBody
}

// handleListCompanies implementiert GET /v1/companies — bewusst AUSSERHALB von
// registerEntityListRoute[T] (wie schon handleListBoxes), weil companyResponseBody eine Mapping-
// Stufe zwischen fileee.Company und dem Response-Body braucht (companyListBody-Doku), die der
// generische Helfer (der T unverändert in entityListBody[T] durchreicht) nicht abbildet.
func (s *Server) handleListCompanies(ctx context.Context, in *emptyInput) (*listCompaniesOutput, error) {
	res, err := s.fc.Companies.Query(ctx, fileee.QueryOptions{})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]companyResponseBody, 0, len(res.Rows))
	for _, c := range res.Rows {
		items = append(items, newCompanyResponseBody(c))
	}
	return &listCompaniesOutput{Body: companyListBody{Items: items, TotalRows: res.TotalRows}}, nil
}

// getBoxInput steuert GET /v1/boxes/{id}.
type getBoxInput struct {
	ID string `path:"id" doc:"FileeeBox-ID."`
}

// getBoxOutput ist der Response-Body von GET /v1/boxes/{id}.
type getBoxOutput struct {
	Body fileee.Box
}

// listBoxesOutput ist der Response-Body von GET /v1/boxes.
type listBoxesOutput struct {
	Body entityListBody[fileee.Box]
}

// registerEntityRoutes registriert die Stammdaten-Operationen (Task 7 Read + Task 8 Write,
// Design-Spec §4.1/§4.2, docs/superpowers/specs/2026-07-24-fileee-server-design.md im
// homelab-management-Repo): Tags, Companies, Contacts, DocumentTypes, DocumentTypeSchemes,
// Reminders (alle über das generische ReadService[T]-Muster) sowie Boxes (eigenes
// BoxService-Interface mit List/Get statt Query/Diff/Get). Write-Operationen (Reminders/Contacts
// Create+Update, Box-Dokument-Zuordnung) delegieren wie die Read-Operationen direkt an s.fc und
// übersetzen Fehler ausschließlich über mapError.
func (s *Server) registerEntityRoutes(api huma.API) {
	registerEntityListRoute(api, "list-tags", "/v1/tags", s.fc.Tags.Query)
	huma.Register(api, huma.Operation{
		OperationID: "list-companies",
		Method:      http.MethodGet,
		Path:        "/v1/companies",
	}, s.handleListCompanies)
	registerEntityListRoute(api, "list-contacts", "/v1/contacts", s.fc.Contacts.Query)
	registerEntityListRoute(api, "list-document-types", "/v1/document-types", s.fc.DocumentTypes.Query)
	registerEntityListRoute(api, "list-document-type-schemes", "/v1/document-type-schemes", s.fc.DocumentTypeSchemes.Query)
	registerEntityListRoute(api, "list-reminders", "/v1/reminders", s.fc.Reminders.Query)

	huma.Register(api, huma.Operation{
		OperationID: "list-boxes",
		Method:      http.MethodGet,
		Path:        "/v1/boxes",
	}, s.handleListBoxes)

	huma.Register(api, huma.Operation{
		OperationID: "get-box",
		Method:      http.MethodGet,
		Path:        "/v1/boxes/{id}",
	}, s.handleGetBox)

	huma.Register(api, huma.Operation{
		OperationID: "add-box-document",
		Method:      http.MethodPost,
		Path:        "/v1/boxes/{boxId}/documents/{docId}",
		Summary:     "Dokument in eine FileeeBox einheften",
	}, s.handleAddBoxDocument)

	huma.Register(api, huma.Operation{
		OperationID: "remove-box-document",
		Method:      http.MethodDelete,
		Path:        "/v1/boxes/{boxId}/documents/{docId}",
		Summary:     "Dokument aus einer FileeeBox aushängen (kein Destruktiv-Gate, Ausheften ≠ Löschen)",
	}, s.handleRemoveBoxDocument)

	huma.Register(api, huma.Operation{
		OperationID: "create-reminder",
		Method:      http.MethodPost,
		Path:        "/v1/reminders",
		Summary:     "Erinnerung anlegen",
		// SkipValidateBody: siehe Begründung bei "update-document" (handlers_documents.go) —
		// fileee.Reminder ist derselbe Fall eines omitempty-losen Lib-Wire-Typs; Reminders.Create
		// füllt fehlende id ohnehin selbst auf (fileee/reminders.go).
		SkipValidateBody: true,
	}, s.handleCreateReminder)

	huma.Register(api, huma.Operation{
		OperationID:      "update-reminder",
		Method:           http.MethodPut,
		Path:             "/v1/reminders/{id}",
		Summary:          "Erinnerung aktualisieren",
		SkipValidateBody: true,
	}, s.handleUpdateReminder)

	huma.Register(api, huma.Operation{
		OperationID:      "create-contact",
		Method:           http.MethodPost,
		Path:             "/v1/contacts",
		Summary:          "Kontakt anlegen",
		SkipValidateBody: true,
	}, s.handleCreateContact)

	huma.Register(api, huma.Operation{
		OperationID:      "update-contact",
		Method:           http.MethodPut,
		Path:             "/v1/contacts/{id}",
		Summary:          "Kontakt aktualisieren",
		SkipValidateBody: true,
	}, s.handleUpdateContact)
}

// handleListBoxes implementiert GET /v1/boxes über Boxes.List (intern ein Diff mit vollem
// Box-Cursor, fileee/boxes.go) — TotalRows wird hier aus len(boxes) abgeleitet, da
// BoxService.List keinen separaten TotalRows-Wert liefert.
func (s *Server) handleListBoxes(ctx context.Context, in *emptyInput) (*listBoxesOutput, error) {
	boxes, err := s.fc.Boxes.List(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return &listBoxesOutput{Body: entityListBody[fileee.Box]{Items: boxes, TotalRows: len(boxes)}}, nil
}

// handleGetBox implementiert GET /v1/boxes/{id} — dünner Durchgriff auf Boxes.Get.
func (s *Server) handleGetBox(ctx context.Context, in *getBoxInput) (*getBoxOutput, error) {
	box, err := s.fc.Boxes.Get(ctx, in.ID)
	if err != nil {
		return nil, mapError(err)
	}
	return &getBoxOutput{Body: *box}, nil
}

// boxDocumentInput steuert POST/DELETE /v1/boxes/{boxId}/documents/{docId} — dieselben Pfad-
// Parameter für Ein- und Aushängen (Design-Spec §4.2: "Ausheften ≠ Löschen → kein Destruktiv-Gate",
// deshalb ohne FILEEE_ALLOW_DESTRUCTIVE-Prüfung, anders als die echten Hard-DELETE-Routen).
type boxDocumentInput struct {
	BoxID string `path:"boxId" doc:"FileeeBox-ID."`
	DocID string `path:"docId" doc:"Dokument-ID."`
}

// handleAddBoxDocument implementiert POST /v1/boxes/{boxId}/documents/{docId} — dünner Durchgriff
// auf Boxes.AddDocument. Kein Response-Body (204 No Content), da die Lib-Methode selbst nichts
// zurückliefert.
func (s *Server) handleAddBoxDocument(ctx context.Context, in *boxDocumentInput) (*struct{}, error) {
	if err := s.fc.Boxes.AddDocument(ctx, in.BoxID, in.DocID); err != nil {
		return nil, mapError(err)
	}
	return nil, nil
}

// handleRemoveBoxDocument implementiert DELETE /v1/boxes/{boxId}/documents/{docId} — dünner
// Durchgriff auf Boxes.RemoveDocument (kein Destruktiv-Gate, siehe boxDocumentInput-Doku).
func (s *Server) handleRemoveBoxDocument(ctx context.Context, in *boxDocumentInput) (*struct{}, error) {
	if err := s.fc.Boxes.RemoveDocument(ctx, in.BoxID, in.DocID); err != nil {
		return nil, mapError(err)
	}
	return nil, nil
}

// createReminderInput steuert POST /v1/reminders.
type createReminderInput struct {
	Body fileee.Reminder
}

// reminderOutput ist der gemeinsame Response-Body von POST /v1/reminders und
// PUT /v1/reminders/{id}.
type reminderOutput struct {
	Body fileee.Reminder
}

// handleCreateReminder implementiert POST /v1/reminders — dünner Durchgriff auf
// Reminders.Create (fehlt Body.ID, generiert die Lib selbst eine ObjectId, siehe
// fileee/reminders.go).
func (s *Server) handleCreateReminder(ctx context.Context, in *createReminderInput) (*reminderOutput, error) {
	created, err := s.fc.Reminders.Create(ctx, &in.Body)
	if err != nil {
		return nil, mapError(err)
	}
	return &reminderOutput{Body: *created}, nil
}

// updateReminderInput steuert PUT /v1/reminders/{id}. Die Pfad-id ist maßgeblich — sie
// überschreibt ein eventuell abweichendes Body.ID (analog updateDocumentInput,
// handlers_documents.go).
type updateReminderInput struct {
	ID   string `path:"id" doc:"Erinnerungs-ID."`
	Body fileee.Reminder
}

// handleUpdateReminder implementiert PUT /v1/reminders/{id} — dünner Durchgriff auf
// Reminders.Update.
func (s *Server) handleUpdateReminder(ctx context.Context, in *updateReminderInput) (*reminderOutput, error) {
	r := in.Body
	r.ID = in.ID
	updated, err := s.fc.Reminders.Update(ctx, &r)
	if err != nil {
		return nil, mapError(err)
	}
	return &reminderOutput{Body: *updated}, nil
}

// createContactInput steuert POST /v1/contacts.
type createContactInput struct {
	Body fileee.Contact
}

// contactOutput ist der gemeinsame Response-Body von POST /v1/contacts und
// PUT /v1/contacts/{id}.
type contactOutput struct {
	Body fileee.Contact
}

// handleCreateContact implementiert POST /v1/contacts — dünner Durchgriff auf Contacts.Create
// (contactStatus defaultet in der Lib auf CUSTOM, wenn nicht gesetzt, siehe fileee/contacts.go).
func (s *Server) handleCreateContact(ctx context.Context, in *createContactInput) (*contactOutput, error) {
	created, err := s.fc.Contacts.Create(ctx, &in.Body)
	if err != nil {
		return nil, mapError(err)
	}
	return &contactOutput{Body: *created}, nil
}

// updateContactInput steuert PUT /v1/contacts/{id}. Die Pfad-id ist maßgeblich — sie überschreibt
// ein eventuell abweichendes Body.ID.
type updateContactInput struct {
	ID   string `path:"id" doc:"Kontakt-ID."`
	Body fileee.Contact
}

// handleUpdateContact implementiert PUT /v1/contacts/{id} — dünner Durchgriff auf
// Contacts.Update.
func (s *Server) handleUpdateContact(ctx context.Context, in *updateContactInput) (*contactOutput, error) {
	c := in.Body
	c.ID = in.ID
	updated, err := s.fc.Contacts.Update(ctx, &c)
	if err != nil {
		return nil, mapError(err)
	}
	return &contactOutput{Body: *updated}, nil
}
