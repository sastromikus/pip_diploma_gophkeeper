package version

import "testing"

func TestCurrent(t *testing.T) {
	oldVersion, oldBuildDate, oldCommit := Version, BuildDate, Commit
	t.Cleanup(func() {
		Version, BuildDate, Commit = oldVersion, oldBuildDate, oldCommit
	})

	Version = "1.2.3"
	BuildDate = "2026-06-09T03:00:00Z"
	Commit = "abc1234"

	got := Current()
	if got.Version != Version {
		t.Fatalf("Version = %q, want %q", got.Version, Version)
	}
	if got.BuildDate != BuildDate {
		t.Fatalf("BuildDate = %q, want %q", got.BuildDate, BuildDate)
	}
	if got.Commit != Commit {
		t.Fatalf("Commit = %q, want %q", got.Commit, Commit)
	}
}

func TestCurrentUsesFallbacksForEmptyValues(t *testing.T) {
	oldVersion, oldBuildDate, oldCommit := Version, BuildDate, Commit
	t.Cleanup(func() {
		Version, BuildDate, Commit = oldVersion, oldBuildDate, oldCommit
	})

	Version = ""
	BuildDate = ""
	Commit = ""

	got := Current()
	if got.Version != defaultVersion {
		t.Fatalf("Version = %q, want %q", got.Version, defaultVersion)
	}
	if got.BuildDate != defaultBuildDate {
		t.Fatalf("BuildDate = %q, want %q", got.BuildDate, defaultBuildDate)
	}
	if got.Commit != defaultCommit {
		t.Fatalf("Commit = %q, want %q", got.Commit, defaultCommit)
	}
}

func TestFormat(t *testing.T) {
	oldVersion, oldBuildDate, oldCommit := Version, BuildDate, Commit
	t.Cleanup(func() {
		Version, BuildDate, Commit = oldVersion, oldBuildDate, oldCommit
	})

	Version = "1.0.0"
	BuildDate = "2026-06-09"
	Commit = "deadbee"

	want := "GophKeeper client\nVersion: 1.0.0\nBuild date: 2026-06-09\nCommit: deadbee\n"
	if got := Format("GophKeeper client"); got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestIsCommand(t *testing.T) {
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
		{name: "other command", args: []string{"login"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCommand(tt.args); got != tt.want {
				t.Fatalf("IsCommand(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
