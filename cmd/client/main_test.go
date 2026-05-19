package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"version"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	got := output.String()
	for _, expected := range []string{clientName, "Version: ", "Build date: ", "Commit: "} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version output %q does not contain %q", got, expected)
		}
	}
}

func TestRunShowsUsageWithoutCommand(t *testing.T) {
	var output bytes.Buffer
	if err := runWithIO(nil, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gophkeeper-client register") {
		t.Fatalf("usage = %q", output.String())
	}
}

func TestReadCredentials(t *testing.T) {
	var output bytes.Buffer
	login, password, err := readCredentials(strings.NewReader("alice\nsecret-password\nsecret-password\n"), &output, true)
	if err != nil {
		t.Fatal(err)
	}
	if login != "alice" || password != "secret-password" {
		t.Fatalf("credentials = %q/%q", login, password)
	}
}

func TestReadCredentialsRejectsMismatch(t *testing.T) {
	_, _, err := readCredentials(strings.NewReader("alice\none\ntwo\n"), &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsVersionCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"command", []string{"version"}, true}, {"short flag", []string{"-version"}, true}, {"long flag", []string{"--version"}, true}, {"missing", nil, false}, {"extra argument", []string{"version", "extra"}, false}, {"different command", []string{"login"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVersionCommand(tt.args); got != tt.want {
				t.Fatalf("isVersionCommand(%q)=%v want %v", tt.args, got, tt.want)
			}
		})
	}
}
