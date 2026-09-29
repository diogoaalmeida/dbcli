package cmd

import (
	"reflect"
	"testing"
)

func TestSplitFlagsAndPositional(t *testing.T) {
	cases := []struct {
		name           string
		args           []string
		boolFlags      map[string]bool
		wantFlags      []string
		wantPositional []string
	}{
		{
			name:           "flag before positional",
			args:           []string{"--limit", "1", "vehicles"},
			wantFlags:      []string{"--limit", "1"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "flag after positional, the natural way people type it",
			args:           []string{"vehicles", "--limit", "1"},
			wantFlags:      []string{"--limit", "1"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "flag interleaved between two positionals",
			args:           []string{"vehicles", "--schema", "core", "extra"},
			wantFlags:      []string{"--schema", "core"},
			wantPositional: []string{"vehicles", "extra"},
		},
		{
			name:           "equals form is self-contained",
			args:           []string{"vehicles", "--limit=5"},
			wantFlags:      []string{"--limit=5"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "bool flag takes no value",
			args:           []string{"select 1", "--analyze"},
			boolFlags:      map[string]bool{"analyze": true},
			wantFlags:      []string{"--analyze"},
			wantPositional: []string{"select 1"},
		},
		{
			name:           "bool flag before positional does not consume it as a value",
			args:           []string{"--analyze", "select 1"},
			boolFlags:      map[string]bool{"analyze": true},
			wantFlags:      []string{"--analyze"},
			wantPositional: []string{"select 1"},
		},
		{
			name:           "dangling flag with no following value at end of args",
			args:           []string{"vehicles", "--limit"},
			wantFlags:      []string{"--limit"},
			wantPositional: []string{"vehicles"},
		},
		{
			name:           "single dash alone is treated as positional",
			args:           []string{"-"},
			wantFlags:      nil,
			wantPositional: []string{"-"},
		},
		{
			name:           "no flags at all",
			args:           []string{"select 1"},
			wantFlags:      nil,
			wantPositional: []string{"select 1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFlags, gotPositional := splitFlagsAndPositional(tc.args, tc.boolFlags)
			if !reflect.DeepEqual(gotFlags, tc.wantFlags) {
				t.Fatalf("flags: got %#v, want %#v", gotFlags, tc.wantFlags)
			}
			if !reflect.DeepEqual(gotPositional, tc.wantPositional) {
				t.Fatalf("positional: got %#v, want %#v", gotPositional, tc.wantPositional)
			}
		})
	}
}
