package model

const (
	// CurrentRecordEncryptionVersion is the encrypted record format currently accepted by client and server.
	CurrentRecordEncryptionVersion uint32 = 1
	// RecordNonceSize is the AES-GCM nonce size used by the current encrypted record format.
	RecordNonceSize = 12
	// RecordAuthenticationTagSize is the minimum AES-GCM ciphertext overhead for the current format.
	RecordAuthenticationTagSize = 16
)
