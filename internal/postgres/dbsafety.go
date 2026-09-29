package postgres

import (
	"fmt"
	"regexp"
	"strings"
)

var allowedStart = regexp.MustCompile(`(?is)^\s*(select|with|explain|table)\b`)

// ValidateQuery strips SQL comments, rejects anything but a single statement,
// and requires the statement to start with SELECT/WITH/EXPLAIN/TABLE. It
// returns the cleaned, single-statement SQL with any trailing terminators
// removed. This is the first of two layers of defense — the second is that
// every query still runs inside a READ ONLY transaction, so even a gap here
// can't result in a write.
func ValidateQuery(sql string) (string, error) {
	cleaned, semicolonOffsets := stripComments(sql)
	body := strings.TrimRight(cleaned, "; \t\r\n")

	for _, off := range semicolonOffsets {
		if off < len(body) {
			return "", fmt.Errorf("query rejected: only a single statement is allowed")
		}
	}

	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("query rejected: empty statement")
	}

	if !allowedStart.MatchString(body) {
		return "", fmt.Errorf("query rejected: only SELECT/WITH/EXPLAIN/TABLE statements are allowed")
	}

	return strings.TrimSpace(body), nil
}

// WrapWithLimit wraps an already-validated SELECT/WITH/TABLE statement in an
// outer LIMIT, regardless of whether the inner query already has one. This
// is a hard ceiling, not a default applied only when a limit is absent — it
// always applies, so an inner LIMIT larger than the cap is still clamped.
func WrapWithLimit(sql string, limit int) string {
	return fmt.Sprintf("SELECT * FROM (%s) AS dbcli_subquery LIMIT %d", sql, limit)
}

// stripComments removes -- line comments and /* */ block comments while
// leaving the contents of '...' strings, "..." identifiers, and $tag$...$tag$
// dollar-quoted strings untouched. It also records the offset (in the
// returned string) of every semicolon encountered outside of those quoted
// contexts, so the caller can detect statement-separating semicolons versus
// ones embedded in string literals.
func stripComments(sql string) (string, []int) {
	runes := []rune(sql)
	n := len(runes)
	var b strings.Builder
	var semicolonOffsets []int

	i := 0
	dollarTag := ""
	inDollar := false
	inSingle := false
	inDouble := false

	for i < n {
		c := runes[i]

		if inDollar {
			closing := "$" + dollarTag + "$"
			if strings.HasPrefix(string(runes[i:]), closing) {
				b.WriteString(closing)
				i += len([]rune(closing))
				inDollar = false
				continue
			}
			b.WriteRune(c)
			i++
			continue
		}

		if inSingle {
			b.WriteRune(c)
			if c == '\'' {
				if i+1 < n && runes[i+1] == '\'' {
					b.WriteRune(runes[i+1])
					i += 2
					continue
				}
				inSingle = false
			}
			i++
			continue
		}

		if inDouble {
			b.WriteRune(c)
			if c == '"' {
				inDouble = false
			}
			i++
			continue
		}

		switch {
		case c == '\'':
			inSingle = true
			b.WriteRune(c)
			i++
		case c == '"':
			inDouble = true
			b.WriteRune(c)
			i++
		case c == '$':
			j := i + 1
			for j < n && (isDollarTagRune(runes[j])) {
				j++
			}
			if j < n && runes[j] == '$' {
				tag := string(runes[i+1 : j])
				dollarTag = tag
				inDollar = true
				b.WriteString(string(runes[i : j+1]))
				i = j + 1
				continue
			}
			b.WriteRune(c)
			i++
		case c == '-' && i+1 < n && runes[i+1] == '-':
			for i < n && runes[i] != '\n' {
				i++
			}
			b.WriteRune(' ')
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i = min(i+2, n)
			b.WriteRune(' ')
		case c == ';':
			semicolonOffsets = append(semicolonOffsets, b.Len())
			b.WriteRune(c)
			i++
		default:
			b.WriteRune(c)
			i++
		}
	}

	return b.String(), semicolonOffsets
}

func isDollarTagRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
