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
	"io"
	"net/http"
)

// maxBodyBytes caps a request body. Unbounded decoding of attacker-controlled
// input is a denial of service with no upside; nothing holzkube accepts in this
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
