// Package auth provides server-side password hashing and session token helpers.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	defaultMemory      = 64 * 1024
	defaultIterations  = 3
	defaultParallelism = 2
	defaultSaltLength  = 16
	defaultKeyLength   = 32

	maxMemory      = 1024 * 1024
	maxIterations  = 10
	maxParallelism = 32
	maxSaltLength  = 1024
	maxHashLength  = 1024
)

// Argon2idHasher hashes and verifies account passwords using an encoded PHC-like format.
type Argon2idHasher struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
	Random      io.Reader
}

// NewArgon2idHasher creates a password hasher with production defaults.
func NewArgon2idHasher() Argon2idHasher {
	return Argon2idHasher{
		Memory:      defaultMemory,
		Iterations:  defaultIterations,
		Parallelism: defaultParallelism,
		SaltLength:  defaultSaltLength,
		KeyLength:   defaultKeyLength,
		Random:      rand.Reader,
	}
}

// Hash creates a salted Argon2id password hash.
func (hasher Argon2idHasher) Hash(password string) (string, error) {
	if err := hasher.validate(); err != nil {
		return "", err
	}

	salt := make([]byte, hasher.SaltLength)
	if _, err := io.ReadFull(hasher.Random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, hasher.Iterations, hasher.Memory, hasher.Parallelism, hasher.KeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		hasher.Memory,
		hasher.Iterations,
		hasher.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Verify checks a password against an encoded Argon2id hash.
func (hasher Argon2idHasher) Verify(password, encoded string) (bool, error) {
	parameters, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		parameters.iterations,
		parameters.memory,
		parameters.parallelism,
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func (hasher Argon2idHasher) validate() error {
	if hasher.Memory == 0 || hasher.Iterations == 0 || hasher.Parallelism == 0 || hasher.SaltLength == 0 || hasher.KeyLength == 0 {
		return errors.New("argon2id parameters must be positive")
	}
	if hasher.Random == nil {
		return errors.New("argon2id random source is required")
	}
	return nil
}

type passwordHashParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordHash(encoded string) (passwordHashParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return passwordHashParameters{}, nil, nil, errors.New("invalid argon2id hash format")
	}

	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return passwordHashParameters{}, nil, nil, errors.New("unsupported argon2id version")
	}

	var parameters passwordHashParameters
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.memory, &parameters.iterations, &parameters.parallelism); err != nil {
		return passwordHashParameters{}, nil, nil, fmt.Errorf("parse argon2id parameters: %w", err)
	}
	if parameters.memory == 0 || parameters.iterations == 0 || parameters.parallelism == 0 {
		return passwordHashParameters{}, nil, nil, errors.New("invalid argon2id parameters")
	}
	if parameters.memory > maxMemory || parameters.iterations > maxIterations || parameters.parallelism > maxParallelism {
		return passwordHashParameters{}, nil, nil, errors.New("argon2id parameters exceed safety limits")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 || len(salt) > maxSaltLength {
		return passwordHashParameters{}, nil, nil, errors.New("invalid argon2id salt")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) == 0 || len(hash) > maxHashLength {
		return passwordHashParameters{}, nil, nil, errors.New("invalid argon2id hash")
	}
	return parameters, salt, hash, nil
}
