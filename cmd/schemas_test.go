package cmd

import (
	"encoding/json"
	"testing"
)

// TestSchemas_ReturnsDataEnvelope is the cmd-level entry-point test the
// audit found missing: internal/postgres's ListSchemas is well tested,
// but nothing exercised the Schemas() CLI command itself (flag parsing,
// connect(), the data envelope on stdout).
func TestSchemas_ReturnsDataEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Schemas(nil); code != 0 {
			t.Fatalf("Schemas: exit code %d", code)
		}
	})

	var decoded struct {
		OK   bool `json:"ok"`
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK {
		t.Fatalf("expected ok:true, got %s", out)
	}
	var sawPublic bool
	for _, s := range decoded.Data {
		if s.Name == "public" {
			sawPublic = true
		}
	}
	if !sawPublic {
		t.Fatalf("expected \"public\" schema in %s", out)
	}
}

// TestSchemas_DriverFlagOverridesInferredScheme covers --driver, which
// exists on every command but had no coverage anywhere: passing the
// real scheme explicitly should still work (it isn't just ignored), and
// an unknown scheme should fail cleanly with the JSON error envelope,
// not a panic or a bare non-JSON error.
func TestSchemas_DriverFlagOverridesInferredScheme(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	t.Run("matching driver still succeeds", func(t *testing.T) {
		out := captureStdout(t, func() {
			if code := Schemas([]string{"--driver", "postgres"}); code != 0 {
				t.Fatalf("Schemas --driver postgres: exit code %d", code)
			}
		})
		var decoded struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil || !decoded.OK {
			t.Fatalf("expected ok:true: %v, output: %s", err, out)
		}
	})

	t.Run("unknown driver fails with the JSON error envelope", func(t *testing.T) {
		out := captureStdout(t, func() {
			if code := Schemas([]string{"--driver", "not-a-real-driver"}); code == 0 {
				t.Fatalf("expected a non-zero exit code")
			}
		})
		var decoded struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("did not print valid JSON even on error: %v\noutput: %s", err, out)
		}
		if decoded.OK || decoded.Code == "" {
			t.Fatalf("expected ok:false with a non-empty code, got %+v", decoded)
		}
	})
}
