package main

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/strausmann/go-fileee/fileee"
)

// registeredResponseBodyTypesFromRealServer builds a throwaway *Server with AllowDestructive true
// (so the three conditionally-registered Hard-DELETE routes are captured too) and returns the
// list of response body types that were ACTUALLY passed through registerOperation
// (operation_registry.go) while building it — see TestNoFileeeMarshalerTypeInAnyResponseBody
// below.
//
// # Why this test exists
//
// Security review on PR #38 (Issue #37, https://github.com/strausmann/fileee-server/pull/38) found
// a CRITICAL finding: documentListBody.Items was []fileee.Document. fileee.Document carries its
// own MarshalJSON (fileee/types.go) that unconditionally reconstructs the full wire envelope
// {"attributes":{"data":{...}}} — including RawExtra — for EVERY marshal, REGARDLESS of the
// field's own `json:"-"` tag (that tag only stops encoding/json's DEFAULT struct-field marshaling;
// a custom MarshalJSON method bypasses it entirely, by design of the Go json package). Every
// document returned by GET /v1/documents (search AND diff/sync mode) therefore leaked its full
// financial PII (IBAN, amounts, customer number, invoice number/date, sender, ...) — completely
// UNGATED, independent of the includeAttributes query param or the FILEEE_EXPOSE_ATTRIBUTES env
// gate that this same PR had correctly added for GET /v1/documents/{id}, POST /v1/documents, and
// PUT /v1/documents/{id}. The list/diff endpoint was simply never touched by that gate.
//
// The same audit (triggered by this finding) found the IDENTICAL, pre-existing, unrelated-to-
// Issue-#37 leak on GET /v1/companies: entityListBody[fileee.Company] put fileee.Company directly
// into Items, and fileee.Company has the exact same MarshalJSON pattern (fileee.CompanyAttributes:
// IBANs, VAT IDs, emails, phone numbers, websites, German tax IDs). Both are fixed (see
// documentListBody/mapDocuments in handlers_documents.go, companyListBody/handleListCompanies in
// handlers_entities.go) by projecting onto dedicated response DTOs (documentResponseBody,
// companyResponseBody) that mirror the safe, public fields only.
//
// A per-type unit test catches a REGRESSION on a type we already know to check. It does NOT catch
// a NEW route being added tomorrow with a NEW go-fileee type that happens to carry (now, or in a
// future go-fileee release) its own MarshalJSON. That is exactly the gap this test closes: it
// walks the full type graph of every registered response body (struct fields, slice/array/map
// elements, pointer indirection) and fails the build the moment ANY type belonging to
// go-fileee's `fileee` package that implements json.Marshaler shows up ANYWHERE in that graph —
// whether or not it currently carries PII. (pageBody in attributes.go exists precisely because of
// this: fileee.Page's flexInt64 fields implement MarshalJSON too, carry no PII at all, and were
// still replaced — the invariant is kept absolute on purpose, so nobody has to argue a case-by-case
// exception under time pressure again.)
//
// # Issue #43: derived from the real registration, not a hand-maintained list
//
// Until this fix, the input to this walk was a hand-maintained []reflect.Type literal that a
// developer had to remember to extend whenever a new route was added. A future "cleanup" that
// inlines a go-fileee Marshaler type directly into a route's output struct would slip through BOTH
// the per-type HTTP regression tests (blind to top-level bodies — see pii_leak_regression_test.go,
// TestGetCompany_NeverLeaksAttributes doc comment) AND this walk, simply because nobody updated the
// list. registeredResponseBodyTypesFromRealServer closes that gap: it builds an actual *Server,
// which registers its actual routes through registerOperation (the ONLY sanctioned way to call
// huma.Register in this codebase, see operation_registry.go), and reads back exactly the body
// types that registration produced — so a new or changed route is covered automatically, with
// nothing to remember to update here.
func registeredResponseBodyTypesFromRealServer(t *testing.T) []reflect.Type {
	t.Helper()

	var types []reflect.Type
	operationBodyTypeRecorder = func(rt reflect.Type) { types = append(types, rt) }
	defer func() { operationBodyTypeRecorder = nil }()

	// AllowDestructive:true captures the three Hard-DELETE routes too (server.go only calls
	// registerDestructiveRoutes inside that guard) — exhaustiveness over "everything that CAN be
	// registered", not just the default-config subset.
	newTestServerWithConfig(t, Config{AllowDestructive: true}, nil)

	return types
}

// jsonMarshalerType is the reflect.Type of the standard library's json.Marshaler interface —
// implemented by exactly the go-fileee types this test hunts for (Document, Company,
// DocumentAttributes, CompanyAttributes, the unexported flexInt64 — see fileee/types.go).
var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// fileeePackagePath is go-fileee's fully-qualified import path, used to scope the search to that
// package specifically — this test is not concerned with stdlib types (e.g. json.RawMessage,
// which itself implements json.Marshaler) or any other dependency's Marshaler types.
const fileeePackagePath = "github.com/strausmann/go-fileee/fileee"

