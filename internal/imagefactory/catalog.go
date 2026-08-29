package imagefactory

import (
	"fmt"
	"strings"
)

// ValidateExtensions checks every wanted extension name against a catalog
// fetched for the target Talos version.
//
// This is the check the Factory does not do. POSTing a schematic that names an
// extension which does not exist returns 201 and an ordinary id; the failure
// surfaces later, as a 400 on the ISO, by which point a UI has already told the
// operator the schematic was created.
//
// The catalog argument is a fetched list rather than a version string on
// purpose. A function that fetched its own catalog could fall back to a cached
// one when the fetch failed, and there must be no route from a failed catalog
// fetch to a POST: an extension list from the wrong version is worse than no
// list, because it validates successfully and produces an image that will not
// build.
//
// Every unknown name is reported at once. Reporting only the first turns
// fixing three typos into three round trips to an upstream that is not fast.
func ValidateExtensions(catalog []Extension, wanted []string) error {
	if len(wanted) == 0 {
		return nil
	}

	known := make(map[string]struct{}, len(catalog))
	for _, e := range catalog {
		known[e.Name] = struct{}{}
	}

	var unknown []string
	seen := make(map[string]struct{}, len(wanted))
	for _, name := range wanted {
		if _, ok := known[name]; ok {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		unknown = append(unknown, name)
	}

	if len(unknown) == 0 {
		return nil
	}
	return &UnknownExtensionsError{Names: unknown, CatalogSize: len(catalog)}
}

// UnknownExtensionsError carries every name the catalog did not contain.
//
// The names are a field rather than only a formatted string because a caller
// that has to report them per field -- an HTTP handler building an RFC 9457
// validation problem with one entry per bad name -- would otherwise have to
// re-derive them, which means a second implementation of the comparison this
// function already made. It satisfies errors.Is(err, ErrExtensionUnknown), so
// callers that only care about the class are unaffected.
type UnknownExtensionsError struct {
	// Names are the unknown extension names, in the order they were requested,
	// deduplicated.
	Names []string

	// CatalogSize is how many extensions the version-scoped catalog did list.
	// It is the difference between "your name is wrong" and "this version has
	// almost nothing", which are different things to tell an operator.
	CatalogSize int
}

func (e *UnknownExtensionsError) Error() string {
	return fmt.Sprintf("%s: %s (the catalog for this Talos version lists %d extensions)",
		ErrExtensionUnknown.Error(), strings.Join(e.Names, ", "), e.CatalogSize)
}

// Unwrap makes errors.Is(err, ErrExtensionUnknown) true, so this type is an
// addition to the sentinel rather than a replacement for it.
func (e *UnknownExtensionsError) Unwrap() error { return ErrExtensionUnknown }

// ExtensionNames projects a catalog onto its names, in catalog order, for a
// caller building a picker.
func ExtensionNames(catalog []Extension) []string {
	names := make([]string, 0, len(catalog))
	for _, e := range catalog {
		names = append(names, e.Name)
	}
	return names
}
