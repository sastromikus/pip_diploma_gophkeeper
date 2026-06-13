package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const maxCLIFileSize = int64(10 * 1024 * 1024)

type vaultService interface {
	Create(context.Context, string, model.RecordType, any, clientmodel.Metadata) (clienttransport.RemoteRecord, error)
	Get(context.Context, string, model.ID) (clientapp.RecordView, error)
	List(context.Context, string) iter.Seq2[clientapp.RecordSummary, error]
	Update(context.Context, string, model.ID, model.RecordType, any, clientmodel.Metadata) (clienttransport.RemoteRecord, error)
	Delete(context.Context, model.ID) error
}

type vaultCommand struct {
	name       string
	recordType model.RecordType
	id         model.ID
	outputPath string
	configArgs []string
}

func parseVaultCommand(args []string) (vaultCommand, error) {
	if len(args) == 0 {
		return vaultCommand{}, errors.New("vault command is required")
	}
	command := vaultCommand{name: args[0]}
	rest := args[1:]
	switch command.name {
	case "add":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return vaultCommand{}, errors.New("record type is required")
		}
		recordType, err := parseRecordType(rest[0])
		if err != nil {
			return vaultCommand{}, err
		}
		command.recordType = recordType
		command.configArgs = rest[1:]
	case "list":
		command.configArgs = rest
	case "get":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return vaultCommand{}, errors.New("record ID is required")
		}
		id, err := model.ParseID(rest[0])
		if err != nil {
			return vaultCommand{}, fmt.Errorf("parse record ID: %w", err)
		}
		command.id = id
		rest = rest[1:]
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			command.outputPath = rest[0]
			rest = rest[1:]
		}
		command.configArgs = rest
	case "update", "delete":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return vaultCommand{}, errors.New("record ID is required")
		}
		id, err := model.ParseID(rest[0])
		if err != nil {
			return vaultCommand{}, fmt.Errorf("parse record ID: %w", err)
		}
		command.id = id
		command.configArgs = rest[1:]
	default:
		return vaultCommand{}, fmt.Errorf("unknown vault command %q", command.name)
	}
	return command, nil
}

func parseRecordType(value string) (model.RecordType, error) {
	switch strings.ToLower(value) {
	case "credentials":
		return model.RecordTypeCredentials, nil
	case "text":
		return model.RecordTypeText, nil
	case "binary":
		return model.RecordTypeBinary, nil
	case "card", "bank-card", "bank_card":
		return model.RecordTypeBankCard, nil
	default:
		return "", fmt.Errorf("unsupported record type %q", value)
	}
}

func executeVaultCommand(command vaultCommand, service vaultService, stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	switch command.name {
	case "add":
		password, err := readPassword(stdin, reader, stdout, "Master password: ")
		if err != nil {
			return err
		}
		payload, metadata, err := readRecordInput(stdin, reader, stdout, command.recordType)
		if err != nil {
			return err
		}
		ctx, cancel := commandContext()
		defer cancel()
		record, err := service.Create(ctx, password, command.recordType, payload, metadata)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Record saved locally: %s. Run sync to upload it.\n", record.ID)
		return err
	case "list":
		password, err := readPassword(stdin, reader, stdout, "Master password: ")
		if err != nil {
			return err
		}
		ctx, cancel := commandContext()
		defer cancel()
		found := false
		for record, err := range service.List(ctx, password) {
			if err != nil {
				return err
			}
			found = true
			if _, err := fmt.Fprintf(stdout, "%s\t%s\tv%d\t%s\t%s\n", record.ID, record.Type, record.Version, record.SyncStatus, record.Title); err != nil {
				return fmt.Errorf("write record list: %w", err)
			}
		}
		if !found {
			_, err = fmt.Fprintln(stdout, "Vault is empty.")
			return err
		}
		return nil
	case "get":
		password, err := readPassword(stdin, reader, stdout, "Master password: ")
		if err != nil {
			return err
		}
		ctx, cancel := commandContext()
		defer cancel()
		record, err := service.Get(ctx, password, command.id)
		if err != nil {
			return err
		}
		return writeRecord(stdout, record, command.outputPath)
	case "update":
		password, err := readPassword(stdin, reader, stdout, "Master password: ")
		if err != nil {
			return err
		}
		getCtx, getCancel := commandContext()
		current, err := service.Get(getCtx, password, command.id)
		getCancel()
		if err != nil {
			return err
		}
		payload, metadata, err := readRecordInput(stdin, reader, stdout, current.Type)
		if err != nil {
			return err
		}
		updateCtx, updateCancel := commandContext()
		defer updateCancel()
		updated, err := service.Update(updateCtx, password, command.id, current.Type, payload, metadata)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Record updated locally: %s. Run sync to upload changes.\n", updated.ID)
		return err
	case "delete":
		ctx, cancel := commandContext()
		defer cancel()
		if err := service.Delete(ctx, command.id); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "Record deleted locally: %s. Run sync to propagate deletion.\n", command.id)
		return err
	default:
		return fmt.Errorf("unsupported vault command %q", command.name)
	}
}