// typeImplementsJSONMarshaler reports whether t (or a pointer to t) implements json.Marshaler —
// covers both value-receiver methods (fileee.Document, fileee.Company, ...) and, defensively, any
// future pointer-receiver MarshalJSON implementation.
func typeImplementsJSONMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType)
}

// findFileeeMarshalerTypes walks t — following struct fields (skipping unexported fields and
// fields tagged `json:"-"`, since encoding/json never marshals either), slice/array/map element
// types, and pointer indirection — and records every DISTINCT type belonging to go-fileee's
// `fileee` package that implements json.Marshaler into found. visited prevents revisiting the same
// type twice and guards against infinite recursion on self-referential types.
//
// Skipping `json:"-"`-tagged fields is deliberate and important: such a field's value is NEVER
// passed to encoding/json's marshaling machinery for that field, so whatever type it holds can
// never leak through THIS field, regardless of that type's own Marshaler. (This is exactly the
// property that a custom MarshalJSON on the OUTER struct can violate — which is the whole point of
// this test: fileee.Document/Company have `json:"-"` on Attributes yet leak it anyway, because
// their OWN MarshalJSON ignores that tag and reconstructs the field manually. That's why this
// walker flags the OUTER type itself (Document/Company) the moment it's encountered — not by
// inspecting its json:"-" field, but by checking the type itself against jsonMarshalerType before
// descending into its fields at all.)
func findFileeeMarshalerTypes(t reflect.Type, visited map[reflect.Type]bool, found map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if visited[t] {
		return
	}
	visited[t] = true

	if t.PkgPath() == fileeePackagePath && typeImplementsJSONMarshaler(t) {
		found[t] = true
		// Deliberately continue descending below even though t is already flagged: a Marshaler
		// type could itself embed ANOTHER go-fileee Marshaler type we'd otherwise miss, and
		// finding every offender in one run (instead of one-at-a-time across repeated test runs)
		// is strictly more useful for whoever has to fix it.
	}

	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if tag, ok := f.Tag.Lookup("json"); ok {
				name, _, _ := strings.Cut(tag, ",")
				if name == "-" {
					continue
				}
			}
			findFileeeMarshalerTypes(f.Type, visited, found)
		}
	case reflect.Slice, reflect.Array:
		findFileeeMarshalerTypes(t.Elem(), visited, found)
	case reflect.Map:
		findFileeeMarshalerTypes(t.Elem(), visited, found)
	}
}

// TestNoFileeeMarshalerTypeInAnyResponseBody is the structural guardrail described in the doc
// comment on registeredResponseBodyTypes above: it fails the moment ANY currently-registered
// response body type contains — directly or transitively — a go-fileee type with its own
// MarshalJSON. Run as part of the normal `go test ./...` suite, so it fails at development time,
// not first in review.
func TestNoFileeeMarshalerTypeInAnyResponseBody(t *testing.T) {
	all := registeredResponseBodyTypesFromRealServer(t)
	if len(all) == 0 {
		t.Fatal("registeredResponseBodyTypesFromRealServer returned nothing — registerOperation wiring itself is broken, this test would otherwise pass vacuously")
	}

	// Dedupe (several routes legitimately share the same body type, e.g. reminderOutput for both
	// create-reminder and update-reminder) so each distinct type gets exactly one, stably-named
	// subtest instead of testing.T auto-suffixing duplicates ("...#01").
	seen := map[reflect.Type]bool{}
	unique := make([]reflect.Type, 0, len(all))
	for _, typ := range all {
		if !seen[typ] {
			seen[typ] = true
			unique = append(unique, typ)
		}
	}

	for _, typ := range unique {
		t.Run(typ.String(), func(t *testing.T) {
			visited := map[reflect.Type]bool{}
			found := map[reflect.Type]bool{}
			findFileeeMarshalerTypes(typ, visited, found)
			if len(found) == 0 {
				return
			}
			names := make([]string, 0, len(found))
			for ft := range found {
				names = append(names, ft.String())
			}
			sort.Strings(names)
			t.Errorf("response body type %s contains go-fileee type(s) with their own MarshalJSON: %v — "+
				"these bypass json:\"-\" tags and leak EVERYTHING on marshal (see the incident documented "+
				"on registeredResponseBodyTypesFromRealServer). Project onto a dedicated response DTO "+
				"instead (see documentResponseBody/newDocumentResponseBody, "+
				"companyResponseBody/newCompanyResponseBody, pageBody/mapPages for the established "+
				"pattern).", typ, names)
		})
	}
}

