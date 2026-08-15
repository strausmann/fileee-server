package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// probeOutput is a minimal Huma output struct with a "Body" field — the established pattern
// across this codebase's real routes (e.g. documentListOutput.Body documentListBody).
type probeOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// TestOperationBodyType_StructWithBodyField belegt den Regelfall: operationBodyType liefert den
// TYP des "Body"-Felds, nicht den Output-Typ selbst.
func TestOperationBodyType_StructWithBodyField(t *testing.T) {
	got := operationBodyType[probeOutput]()
	want := reflect.TypeOf(probeOutput{}.Body)
	if got != want {
		t.Errorf("operationBodyType[probeOutput]() = %v, want %v", got, want)
	}
}

// TestOperationBodyType_BodylessStruct belegt den body-losen Fall (z. B. *struct{} für 204 No
// Content, siehe handlers_destructive.go): ohne "Body"-Feld liefert operationBodyType den
// Output-Typ selbst zurück — hier struct{}, das trivialerweise nie fileeePackagePath trägt.
func TestOperationBodyType_BodylessStruct(t *testing.T) {
	got := operationBodyType[struct{}]()
	want := reflect.TypeOf(struct{}{})
	if got != want {
		t.Errorf("operationBodyType[struct{}]() = %v, want %v", got, want)
	}
}

// TestOperationBodyType_StreamResponse belegt den Sonderfall huma.StreamResponse
// (handleDownloadDocumentPDF/handleDownloadPageImage, handlers_documents.go): dessen EIGENES
// "Body"-Feld ist ein func(ctx huma.Context)-Callback, kein marshalter Wert — operationBodyType
// liefert also den Funktionstyp, der findFileeeMarshalerTypes garantiert nie triggert (weder
// Struct/Slice/Array/Map-Kind noch je fileeePackagePath).
func TestOperationBodyType_StreamResponse(t *testing.T) {
	got := operationBodyType[huma.StreamResponse]()
	want := reflect.TypeOf(huma.StreamResponse{}.Body)
	if got != want {
		t.Errorf("operationBodyType[huma.StreamResponse]() = %v, want %v", got, want)
	}
	if got.Kind() != reflect.Func {
		t.Errorf("Kind() = %v, want Func", got.Kind())
	}
}

// TestRegisterOperation_DelegatesToHumaRegister belegt, dass registerOperation ECHT
// registriert — d.h. sich exakt wie ein direkter huma.Register-Aufruf verhält — und NICHT nur
// den Recorder aufruft, ohne die Operation tatsächlich zu verdrahten. Ohne operationBodyTypeRecorder
// gesetzt (Normalbetrieb) darf sich am Verhalten nichts ändern.
func TestRegisterOperation_DelegatesToHumaRegister(t *testing.T) {
	mux := http.NewServeMux()
	api := newAPI(mux)

	registerOperation(api, huma.Operation{
		OperationID: "probe",
		Method:      http.MethodGet,
		Path:        "/probe",
	}, func(ctx context.Context, in *struct{}) (*probeOutput, error) {
		out := &probeOutput{}
		out.Body.Message = "ok"
		return out, nil
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/probe")
	if err != nil {
		t.Fatalf("GET /probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestRegisterOperation_RecordsBodyType belegt den eigentlichen Zweck (Issue #43): ist
// operationBodyTypeRecorder gesetzt, wird er bei registerOperation MIT DEM RICHTIGEN TYP
// aufgerufen — das ist der Mechanismus, über den
// registeredResponseBodyTypesFromRealServer (response_body_safety_test.go) die reale
// Registrierung abliest.
func TestRegisterOperation_RecordsBodyType(t *testing.T) {
	var recorded []reflect.Type
	operationBodyTypeRecorder = func(t reflect.Type) { recorded = append(recorded, t) }
	t.Cleanup(func() { operationBodyTypeRecorder = nil })

	api := newAPI(http.NewServeMux())
	registerOperation(api, huma.Operation{
		OperationID: "probe-record",
		Method:      http.MethodGet,
		Path:        "/probe-record",
	}, func(ctx context.Context, in *struct{}) (*probeOutput, error) {
		return &probeOutput{}, nil
	})

	if len(recorded) != 1 {
		t.Fatalf("recorded = %v, want genau 1 Eintrag", recorded)
	}
	want := reflect.TypeOf(probeOutput{}.Body)
	if recorded[0] != want {
		t.Errorf("recorded[0] = %v, want %v", recorded[0], want)
	}
}

// TestRegisterOperation_NilRecorderIsNoop belegt, dass ein nil operationBodyTypeRecorder (der
// Normalzustand im echten Serverbetrieb) registerOperation NICHT zum Absturz bringt — reine
// Absicherung gegen einen naiven "recorder(...)"-Aufruf ohne nil-Check.
func TestRegisterOperation_NilRecorderIsNoop(t *testing.T) {
	operationBodyTypeRecorder = nil

	api := newAPI(http.NewServeMux())
	registerOperation(api, huma.Operation{
		OperationID: "probe-nil-recorder",
		Method:      http.MethodGet,
		Path:        "/probe-nil-recorder",
	}, func(ctx context.Context, in *struct{}) (*probeOutput, error) {
		return &probeOutput{}, nil
	})
	// Kein Panic bis hierhin ist die Assertion.
}
