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
		serverName,
		"Version: ",
		"Build date: ",
		"Commit: ",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version output %q does not contain %q", got, expected)
		}
	}
}
