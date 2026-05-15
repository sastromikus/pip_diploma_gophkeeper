package model

import (
	"fmt"
	"strings"
)

const (
	// MaxTitleLength is the maximum title length in bytes.
	MaxTitleLength = 512
	// MaxMetadataLength is the maximum plaintext metadata length in bytes.
	MaxMetadataLength = 64 * 1024
)

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
	if credentials.Password == "" {
		return fmt.Errorf("credentials password is required")
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
	if strings.TrimSpace(binary.Filename) == "" {
		return fmt.Errorf("binary filename is required")
	}
	if strings.TrimSpace(binary.MIMEType) == "" {
		return fmt.Errorf("binary MIME type is required")
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

// Validate checks required bank card fields without logging their values.
func (card BankCard) Validate() error {
	if err := validateTitle(card.Name); err != nil {
		return fmt.Errorf("bank card name: %w", err)
	}
	if strings.TrimSpace(card.Number) == "" {
		return fmt.Errorf("bank card number is required")
	}
	if strings.TrimSpace(card.Holder) == "" {
		return fmt.Errorf("bank card holder is required")
	}
	if strings.TrimSpace(card.ExpiryDate) == "" {
		return fmt.Errorf("bank card expiry date is required")
	}
	if strings.TrimSpace(card.CVV) == "" {
		return fmt.Errorf("bank card CVV is required")
	}
	return nil
}

// MaskedNumber returns a display-safe representation of the card number.
func (card BankCard) MaskedNumber() string {
	digits := make([]byte, 0, len(card.Number))
	for i := range card.Number {
		if card.Number[i] >= '0' && card.Number[i] <= '9' {
			digits = append(digits, card.Number[i])
		}
	}
	if len(digits) <= 4 {
		return string(digits)
	}
	return "**** " + string(digits[len(digits)-4:])
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
