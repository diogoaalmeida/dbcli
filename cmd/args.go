package cmd

import "strings"

// splitFlagsAndPositional separates a mixed argument list into flag tokens
// and positional tokens, so callers can `fs.Parse(flags)` and use
// `positional` directly. This exists because Go's flag package stops
// parsing at the first non-flag argument, which would otherwise silently
// ignore flags typed after a positional argument (e.g.
// `dbcli sample vehicles --limit 1`, the natural way to type it).
//
// knownFlags maps every valid flag name for the calling command (without
// dashes) to whether it's boolean (true, takes no value) or not (false,
// takes a value). A "-"-prefixed token that isn't in knownFlags is treated
// as positional rather than an unrecognized flag — this lets a query or
// table name that happens to start with "-" survive instead of being
// misparsed.
func splitFlagsAndPositional(args []string, knownFlags map[string]bool) (flags []string, positional []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			i++
			continue
		}

		name := strings.TrimLeft(a, "-")
		hasValue := false
		if eq := strings.Index(name, "="); eq != -1 {
			name = name[:eq]
			hasValue = true
		}

		isBool, known := knownFlags[name]
		if !known {
			positional = append(positional, a)
			i++
			continue
		}

		flags = append(flags, a)
		if hasValue || isBool {
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
