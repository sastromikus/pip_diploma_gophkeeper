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
	for _, expected := range []string{
		clientName,
		"Version: ",
		"Build date: ",
		"Commit: ",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version output %q does not contain %q", got, expected)
		}
	}
}

func TestIsVersionCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "command", args: []string{"version"}, want: true},
		{name: "short flag", args: []string{"-version"}, want: true},
		{name: "long flag", args: []string{"--version"}, want: true},
		{name: "missing", args: nil, want: false},
		{name: "extra argument", args: []string{"version", "extra"}, want: false},
		{name: "different command", args: []string{"login"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVersionCommand(tt.args); got != tt.want {
				t.Fatalf("isVersionCommand(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
