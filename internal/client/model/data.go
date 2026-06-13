package model

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// MaxTitleLength is the maximum title length in bytes.
	MaxTitleLength = 512
	// MaxMetadataLength is the maximum plaintext metadata length in bytes.
	MaxMetadataLength = 64 * 1024
	// MaxCredentialLoginLength is the maximum credentials login length in bytes.
	MaxCredentialLoginLength = 1024
	// MaxCredentialPasswordLength is the maximum credentials password length in bytes.
	MaxCredentialPasswordLength = 16 * 1024
	// MaxTextBodyLength is the maximum plaintext text body length in bytes.
	MaxTextBodyLength = 10 * 1024 * 1024
	// MaxFilenameLength is the maximum stored filename length in bytes.
	MaxFilenameLength = 255
	// MaxMIMETypeLength is the maximum MIME type length in bytes.
	MaxMIMETypeLength = 255
	// MaxCardHolderLength is the maximum card holder length in bytes.
	MaxCardHolderLength = 255
)

var expiryPattern = regexp.MustCompile(`^(0[1-9]|1[0-2])/[0-9]{2}$`)

// Metadata contains arbitrary textual information associated with a record.
type Metadata struct {
	Text string `json:"text"`
}

// Validate checks the metadata size.
func (metadata Metadata) Validate() error {
	if len(metadata.Text) > MaxMetadataLength {
		return fmt.Errorf("metadata exceeds %d bytes", MaxMetadataLength)
	}
	return nil
}

// Credentials contains plaintext authentication data for a service.
type Credentials struct {
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Validate checks required credentials fields.
func (credentials Credentials) Validate() error {
	if err := validateTitle(credentials.Name); err != nil {
		return fmt.Errorf("credentials name: %w", err)
	}
	if strings.TrimSpace(credentials.Login) == "" {
		return fmt.Errorf("credentials login is required")
	}
	if len(credentials.Login) > MaxCredentialLoginLength {
		return fmt.Errorf("credentials login exceeds %d bytes", MaxCredentialLoginLength)
	}
	if credentials.Password == "" {
		return fmt.Errorf("credentials password is required")
	}
	if len(credentials.Password) > MaxCredentialPasswordLength {
		return fmt.Errorf("credentials password exceeds %d bytes", MaxCredentialPasswordLength)
	}
	return nil
}

// Text contains a plaintext text record.
type Text struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Validate checks required text record fields.
func (text Text) Validate() error {
	if err := validateTitle(text.Title); err != nil {
		return fmt.Errorf("text title: %w", err)
	}
	if text.Body == "" {
		return fmt.Errorf("text body is required")
	}
	if len(text.Body) > MaxTextBodyLength {
		return fmt.Errorf("text body exceeds %d bytes", MaxTextBodyLength)
	}
	return nil
}

// Binary contains a plaintext binary file before encryption.
type Binary struct {
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

// Validate checks required binary record fields.
func (binary Binary) Validate(maxSize int64) error {
	filename := strings.TrimSpace(binary.Filename)
	if filename == "" {
		return fmt.Errorf("binary filename is required")
	}
	if filename != filepath.Base(filename) || filename == "." || filename == ".." {
		return fmt.Errorf("binary filename must not contain a path")
	}
	if len(filename) > MaxFilenameLength {
		return fmt.Errorf("binary filename exceeds %d bytes", MaxFilenameLength)
	}
	if strings.TrimSpace(binary.MIMEType) == "" {
		return fmt.Errorf("binary MIME type is required")
	}
	if len(binary.MIMEType) > MaxMIMETypeLength {
		return fmt.Errorf("binary MIME type exceeds %d bytes", MaxMIMETypeLength)
	}
	if len(binary.Data) == 0 {
		return fmt.Errorf("binary data is required")
	}
	if maxSize <= 0 {
		return fmt.Errorf("maximum binary size must be positive")
	}
	if int64(len(binary.Data)) > maxSize {
		return fmt.Errorf("binary data exceeds %d bytes", maxSize)
	}
	return nil
}

// BankCard contains plaintext bank card details before encryption.
type BankCard struct {
	Name       string `json:"name"`
	Number     string `json:"number"`
	Holder     string `json:"holder"`
	ExpiryDate string `json:"expiry_date"`
	CVV        string `json:"cvv"`
}

// Validate checks bank card fields without logging their values.
func (card BankCard) Validate() error {
	if err := validateTitle(card.Name); err != nil {
		return fmt.Errorf("bank card name: %w", err)
	}
	digits, ok := cardDigits(card.Number)
	if !ok || len(digits) < 12 || len(digits) > 19 {
		return fmt.Errorf("bank card number must contain 12 to 19 digits")
	}
	if strings.TrimSpace(card.Holder) == "" {
		return fmt.Errorf("bank card holder is required")
	}
	if len(card.Holder) > MaxCardHolderLength {
		return fmt.Errorf("bank card holder exceeds %d bytes", MaxCardHolderLength)
	}
	if !expiryPattern.MatchString(card.ExpiryDate) {
		return fmt.Errorf("bank card expiry date must use MM/YY format")
	}
	if len(card.CVV) != 3 && len(card.CVV) != 4 {
		return fmt.Errorf("bank card CVV must contain 3 or 4 digits")
	}
	for i := range card.CVV {
		if card.CVV[i] < '0' || card.CVV[i] > '9' {
			return fmt.Errorf("bank card CVV must contain digits only")
		}
	}
	return nil
}

// MaskedNumber returns a display-safe representation of the card number.
func (card BankCard) MaskedNumber() string {
	digits, ok := cardDigits(card.Number)
	if !ok || len(digits) < 4 {
		return "****"
	}
	return "**** " + digits[len(digits)-4:]
}

func cardDigits(number string) (string, bool) {
	var builder strings.Builder
	builder.Grow(len(number))
	for i := range number {
		switch {
		case number[i] >= '0' && number[i] <= '9':
			builder.WriteByte(number[i])
		case number[i] == ' ' || number[i] == '-':
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func validateTitle(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value is required")
	}
	if len(value) > MaxTitleLength {
		return fmt.Errorf("value exceeds %d bytes", MaxTitleLength)
	}
	return nil
}
