package store

import "errors"

var ErrNotFound = errors.New("store entry not found")

var (
	ErrDefinitionNotFound    = ErrNotFound
	ErrIndexerConfigNotFound = ErrNotFound
)
