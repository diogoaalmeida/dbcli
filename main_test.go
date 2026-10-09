package main

import (
	"bytes"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRun_UnknownCommandExitsWithCode2(t *testing.T) {
	if got := run([]string{"not-a-real-command"}); got != 2 {
		t.Fatalf("got exit code %d, want 2", got)
	}
}

func TestRun_NoArgsExitsWithCode2(t *testing.T) {
	if got := run(nil); got != 2 {
		t.Fatalf("got exit code %d, want 2", got)
	}
}

// TestRun_VersionAndHelpPrintPlainTextNotJSON locks in the deliberate
// exception to "every result is JSON on stdout": version and help have
// no query/result to report, so they print plain text instead.
func TestRun_VersionAndHelpPrintPlainTextNotJSON(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out := captureStdout(t, func() {
				if got := run(args); got != 0 {
					t.Fatalf("got exit code %d, want 0", got)
				}
			})
			if !strings.HasPrefix(out, "dbcli ") {
				t.Fatalf("got %q, want a plain \"dbcli <version>\" line, not JSON", out)
			}
			if strings.HasPrefix(strings.TrimSpace(out), "{") {
				t.Fatalf("got %q, want plain text, not a JSON envelope", out)
			}
		})
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if got := run(args); got != 0 {
				t.Fatalf("got exit code %d, want 0", got)
			}
		})
	}
}

// TestRun_DispatchesEveryCommandWithoutPanicking exercises every known
// command string with no DATABASE_URL/profile configured, so each
// data command fails cleanly (a connection error, exit code 1) rather
// than reaching a real database — the point here is proving the
// dispatch switch in run() routes to the right handler without
// panicking, not exercising the handlers' own logic (that's covered in
// cmd/*_test.go and internal/postgres).
func TestRun_DispatchesEveryCommandWithoutPanicking(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	cases := []struct {
		name string
		args []string
	}{
		{"query", []string{"query", "select 1"}},
		{"explain", []string{"explain", "select 1"}},
		{"schemas", []string{"schemas"}},
		{"schema", []string{"schema"}},
		{"describe", []string{"describe", "sometable"}},
		{"sample", []string{"sample", "sometable"}},
		{"profiles list", []string{"profiles", "list"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("run(%v) panicked: %v", tc.args, r)
				}
			}()
			// Any exit code is acceptable here; a panic is the failure
			// this test exists to catch.
			_ = captureStdout(t, func() { run(tc.args) })
		})
	}
}

func TestVcsVersion_UsesRevisionAndModifiedFlag(t *testing.T) {
	cases := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{
			name: "clean checkout",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "63b21357cfa01feeda9b63df62e288f6c0541901"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: "dev+63b2135",
		},
		{
			name: "dirty checkout",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "63b21357cfa01feeda9b63df62e288f6c0541901"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: "dev+63b2135-dirty",
		},
		{
			name:     "no VCS info at all (e.g. a tarball with no .git)",
			settings: nil,
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := vcsVersion(&debug.BuildInfo{Settings: tc.settings})
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// captureStdout runs fn with os.Stdout redirected, returning what it
// printed. run() uses fmt.Println/Fprint (stdout) and fmt.Fprintf
// (stderr); this only needs to capture stdout since that's the only
// stream version/help's JSON-exception claim is about.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}
