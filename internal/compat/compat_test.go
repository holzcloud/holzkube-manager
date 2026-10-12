package compat_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/holzcloud/holzkube-manager/internal/compat"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// TestEverySupportedTalosVersionHasAWindow is the join between the two tables
// D-25 keeps apart.
//
// They say different things and neither may be derived from the other, but
// they do have to overlap in one direction: a Talos version holzkube-manager claims to
// support and cannot make a compatibility statement about would show an
// operator "no statement is made" on a version the product advertises.
func TestEverySupportedTalosVersionHasAWindow(t *testing.T) {
	t.Parallel()

	for _, v := range []string{talos.MinSupportedVersion, talos.MaxSupportedVersion} {
		if got := compat.Check(v, "v1.34.1"); !got.Known {
			t.Errorf("Talos %s is inside the supported range but has no compatibility window: %s", v, got.Sentence)
		}
	}
}

func TestCheckClassifiesTheWindow(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name      string
		talos     string
		k8s       string
		supported bool
		headroom  int
	}{
		{name: "inside with headroom", talos: "v1.13.9", k8s: "v1.34.4", supported: true, headroom: 2},
		{name: "at the top", talos: "v1.13.9", k8s: "v1.36.1", supported: true, headroom: 0},
		{name: "above the top", talos: "v1.13.9", k8s: "v1.37.0", supported: false, headroom: -1},
		{name: "below the floor", talos: "v1.13.9", k8s: "v1.30.0", supported: false, headroom: 6},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			got := compat.Check(row.talos, row.k8s)
			if !got.Known {
				t.Fatalf("no verdict for %s / %s", row.talos, row.k8s)
			}
			if got.Supported != row.supported {
				t.Errorf("Supported = %v, want %v (%s)", got.Supported, row.supported, got.Sentence)
			}
			if got.HeadroomMinors != row.headroom {
				t.Errorf("HeadroomMinors = %d, want %d", got.HeadroomMinors, row.headroom)
			}
			if got.Sentence == "" {
				t.Error("the verdict carries no sentence; D-24 asks for the number and the words")
			}
		})
	}
}

// TestUnknownVersionsAreNotGuessed is the property that keeps a phase 9
// upgrade from being authorised by an extrapolation.
func TestUnknownVersionsAreNotGuessed(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"v1.99.0", "not-a-version", "", "v2.0.0"} {
		got := compat.Check(v, "v1.34.1")
		if got.Known {
			t.Errorf("Check(%q) claimed a window; an uncurated version has none", v)
		}
		if !strings.Contains(got.Sentence, "no statement") && !strings.Contains(got.Sentence, "could not be read") {
			t.Errorf("Check(%q) said %q, which does not tell the operator that nothing is known", v, got.Sentence)
		}
	}
}

// TestWindowsAreWellFormed walks the whole table so that a row added with its
// bounds the wrong way round, or a default outside its own window, is a red
// test rather than a wrong number on a screen.
func TestWindowsAreWellFormed(t *testing.T) {
	t.Parallel()

	for _, m := range compat.TalosMinors() {
		v := compat.Check("v1."+itoa(m)+".0", "v1.34.0")
		if !v.Known {
			t.Fatalf("Talos minor %d is in the table but Check does not know it", m)
		}
		w := v.Window
		if w.MinMinor > w.MaxMinor {
			t.Errorf("Talos 1.%d: window %d..%d is inverted", m, w.MinMinor, w.MaxMinor)
		}
		if w.DefaultMinor < w.MinMinor || w.DefaultMinor > w.MaxMinor {
			t.Errorf("Talos 1.%d: default 1.%d is outside its own window %d..%d",
				m, w.DefaultMinor, w.MinMinor, w.MaxMinor)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestTheNewestRowMatchesTheCompiledMachinery holds the matrix row of the
// machinery this binary is built with to that library's own constants. The
// matrix is edited by hand; on 2026-10-12 it still said Kubernetes 1.35 for a
// Talos whose machinery defaults to 1.37, and nothing noticed.
//
// The top and the default must equal the machinery's default, and the bottom is
// its count of supported minors back from it. (The Talos documentation lists one
// fewer at the bottom; the operator chose the machinery's, because it is what
// the code allows and a working cluster is not reported as outside.)
func TestTheNewestRowMatchesTheCompiledMachinery(t *testing.T) {
	t.Parallel()

	def := strings.SplitN(strings.TrimPrefix(constants.DefaultKubernetesVersion, "1."), ".", 2)[0]
	defMinor, err := strconv.Atoi(def)
	if err != nil {
		t.Fatalf("machinery default %q: %v", constants.DefaultKubernetesVersion, err)
	}

	v := compat.Check(gendata.VersionTag, constants.DefaultKubernetesVersion)
	if !v.Known {
		t.Fatalf("the compiled machinery's Talos %s has no row in the matrix", gendata.VersionTag)
	}
	if v.Window.MaxMinor != defMinor || v.Window.DefaultMinor != defMinor {
		t.Errorf("Talos %s row top/default = %d/%d, the compiled machinery defaults to Kubernetes 1.%d",
			gendata.VersionTag, v.Window.MaxMinor, v.Window.DefaultMinor, defMinor)
	}
	if want := defMinor - constants.SupportedKubernetesVersions + 1; v.Window.MinMinor != want {
		t.Errorf("Talos %s row bottom = %d, want %d (default %d, %d minors supported)",
			gendata.VersionTag, v.Window.MinMinor, want, defMinor, constants.SupportedKubernetesVersions)
	}
}
