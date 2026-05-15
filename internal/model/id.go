package model

import (
	"fmt"
	"strings"
)

// ID is a canonical UUID string used by GophKeeper domain entities.
type ID string

// ParseID validates and returns a canonical UUID identifier.
func ParseID(value string) (ID, error) {
	if !isCanonicalUUID(value) {
		return "", fmt.Errorf("%w: invalid UUID %q", ErrInvalidInput, value)
	}
	return ID(strings.ToLower(value)), nil
}

// String returns the canonical textual representation of the identifier.
func (id ID) String() string {
	return string(id)
}

// IsZero reports whether the identifier is empty.
func (id ID) IsZero() bool {
	return id == ""
}

func isCanonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}

	for i := range value {
		switch i {
		case 8, 13, 18, 23:
			if value[i] != '-' {
				return false
			}
		default:
			if !isHex(value[i]) {
				return false
			}
		}
	}

	return true
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}
