package cmd

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/config"
)

// withStdin redirects os.Stdin to a pipe pre-loaded with content for the
// duration of fn.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(content); err != nil {
		t.Fatalf("write to stdin pipe: %v", err)
	}
	w.Close()

	original := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = original }()

	fn()
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(out)
}

// TestProfiles_AddAndRemoveEmitJSON locks in the README's own contract
// ("every result, success or failure, is JSON on stdout"): profiles add and
// remove used to print plain text (fmt.Fprintf(os.Stdout, "profile %q
// saved\n", ...)) instead of going through the envelope, which would break
// any agent parsing stdout as JSON right after calling either one.
func TestProfiles_AddAndRemoveEmitJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	addOut := captureStdout(t, func() {
		if code := Profiles([]string{"add", "dev", "postgres://user:pass@localhost:5432/db"}); code != 0 {
			t.Fatalf("Profiles add: exit code %d", code)
		}
	})
	var addDecoded struct {
		OK   bool              `json:"ok"`
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(addOut), &addDecoded); err != nil {
		t.Fatalf("profiles add did not print valid JSON: %v\noutput: %s", err, addOut)
	}
	if !addDecoded.OK || addDecoded.Data["profile"] != "dev" || addDecoded.Data["action"] != "saved" {
		t.Fatalf("unexpected add envelope: %+v", addDecoded)
	}

	removeOut := captureStdout(t, func() {
		if code := Profiles([]string{"remove", "dev"}); code != 0 {
			t.Fatalf("Profiles remove: exit code %d", code)
		}
	})
	var removeDecoded struct {
		OK   bool              `json:"ok"`
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(removeOut), &removeDecoded); err != nil {
		t.Fatalf("profiles remove did not print valid JSON: %v\noutput: %s", err, removeOut)
	}
	if !removeDecoded.OK || removeDecoded.Data["profile"] != "dev" || removeDecoded.Data["action"] != "removed" {
		t.Fatalf("unexpected remove envelope: %+v", removeDecoded)
	}
}

// TestProfiles_AddReadsDSNFromStdinWhenOmitted covers the credentials-as-argv
// mitigation: `dbcli profiles add <name>` with no dsn positional arg should
// read it from stdin instead, so the DSN (with its password) never has to
// appear as a plain command-line argument visible in shell history or `ps`.
func TestProfiles_AddReadsDSNFromStdinWhenOmitted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	withStdin(t, "postgres://user:pass@localhost:5432/viastdin\n", func() {
		out := captureStdout(t, func() {
			if code := Profiles([]string{"add", "dev"}); code != 0 {
				t.Fatalf("Profiles add: exit code %d", code)
			}
		})
		var decoded struct {
			OK   bool              `json:"ok"`
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
		}
		if !decoded.OK || decoded.Data["profile"] != "dev" {
			t.Fatalf("unexpected envelope: %+v", decoded)
		}
	})

	dsn, err := config.Resolve("dev")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dsn != "postgres://user:pass@localhost:5432/viastdin" {
		t.Fatalf("got %q, want the trimmed DSN read from stdin", dsn)
	}
}
