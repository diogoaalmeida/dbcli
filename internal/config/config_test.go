package config

import "testing"

func TestAddListResolveRemove(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PROFILE_DEV", "")

	if err := Add("dev", "postgres://user:pass@localhost:5432/app"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add("prod", "postgres://user:pass@prod-host:5432/app"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	names, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 || names[0] != "dev" || names[1] != "prod" {
		t.Fatalf("got %v, want [dev prod]", names)
	}

	dsn, err := Resolve("dev")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dsn != "postgres://user:pass@localhost:5432/app" {
		t.Fatalf("got %q", dsn)
	}

	if err := Remove("dev"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := Resolve("dev"); err == nil {
		t.Fatalf("expected error resolving removed profile")
	}

	if err := Remove("does_not_exist"); err == nil {
		t.Fatalf("expected error removing unknown profile")
	}
}

func TestAdd_RejectsInvalidProfileNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cases := []string{
		"has-hyphen",           // breaks godotenv's key=value parsing outright
		"has space",            // same
		"name\nPROFILE_EVIL=x", // line-injection attempt
		"name=injected",        // would corrupt the key=value pair
		"",                     // empty name
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Add(name, "postgres://user:pass@localhost:5432/app"); err == nil {
				t.Fatalf("expected Add to reject profile name %q", name)
			}
		})
	}
}

func TestAdd_RejectsNewlineInDSN(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Add("dev", "postgres://user:pass@localhost/app\nPROFILE_EVIL=x"); err == nil {
		t.Fatalf("expected Add to reject a dsn containing a newline")
	}
}

func TestAdd_InjectionAttemptNeverCreatesASecondProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Even if validation regressed, prove the actual attack doesn't work:
	// a crafted name can't make it into the file as two lines.
	_ = Add("legit\nPROFILE_EVIL", "postgres://user:pass@localhost/app")

	names, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, n := range names {
		if n == "evil" {
			t.Fatalf("injection succeeded: found an unexpected %q profile in %v", n, names)
		}
	}
}

func TestResolveFallsBackToDatabaseURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/fallback")

	dsn, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dsn != "postgres://user:pass@localhost:5432/fallback" {
		t.Fatalf("got %q", dsn)
	}
}

func TestResolveNoProfileNoDatabaseURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DATABASE_URL", "")

	if _, err := Resolve(""); err == nil {
		t.Fatalf("expected error when no profile and no DATABASE_URL set")
	}
}
