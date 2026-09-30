package cmd

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

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
