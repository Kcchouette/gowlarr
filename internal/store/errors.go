package store

import (
	"errors"
	"fmt"
)

// ErrNotFound is the shared root of every "missing entry" condition. Callers
// that do not care which kind of entry is absent check errors.Is against it.
var ErrNotFound = errors.New("store entry not found")

// ErrDefinitionNotFound and ErrIndexerConfigNotFound are distinct sentinel
// identities so errors.Is can tell a missing definition from a missing indexer
// config. Each wraps ErrNotFound, so errors.Is(err, ErrNotFound) stays true
// for both, and the rendered message keeps the shared "store entry not found"
// prefix. They are created via fmt.Errorf rather than assigned to ErrNotFound
// because two vars assigned to the same error value are a single identity:
// errors.Is would be unable to distinguish them.
var (
	ErrDefinitionNotFound    = fmt.Errorf("%w", ErrNotFound)
	ErrIndexerConfigNotFound = fmt.Errorf("%w", ErrNotFound)
)
