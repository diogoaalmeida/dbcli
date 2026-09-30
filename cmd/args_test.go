package cmd

import (
	"reflect"
	"testing"
)

func TestSplitFlagsAndPositional(t *testing.T) {
	sampleFlags := map[string]bool{"profile": false, "schema": false, "limit": false, "timeout": false, "format": false}
	explainFlags := map[string]bool{"profile": false, "analyze": true, "timeout": false, "format": false}

	cases := []struct {
		name           string
		args           []string
		knownFlags     map[string]bool
		wantFlags      []string
		wantPositional []string
	}{
		{
			name:           "flag before positional",
			args:           []string{"--limit", "1", "vehicles"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--limit", "1"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "flag after positional, the natural way people type it",
			args:           []string{"vehicles", "--limit", "1"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--limit", "1"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "flag interleaved between two positionals",
			args:           []string{"vehicles", "--schema", "core", "extra"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--schema", "core"},
			wantPositional: []string{"vehicles", "extra"},
		},
		{
			name:           "equals form is self-contained",
			args:           []string{"vehicles", "--limit=5"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--limit=5"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "bool flag takes no value",
			args:           []string{"select 1", "--analyze"},
			knownFlags:     explainFlags,
			wantFlags:      []string{"--analyze"},
			wantPositional: []string{"select 1"},
		},
		{
			name:           "bool flag before positional does not consume it as a value",
			args:           []string{"--analyze", "select 1"},
			knownFlags:     explainFlags,
			wantFlags:      []string{"--analyze"},
			wantPositional: []string{"select 1"},
		},
		{
			name:           "dangling known flag with no following value at end of args",
			args:           []string{"vehicles", "--limit"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--limit"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "single dash alone is treated as positional",
			args:           []string{"-"},
			knownFlags:     sampleFlags,
			wantFlags:      nil,
			wantPositional: []string{"-"},
		},
		{
			name:           "no flags at all",
			args:           []string{"select 1"},
			knownFlags:     sampleFlags,
			wantFlags:      nil,
			wantPositional: []string{"select 1"},
		},
		{
			// This is the bug: a table/query value starting with "-" that
			// isn't a recognized flag name must not be swallowed as an
			// unknown flag — it has to survive as positional.
			name:           "unrecognized hyphen-prefixed token is treated as positional, not a flag",
			args:           []string{"--profile", "dev", "-weird-table-name"},
			knownFlags:     sampleFlags,
			wantFlags:      []string{"--profile", "dev"},
			wantPositional: []string{"-weird-table-name"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFlags, gotPositional := splitFlagsAndPositional(tc.args, tc.knownFlags)
			if !reflect.DeepEqual(gotFlags, tc.wantFlags) {
				t.Fatalf("flags: got %#v, want %#v", gotFlags, tc.wantFlags)
			}
			if !reflect.DeepEqual(gotPositional, tc.wantPositional) {
				t.Fatalf("positional: got %#v, want %#v", gotPositional, tc.wantPositional)
			}
		})
	}
}
