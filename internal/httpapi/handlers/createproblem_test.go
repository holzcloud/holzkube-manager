package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
)

// Ledger 55: createProblem's NotRepresentableError branch is unreachable from
// the HTTP route, because refuseUnrepresentable already covers every document
// path the request vocabulary names. It stays for the day a new field forgets
// its check -- and a branch kept for that day has to be tested from here, since
// no route-level test can reach it.
//
// What it must do on that day is the whole of G-02-6: answer 400, never the 502
// that tells an operator to retry something that can never succeed.
func TestAnUnrepresentableValueIsTheRequestsFaultNotTheFactorys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		path      string
		index     int
		wantField string
	}{
		{"a path the request names", "customization.extraKernelArgs", 1, "kernel_args"},
		{"a scalar path the request names", "customization.meta.value", -1, "meta"},
		{"a path no request field maps to yet", "customization.someFutureField", -1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := fmt.Errorf("creating: %w", &imagefactory.NotRepresentableError{
				Path: c.path, Index: c.index, Reason: "contains a control character",
			})

			p := createProblem(err)

			if p.Status != http.StatusBadRequest {
				t.Fatalf("status %d (%s), want 400: an unrepresentable value is never the Factory's fault",
					p.Status, p.Code)
			}
			if len(p.Errors) != 1 {
				t.Fatalf("%d field errors, want exactly 1: %+v", len(p.Errors), p.Errors)
			}
			if p.Errors[0].Field != c.wantField {
				t.Errorf("field %q, want %q", p.Errors[0].Field, c.wantField)
			}
			if p.Errors[0].Reason == "" {
				t.Error("the field error names no reason")
			}
		})
	}
}
