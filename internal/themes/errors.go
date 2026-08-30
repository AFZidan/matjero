package themes

import "errors"

// Theme Engine errors. They are mapped to HTTP semantics by the API layer:
//   - ErrNotFound        -> 404 (safe not-found, also used for cross-store access)
//   - ErrConflict        -> 409
//   - ErrInvalidInput    -> 400 (validation)
//   - ErrSchemaMismatch  -> 400 (config does not match theme version schema)
//   - ErrUnsafeContent   -> 400 (config contains executable content)
//   - ErrVersionImmutable -> 409 (attempt to mutate a published version)
var (
	ErrNotFound         = errors.New("theme entity not found")
	ErrConflict         = errors.New("theme conflict")
	ErrInvalidInput     = errors.New("invalid theme input")
	ErrForbidden        = errors.New("theme operation forbidden")
	ErrVersionImmutable = errors.New("theme version is immutable after publication")
	ErrSchemaMismatch   = errors.New("configuration does not match theme version schema")
	ErrUnsafeContent    = errors.New("configuration contains unsafe executable content")
)
