package main

import (
	"context"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// operationBodyTypeRecorder, wenn nicht nil, wird von registerOperation mit dem Go-Response-Body-
// Typ JEDER darüber registrierten Operation aufgerufen. Bleibt im normalen Serverbetrieb nil (kein
// Overhead, kein Verhaltensunterschied zu einem direkten huma.Register-Aufruf) —
// response_body_safety_test.go setzt ihn für die Dauer des Aufbaus eines *Server, um die REALE
// Liste registrierter Response-Body-Typen abzuleiten, statt sich auf eine handgepflegte Liste zu
// verlassen, die (Issue #43) unbemerkt von der tatsächlichen Verdrahtung abweichen kann.
var operationBodyTypeRecorder func(reflect.Type)

// registerOperation ist ein Drop-in-Wrapper um huma.Register mit IDENTISCHEM Verhalten — jeder
// huma.Register-Aufruf in diesem Codebase MUSS über diesen Wrapper laufen statt huma.Register
// direkt aufzurufen (Issue #43), damit response_body_safety_test.go aufzählen kann, was
// tatsächlich verdrahtet ist, statt sich auf eine Liste zu verlassen, die das nur behauptet.
func registerOperation[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	if operationBodyTypeRecorder != nil {
		operationBodyTypeRecorder(operationBodyType[O]())
	}
	huma.Register(api, op, handler)
}

// operationBodyType liefert den Go-Typ, der tatsächlich im JSON-Response-Body einer Operation
// erscheint: den Typ von O's Feld "Body", falls O eines hat (das etablierte Muster in diesem
// Codebase, z. B. documentListOutput.Body documentListBody) — sonst O selbst (body-lose
// Operationen, z. B. *struct{} für 204 No Content, und huma.StreamResponse, dessen eigenes
// "Body"-Feld ein func(ctx huma.Context)-Callback ist, kein marshalter Wert — als solches
// ungefährlich mitzuführen, da ein Funktionstyp niemals go-fileees fileeePackagePath tragen und
// niemals json.Marshaler implementieren kann).
func operationBodyType[O any]() reflect.Type {
	t := reflect.TypeFor[O]()
	if t.Kind() == reflect.Struct {
		if f, ok := t.FieldByName("Body"); ok {
			return f.Type
		}
	}
	return t
}
