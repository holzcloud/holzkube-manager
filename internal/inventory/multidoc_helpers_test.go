package inventory

import (
	"errors"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
)

// deriveSecretsWithoutSelection is deriveSecrets as it was before the document
// selection: it hands machinery the whole multi-document file.
//
// It exists so the reproduction is part of the test rather than a claim in a
// comment — the test asserts this still fails before asserting the real one
// succeeds, so a machinery that learned the kind cannot leave the test passing
// while proving nothing.
func deriveSecretsWithoutSelection(configYAML string) (*secrets.Bundle, error) {
	provider, err := configloader.NewFromBytes([]byte(configYAML))
	if err != nil {
		return nil, err
	}
	if provider.RawV1Alpha1() == nil {
		return nil, errors.New("no v1alpha1")
	}
	return secrets.NewBundleFromConfig(secrets.NewClock(), provider), nil
}

func isNotControlPlane(err error) bool { return errors.Is(err, ErrNotControlPlane) }
