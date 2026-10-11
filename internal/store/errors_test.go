package store

import (
	"errors"
	"testing"
)

func TestSentinelIdentitiesAreDistinct(t *testing.T) {
	if errors.Is(ErrDefinitionNotFound, ErrIndexerConfigNotFound) {
		t.Error("ErrDefinitionNotFound must not match ErrIndexerConfigNotFound")
	}
	if errors.Is(ErrIndexerConfigNotFound, ErrDefinitionNotFound) {
		t.Error("ErrIndexerConfigNotFound must not match ErrDefinitionNotFound")
	}
}

func TestSentinelsKeepSharedNotFoundRoot(t *testing.T) {
	for _, err := range []error{ErrDefinitionNotFound, ErrIndexerConfigNotFound} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%v must stay matchable via errors.Is(err, ErrNotFound)", err)
		}
		// The rendered message must keep the shared prefix so existing
		// user-visible error text is unchanged.
		if got, want := err.Error(), "store entry not found"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	}
}
