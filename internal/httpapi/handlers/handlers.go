// Package handlers holds the HTTP handlers, one file per resource.
//
// Handlers are thin by rule: decode, call a service, encode. A loop or a
// conditional about domain state belongs in a service, not here -- that is what
// keeps a future holzkubectl cheap.
//
// Each file exports its own Routes function and owns the URL shapes it serves.
// A wave-2 plan adds a route by adding it to its handler file and registering
// that file's Routes function at the composition root; router.go is not touched.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// maxBodyBytes caps a request body. Unbounded decoding of attacker-controlled
// input is a denial of service with no upside; nothing holzkube-manager accepts in this
// phase is anywhere near this size.
const maxBodyBytes = 64 << 10

// decodeJSON reads a JSON body under a hard size cap and rejects unknown fields
// so a typo in a client is a loud 400 rather than a silently ignored setting.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain exactly one JSON object")
	}
	return nil
}

// writeJSON encodes a success response.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Nothing this API returns is cacheable: /api/v1/auth/me carries the
	// operator's username and the audit page carries the archive.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

// handler is a small adapter so route tables read as data.
func handler(fn http.HandlerFunc) http.Handler { return fn }

// decodeProblem turns encoding/json's error into something an operator can act
// on.
//
// Eighteen routes answered a bad body with httpapi.Validation(err.Error()),
// which put sentences like
//
//	json: cannot unmarshal string into Go struct field schematicInput.meta.key of type uint8
//
// into a contracted API response. Three things are wrong with that and only one
// of them is cosmetic. It names Go types and this package's own struct names,
// which are not part of any contract and change without notice. It is not a
// sentence anybody outside this repository can act on. And it puts the field
// name inside prose, where the "errors" member of the problem exists precisely
// so a client can highlight the input that was wrong.
//
// So the decoder's judgement is kept -- it is the thing that actually knows
// which field failed -- and only its wording is replaced.
func decodeProblem(err error) *httpapi.Problem {
	var maxBytes *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError

	switch {
	case errors.Is(err, io.EOF):
		return httpapi.Validation("The request body is empty; this route needs a JSON object.")

	case errors.As(err, &maxBytes):
		return httpapi.Validation(fmt.Sprintf(
			"The request body is larger than the %d byte limit and was not read.", maxBytes.Limit))

	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			return httpapi.Validation(fmt.Sprintf(
				"The request body has a value of the wrong type: a %s was given where a %s was expected.",
				typeErr.Value, jsonTypeName(typeErr.Type)))
		}
		return httpapi.Validation(
			"A field in the request body has the wrong type.",
			httpapi.FieldError{
				Field:  field,
				Reason: fmt.Sprintf("was given a %s; it must be a %s", typeErr.Value, jsonTypeName(typeErr.Type)),
			})

	case errors.As(err, &syntaxErr):
		return httpapi.Validation(fmt.Sprintf(
			"The request body is not valid JSON; it stops making sense at byte %d.", syntaxErr.Offset))

	case strings.HasPrefix(err.Error(), unknownFieldPrefix):
		field := strings.Trim(strings.TrimPrefix(err.Error(), unknownFieldPrefix), `"`)
		return httpapi.Validation(
			"The request body carries a field this route does not accept. Unknown fields are refused "+
				"rather than ignored, so a misspelled setting is visible instead of silently absent.",
			httpapi.FieldError{Field: field, Reason: "not a field of this request"})
	}

	// Anything else: say what happened without repeating a message whose
	// wording this package does not control.
	return httpapi.Validation("The request body could not be read as JSON.")
}

// unknownFieldPrefix is how encoding/json says a field was not in the struct.
// A message that changes shape costs the field name and nothing else: the
// response is still a 400 that says the body could not be read.
const unknownFieldPrefix = "json: unknown field "

// jsonTypeName names a Go type the way JSON does, because "uint8" is not a
// thing a client sends.
func jsonTypeName(t reflect.Type) string {
	if t == nil {
		return "different type"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "different type"
	}
}
