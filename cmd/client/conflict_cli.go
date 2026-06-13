package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientstorage "github.com/sastromikus/gophkeeper/internal/client/storage"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type conflictCommand struct {
	name       string
	id         model.ID
	resolution clientstorage.ConflictResolution
	configArgs []string
}

func parseConflictCommand(args []string) (conflictCommand, error) {
	if len(args) == 0 {
		return conflictCommand{}, errors.New("conflict command is required")
	}
	command := conflictCommand{name: args[0]}
	switch command.name {
	case "conflicts":
		command.configArgs = args[1:]
	case "resolve":
		if len(args) < 3 {
			return conflictCommand{}, errors.New("resolve requires record ID and local|server choice")
		}
		id, err := model.ParseID(args[1])
		if err != nil {
			return conflictCommand{}, fmt.Errorf("parse conflict record ID: %w", err)
		}
		command.id = id
		switch strings.ToLower(args[2]) {
		case string(clientstorage.ConflictResolutionLocal):
			command.resolution = clientstorage.ConflictResolutionLocal
		case string(clientstorage.ConflictResolutionServer):
			command.resolution = clientstorage.ConflictResolutionServer
		default:
			return conflictCommand{}, fmt.Errorf("unsupported conflict resolution %q", args[2])
		}
		command.configArgs = args[3:]
	default:
		return conflictCommand{}, fmt.Errorf("unknown conflict command %q", command.name)
	}
	return command, nil
}

func executeConflictCommand(command conflictCommand, service *clientapp.ConflictService, stdout io.Writer) error {
	ctx, cancel := commandContext()
	defer cancel()

	switch command.name {
	case "conflicts":
		found := false
		for conflict, err := range service.List(ctx) {
			if err != nil {
				return err
			}
			found = true
			remoteState := "active"
			if conflict.RemoteDeleted {
				remoteState = "deleted"
			}
			if _, err := fmt.Fprintf(stdout, "%s\t%s\tlocal-v%d\tserver-v%d\tserver-%s\n", conflict.ID, conflict.Type, conflict.LocalVersion, conflict.RemoteVersion, remoteState); err != nil {
				return fmt.Errorf("write conflict list: %w", err)
			}
		}
		if !found {
			_, err := fmt.Fprintln(stdout, "No unresolved conflicts.")
			return err
		}
		return nil
	case "resolve":
		if err := service.Resolve(ctx, command.id, command.resolution); err != nil {
			return err
		}
		if command.resolution == clientstorage.ConflictResolutionLocal {
			_, err := fmt.Fprintf(stdout, "Conflict resolved with local version: %s. Run sync to upload it.\n", command.id)
			return err
		}
		_, err := fmt.Fprintf(stdout, "Conflict resolved with server version: %s.\n", command.id)
		return err
	default:
		return fmt.Errorf("unsupported conflict command %q", command.name)
	}
}
