package model

import "errors"

// Domain errors are shared by services and transport adapters.
var (
	ErrInvalidInput    = errors.New("invalid input")
	ErrNotFound        = errors.New("not found")
	ErrAlreadyExists   = errors.New("already exists")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
	ErrVersionConflict = errors.New("version conflict")
	ErrPayloadTooLarge = errors.New("payload too large")
)
