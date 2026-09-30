// Package config resolves named connection profiles into DSNs. Profiles
// live in a plain key=value file (godotenv format) so they can be edited by
// hand or managed with the `dbcli profiles` subcommand.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// validProfileName matches valid environment-variable-name characters.
// Anything else (newlines, "=", hyphens, spaces, ...) either breaks
// profiles.env's key=value parsing outright or, worse, lets a crafted name
// inject an extra line into the file — a real risk since dbcli is meant to
// be driven by an agent that could itself be prompt-injected into running
// `dbcli profiles add <attacker-controlled name>`.
var validProfileName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func validateProfileName(name string) error {
	if !validProfileName.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: only letters, digits, and underscores are allowed", name)
	}
	return nil
}

// ConfigDir returns the directory dbcli's config file lives in, honoring
// XDG_CONFIG_HOME when set.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dbcli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "dbcli"), nil
}

// ProfilesPath returns the full path to the profiles file.
func ProfilesPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles.env"), nil
}

func profileKey(name string) string {
	return "PROFILE_" + strings.ToUpper(name)
}

// Resolve returns the DSN for the given profile name. An empty profile
// falls back to the DATABASE_URL environment variable. A profile's env var
// (e.g. PROFILE_DEV), if already exported in the shell, takes precedence
// over the profiles file — useful for CI or one-off overrides.
func Resolve(profile string) (string, error) {
	if profile == "" {
		if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
			return dsn, nil
		}
		return "", fmt.Errorf("no --profile given and DATABASE_URL is not set")
	}

	key := profileKey(profile)
	if dsn := os.Getenv(key); dsn != "" {
		return dsn, nil
	}

	path, err := ProfilesPath()
	if err != nil {
		return "", err
	}
	vars, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("profile %q not found: no profiles file at %s (run `dbcli profiles add %s <dsn>`)", profile, path, profile)
		}
		return "", fmt.Errorf("read profiles file: %w", err)
	}
	dsn, ok := vars[key]
	if !ok || dsn == "" {
		return "", fmt.Errorf("profile %q not found in %s", profile, path)
	}
	return dsn, nil
}

// List returns the profile names currently defined in the profiles file, or
// an empty slice if the file doesn't exist yet.
func List() ([]string, error) {
	path, err := ProfilesPath()
	if err != nil {
		return nil, err
	}
	vars, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profiles file: %w", err)
	}

	var names []string
	for k := range vars {
		if strings.HasPrefix(k, "PROFILE_") {
			names = append(names, strings.ToLower(strings.TrimPrefix(k, "PROFILE_")))
		}
	}
	sort.Strings(names)
	return names, nil
}

// Add writes or overwrites a profile's DSN in the profiles file, creating
// the config directory and file if needed.
func Add(name, dsn string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	if strings.ContainsAny(dsn, "\r\n") {
		return fmt.Errorf("dsn cannot contain newlines")
	}

	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	path, err := ProfilesPath()
	if err != nil {
		return err
	}

	vars, err := godotenv.Read(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read profiles file: %w", err)
	}
	if vars == nil {
		vars = map[string]string{}
	}
	vars[profileKey(name)] = dsn

	return writeEnvFile(path, vars)
}

// Remove deletes a profile from the profiles file.
func Remove(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}

	path, err := ProfilesPath()
	if err != nil {
		return err
	}
	vars, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found: no profiles file at %s", name, path)
		}
		return fmt.Errorf("read profiles file: %w", err)
	}
	key := profileKey(name)
	if _, ok := vars[key]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(vars, key)
	return writeEnvFile(path, vars)
}

func writeEnvFile(path string, vars map[string]string) error {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, k := range names {
		fmt.Fprintf(&b, "%s=%s\n", k, vars[k])
	}

	// 0o600: profiles hold live database credentials.
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
