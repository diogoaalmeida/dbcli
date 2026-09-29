package cmd

import "strings"

// splitFlagsAndPositional separates a mixed argument list into flag tokens
// and positional tokens, so callers can `fs.Parse(flags)` and use
// `positional` directly. This exists because Go's flag package stops
// parsing at the first non-flag argument, which would otherwise silently
// ignore flags typed after a positional argument (e.g.
// `dbcli sample vehicles --limit 1`, the natural way to type it).
// boolFlags lists flag names (without dashes) that take no value.
func splitFlagsAndPositional(args []string, boolFlags map[string]bool) (flags []string, positional []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			i++
			continue
		}

		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") || boolFlags[name] {
			// "--limit=5" is self-contained; known bool flags take no value.
			i++
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i += 2
			continue
		}
		i++
	}
	return flags, positional
}