// TestFindFileeeMarshalerTypes_DetectsKnownOffenders is a meta-test for the guardrail mechanism
// itself: it proves findFileeeMarshalerTypes actually DETECTS the exact shapes that caused the
// PR #38 incident, so a change that accidentally weakens the walker (e.g. an overly broad `json:"-"`
// skip, or a Kind case that stops recursing) fails loudly instead of leaving
// TestNoFileeeMarshalerTypeInAnyResponseBody vacuously green.
func TestFindFileeeMarshalerTypes_DetectsKnownOffenders(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want string // reflect.Type.String() of the expected offender
	}{
		{"direct Document", reflect.TypeOf(fileee.Document{}), "fileee.Document"},
		{"slice of Document (the PR #38 shape)", reflect.TypeOf([]fileee.Document{}), "fileee.Document"},
		{"direct Company", reflect.TypeOf(fileee.Company{}), "fileee.Company"},
		{"slice of Company (the list-companies shape)", reflect.TypeOf([]fileee.Company{}), "fileee.Company"},
		{"struct field holding a Document", reflect.TypeOf(struct{ Doc fileee.Document }{}), "fileee.Document"},
		{"pointer to Document", reflect.TypeOf(&fileee.Document{}), "fileee.Document"},
		{"Page (flexInt64 fields — no PII, still flagged)", reflect.TypeOf(fileee.Page{}), "fileee.flexInt64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visited := map[reflect.Type]bool{}
			found := map[reflect.Type]bool{}
			findFileeeMarshalerTypes(tc.typ, visited, found)
			if len(found) == 0 {
				t.Fatalf("expected findFileeeMarshalerTypes to flag %s, found nothing — the guardrail mechanism itself is broken", tc.want)
			}
			hit := false
			for ft := range found {
				if ft.String() == tc.want {
					hit = true
				}
			}
			if !hit {
				got := make([]string, 0, len(found))
				for ft := range found {
					got = append(got, ft.String())
				}
				t.Fatalf("expected %s among findings, got %v", tc.want, got)
			}
		})
	}
}

// TestFindFileeeMarshalerTypes_JSONDashTagStopsDescent proves the `json:"-"` skip in
// findFileeeMarshalerTypes only stops DESCENDING INTO that field — it must NOT suppress flagging
// the outer type itself if the outer type has its own MarshalJSON (exactly fileee.Document's
// shape: Attributes DocumentAttributes `json:"-"`, but Document.MarshalJSON ignores that tag).
func TestFindFileeeMarshalerTypes_JSONDashTagStopsDescent(t *testing.T) {
	visited := map[reflect.Type]bool{}
	found := map[reflect.Type]bool{}
	findFileeeMarshalerTypes(reflect.TypeOf(fileee.Document{}), visited, found)

	if !found[reflect.TypeOf(fileee.Document{})] {
		t.Fatal("fileee.Document itself must be flagged regardless of its json:\"-\"-tagged Attributes field")
	}
}

// TestNoFileeeMarshalerType_KnownSafeTypesPassCleanly is the positive counterpart: proves the
// already-safe types (that were never part of the incident, or were already fixed by this PR)
// produce ZERO findings — a sanity check that the walker isn't over-broad and doesn't false-positive
// on ordinary go-fileee types.
func TestNoFileeeMarshalerType_KnownSafeTypesPassCleanly(t *testing.T) {
	safe := []reflect.Type{
		reflect.TypeOf(fileee.Tag{}),
		reflect.TypeOf(fileee.Contact{}),
		reflect.TypeOf(fileee.Box{}),
		reflect.TypeOf(fileee.Reminder{}),
		reflect.TypeOf(fileee.Conversation{}),
		reflect.TypeOf(fileee.SentMessage{}),
		reflect.TypeOf(fileee.Share{}),
		reflect.TypeOf(fileee.SharedObject{}),
		reflect.TypeOf([]fileee.OCRToken{}),
		reflect.TypeOf(documentResponseBody{}), // the FIXED type — must be clean after this PR
		reflect.TypeOf(companyResponseBody{}),  // the FIXED type — must be clean after this PR
		reflect.TypeOf(pageBody{}),             // the FIXED type — must be clean after this PR
	}
	for _, typ := range safe {
		t.Run(typ.String(), func(t *testing.T) {
			visited := map[reflect.Type]bool{}
			found := map[reflect.Type]bool{}
			findFileeeMarshalerTypes(typ, visited, found)
			if len(found) != 0 {
				names := make([]string, 0, len(found))
				for ft := range found {
					names = append(names, ft.String())
				}
				t.Fatalf("expected no findings for %s, got %v", typ, names)
			}
		})
	}
}
