// Package parity compares the responses of two HTTP services that are meant to
// answer identically — typically an existing API (the reference) and its port
// to Ginboot (the candidate). It is the tool behind a path-by-path migration:
// send the same requests to both, normalise what is allowed to differ, and
// report everything else.
//
// The package is safe to point at production. Run refuses every method except
// GET and HEAD unless Options.AllowMethods says otherwise, and reports show
// hashes instead of response values unless values are asked for.
package parity

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Rules says which differences between two responses are acceptable. The zero
// value is strict everywhere except number formatting (see StrictNumbers).
//
// Paths use a small JSONPath subset, always rooted at "$":
//
//	$.user.name        a field
//	$.items[0]         an array element
//	$.items[*].id      every element
//	$.*.id             every field of an object
//	$..signedUrl       a field at any depth
type Rules struct {
	// Ignore lists paths whose values are never compared. A path that is
	// missing on one side is still reported unless it is ignored too, which
	// it is: an ignored path is skipped entirely.
	Ignore []string `yaml:"ignore" json:"ignore,omitempty"`

	// Unordered lists arrays compared without regard to order. The value is
	// the field that identifies an element ("id"), or "" to compare the two
	// arrays as multisets.
	Unordered map[string]string `yaml:"unordered" json:"unordered,omitempty"`

	// NullIsMissing lists paths where a null on one side and an absent field
	// on the other count as equal. Use "$..*" for the whole document.
	NullIsMissing []string `yaml:"nullIsMissing" json:"nullIsMissing,omitempty"`

	// TimeAsInstant compares two strings that both parse as timestamps by the
	// instant they denote, so "2026-01-02T10:00:00+05:30" equals
	// "2026-01-02T04:30:00Z".
	TimeAsInstant bool `yaml:"timeAsInstant" json:"timeAsInstant,omitempty"`

	// TimeTolerance is the largest gap between two instants still counted as
	// equal. Only used with TimeAsInstant.
	TimeTolerance time.Duration `yaml:"timeTolerance" json:"timeTolerance,omitempty"`

	// StrictNumbers compares numbers by their JSON text, so 5 and 5.0 differ.
	// By default numbers are compared by value.
	StrictNumbers bool `yaml:"strictNumbers" json:"strictNumbers,omitempty"`

	// Headers lists response headers that must match. Content-Type is compared
	// by media type alone, so "application/json; charset=utf-8" equals
	// "application/json". Nil means Content-Type only; an empty slice means none.
	Headers []string `yaml:"headers" json:"headers,omitempty"`

	// IgnoreStatus skips the status-code comparison.
	IgnoreStatus bool `yaml:"ignoreStatus" json:"ignoreStatus,omitempty"`
}

// Merge returns r with other's settings added: lists are concatenated, map
// entries and true flags from other win.
func (r Rules) Merge(other Rules) Rules {
	out := r
	out.Ignore = append(append([]string{}, r.Ignore...), other.Ignore...)
	out.NullIsMissing = append(append([]string{}, r.NullIsMissing...), other.NullIsMissing...)
	if len(r.Unordered)+len(other.Unordered) > 0 {
		out.Unordered = map[string]string{}
		for k, v := range r.Unordered {
			out.Unordered[k] = v
		}
		for k, v := range other.Unordered {
			out.Unordered[k] = v
		}
	}
	out.TimeAsInstant = r.TimeAsInstant || other.TimeAsInstant
	if other.TimeTolerance > out.TimeTolerance {
		out.TimeTolerance = other.TimeTolerance
	}
	out.StrictNumbers = r.StrictNumbers || other.StrictNumbers
	if other.Headers != nil {
		out.Headers = append(append([]string{}, r.Headers...), other.Headers...)
	}
	out.IgnoreStatus = r.IgnoreStatus || other.IgnoreStatus
	return out
}

// compiledRules is Rules with every path turned into a regular expression
// over concrete paths such as `$.items[3].id`.
type compiledRules struct {
	Rules
	ignore        []*regexp.Regexp
	nullIsMissing []*regexp.Regexp
	unordered     []unorderedRule
}

type unorderedRule struct {
	path *regexp.Regexp
	key  string
}

func (r Rules) compile() (*compiledRules, error) {
	c := &compiledRules{Rules: r}
	var err error
	if c.ignore, err = compilePatterns(r.Ignore); err != nil {
		return nil, err
	}
	if c.nullIsMissing, err = compilePatterns(r.NullIsMissing); err != nil {
		return nil, err
	}
	for p, key := range r.Unordered {
		re, err := compilePattern(p)
		if err != nil {
			return nil, err
		}
		c.unordered = append(c.unordered, unorderedRule{path: re, key: key})
	}
	return c, nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := compilePattern(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

const (
	keySegment   = `\.[^.\[]+`
	indexSegment = `\[\d+\]`
)

// compilePattern turns a path pattern into a regular expression that matches
// concrete paths. A pattern matches its node only, not the node's children:
// ignoring "$.a" stops the walk at "$.a", so its children are never visited.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if !strings.HasPrefix(pattern, "$") {
		return nil, fmt.Errorf("parity: path %q must start with $", pattern)
	}
	var b strings.Builder
	b.WriteString(`^\$`)
	rest := pattern[1:]
	for len(rest) > 0 {
		switch {
		case strings.HasPrefix(rest, ".."):
			// Recursive descent: any run of segments, then the next one.
			b.WriteString(`(?:` + keySegment + `|` + indexSegment + `)*`)
			rest = "." + rest[2:]
		case strings.HasPrefix(rest, ".*"):
			b.WriteString(keySegment)
			rest = rest[2:]
		case strings.HasPrefix(rest, "[*]"):
			b.WriteString(indexSegment)
			rest = rest[3:]
		case rest[0] == '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("parity: unclosed [ in path %q", pattern)
			}
			b.WriteString(regexp.QuoteMeta(rest[:end+1]))
			rest = rest[end+1:]
		case rest[0] == '.':
			end := strings.IndexAny(rest[1:], ".[")
			if end < 0 {
				end = len(rest) - 1
			}
			name := rest[1 : end+1]
			if name == "" {
				return nil, fmt.Errorf("parity: empty field name in path %q", pattern)
			}
			b.WriteString(regexp.QuoteMeta("." + name))
			rest = rest[end+1:]
		default:
			return nil, fmt.Errorf("parity: unexpected %q in path %q", rest[:1], pattern)
		}
	}
	b.WriteString(`$`)
	return regexp.Compile(b.String())
}

func matchesAny(res []*regexp.Regexp, path string) bool {
	for _, re := range res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func (c *compiledRules) unorderedKey(path string) (string, bool) {
	for _, u := range c.unordered {
		if u.path.MatchString(path) {
			return u.key, true
		}
	}
	return "", false
}
