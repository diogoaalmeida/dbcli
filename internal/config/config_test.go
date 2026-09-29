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

	if err := Remove("does-not-exist"); err == nil {
		t.Fatalf("expected error removing unknown profile")
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
