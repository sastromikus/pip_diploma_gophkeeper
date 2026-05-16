package postgres

import (
	"fmt"

	"github.com/sastromikus/gophkeeper/internal/model"
)

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (model.User, error) {
	var user model.User
	var id string
	if err := row.Scan(
		&id,
		&user.Login,
		&user.PasswordHash,
		&user.EncryptedDataKey,
		&user.KeySalt,
		&user.KeyNonce,
		&user.KeyDerivationVersion,
		&user.CreatedAt,
	); err != nil {
		return model.User{}, err
	}
	parsedID, err := model.ParseID(id)
	if err != nil {
		return model.User{}, fmt.Errorf("parse stored user ID: %w", err)
	}
	user.ID = parsedID
	return user, nil
}

func scanSession(row scanner) (model.Session, error) {
	var session model.Session
	var id, userID string
	if err := row.Scan(
		&id,
		&userID,
		&session.TokenHash,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
	); err != nil {
		return model.Session{}, err
	}
	parsedID, err := model.ParseID(id)
	if err != nil {
		return model.Session{}, fmt.Errorf("parse stored session ID: %w", err)
	}
	parsedUserID, err := model.ParseID(userID)
	if err != nil {
		return model.Session{}, fmt.Errorf("parse stored session user ID: %w", err)
	}
	session.ID = parsedID
	session.UserID = parsedUserID
	return session, nil
}

func scanRecord(row scanner) (model.Record, error) {
	var record model.Record
	var id, userID, recordType string
	if err := row.Scan(
		&id,
		&userID,
		&recordType,
		&record.EncryptedPayload,
		&record.EncryptedMetadata,
		&record.PayloadNonce,
		&record.MetadataNonce,
		&record.Version,
		&record.Revision,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	); err != nil {
		return model.Record{}, err
	}
	parsedID, err := model.ParseID(id)
	if err != nil {
		return model.Record{}, fmt.Errorf("parse stored record ID: %w", err)
	}
	parsedUserID, err := model.ParseID(userID)
	if err != nil {
		return model.Record{}, fmt.Errorf("parse stored record user ID: %w", err)
	}
	record.ID = parsedID
	record.UserID = parsedUserID
	record.Type = model.RecordType(recordType)
	return record, nil
}