func readRecordInput(stdin io.Reader, reader *bufio.Reader, stdout io.Writer, recordType model.RecordType) (any, clientmodel.Metadata, error) {
	var payload any
	var err error
	switch recordType {
	case model.RecordTypeCredentials:
		var value clientmodel.Credentials
		if value.Name, err = readLine(reader, stdout, "Name: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.Login, err = readLine(reader, stdout, "Login: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.Password, err = readPassword(stdin, reader, stdout, "Secret password: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		payload = value
	case model.RecordTypeText:
		var value clientmodel.Text
		if value.Title, err = readLine(reader, stdout, "Title: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.Body, err = readMultiline(reader, stdout, "Text (finish with a single . line):\n"); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		payload = value
	case model.RecordTypeBinary:
		path, readErr := readLine(reader, stdout, "File path: ")
		if readErr != nil {
			return nil, clientmodel.Metadata{}, readErr
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, clientmodel.Metadata{}, fmt.Errorf("inspect binary file: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return nil, clientmodel.Metadata{}, errors.New("binary input must be a regular file")
		}
		if info.Size() > maxCLIFileSize {
			return nil, clientmodel.Metadata{}, fmt.Errorf("binary file exceeds %d bytes", maxCLIFileSize)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, clientmodel.Metadata{}, fmt.Errorf("read binary file: %w", readErr)
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		payload = clientmodel.Binary{Filename: filepath.Base(path), MIMEType: mimeType, Data: data}
	case model.RecordTypeBankCard:
		var value clientmodel.BankCard
		if value.Name, err = readLine(reader, stdout, "Name: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.Number, err = readPassword(stdin, reader, stdout, "Card number: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.Holder, err = readLine(reader, stdout, "Card holder: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.ExpiryDate, err = readLine(reader, stdout, "Expiry (MM/YY): "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		if value.CVV, err = readPassword(stdin, reader, stdout, "CVV: "); err != nil {
			return nil, clientmodel.Metadata{}, err
		}
		payload = value
	default:
		return nil, clientmodel.Metadata{}, fmt.Errorf("unsupported record type %q", recordType)
	}
	metadataText, err := readLine(reader, stdout, "Metadata (optional): ")
	if err != nil {
		return nil, clientmodel.Metadata{}, err
	}
	return payload, clientmodel.Metadata{Text: metadataText}, nil
}

func readMultiline(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	if _, err := io.WriteString(stdout, prompt); err != nil {
		return "", fmt.Errorf("write multiline prompt: %w", err)
	}
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read multiline input: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		lines = append(lines, line)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}

func readLine(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	if _, err := io.WriteString(stdout, prompt); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func writeRecord(stdout io.Writer, record clientapp.RecordView, outputPath string) error {
	if _, err := fmt.Fprintf(stdout, "ID: %s\nType: %s\nVersion: %d\n", record.ID, record.Type, record.Version); err != nil {
		return fmt.Errorf("write record header: %w", err)
	}
	switch payload := record.Payload.(type) {
	case clientmodel.Credentials:
		if _, err := fmt.Fprintf(stdout, "Name: %s\nLogin: %s\nPassword: %s\n", payload.Name, payload.Login, payload.Password); err != nil {
			return fmt.Errorf("write credentials record: %w", err)
		}
	case clientmodel.Text:
		if _, err := fmt.Fprintf(stdout, "Title: %s\nText: %s\n", payload.Title, payload.Body); err != nil {
			return fmt.Errorf("write text record: %w", err)
		}
	case clientmodel.Binary:
		if _, err := fmt.Fprintf(stdout, "Filename: %s\nMIME type: %s\nSize: %d bytes\n", payload.Filename, payload.MIMEType, len(payload.Data)); err != nil {
			return fmt.Errorf("write binary record: %w", err)
		}
		if outputPath != "" {
			if err := writeBinaryFile(outputPath, payload.Data); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(stdout, "Saved to: %s\n", outputPath); err != nil {
				return fmt.Errorf("write binary output path: %w", err)
			}
		}
	case clientmodel.BankCard:
		if _, err := fmt.Fprintf(
			stdout,
			"Name: %s\nNumber: %s\nHolder: %s\nExpiry: %s\n",
			payload.Name,
			payload.MaskedNumber(),
			payload.Holder,
			payload.ExpiryDate,
		); err != nil {
			return fmt.Errorf("write bank card record: %w", err)
		}
	default:
		return fmt.Errorf("unsupported decrypted payload type %T", record.Payload)
	}
	if record.Metadata.Text != "" {
		if _, err := fmt.Fprintf(stdout, "Metadata: %s\n", record.Metadata.Text); err != nil {
			return fmt.Errorf("write record metadata: %w", err)
		}
	}
	return nil
}

func writeBinaryFile(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write output file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}
