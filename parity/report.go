package parity

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderOptions control how a report is written.
type RenderOptions struct {
	// ShowValues prints response values in differences. By default values are
	// replaced by their type, size and a short hash, because responses often
	// hold personal data and reports get pasted into tickets.
	ShowValues bool

	// MaxDiffs caps the differences listed per result. Zero means 20.
	MaxDiffs int

	// FailuresOnly leaves passed and skipped results out.
	FailuresOnly bool
}

// WriteText writes a plain-text report.
func (r *Report) WriteText(w io.Writer, o RenderOptions) error {
	passed, failed, skipped := r.Counts()
	fmt.Fprintf(w, "parity: %s (reference) vs %s (candidate)\n", r.Reference, r.Candidate)
	for _, res := range r.Results {
		if o.FailuresOnly && (res.Passed() || res.Skipped != "") {
			continue
		}
		fmt.Fprintf(w, "%-4s %-10s %s %s%s\n", status(res), res.Principal, res.Method, res.Path, statusPair(res))
		if res.Skipped != "" {
			fmt.Fprintf(w, "       skipped: %s\n", res.Skipped)
		}
		if res.Err != "" {
			fmt.Fprintf(w, "       error: %s\n", res.Err)
		}
		for i, d := range res.Diffs {
			if i == maxDiffs(o) {
				fmt.Fprintf(w, "       … %d more\n", len(res.Diffs)-i)
				break
			}
			fmt.Fprintf(w, "       %-7s %s%s\n", d.Kind, d.Path, values(d, o))
		}
	}
	_, err := fmt.Fprintf(w, "\n%d passed, %d failed, %d skipped\n", passed, failed, skipped)
	return err
}

// WriteMarkdown writes a Markdown report: a summary table, then the
// differences of every failing result.
func (r *Report) WriteMarkdown(w io.Writer, o RenderOptions) error {
	passed, failed, skipped := r.Counts()
	fmt.Fprintf(w, "# Parity report: %s vs %s\n\n", r.Reference, r.Candidate)
	fmt.Fprintf(w, "%s · **%d passed, %d failed, %d skipped**\n\n", r.Started.Format("2006-01-02 15:04 MST"), passed, failed, skipped)
	fmt.Fprintf(w, "| | Principal | Request | %s | %s | Diffs |\n|---|---|---|---|---|---|\n", r.Reference, r.Candidate)
	for _, res := range r.Results {
		if o.FailuresOnly && (res.Passed() || res.Skipped != "") {
			continue
		}
		note := fmt.Sprint(len(res.Diffs))
		switch {
		case res.Skipped != "":
			note = "skipped: " + res.Skipped
		case res.Err != "":
			note = "error: " + res.Err
		}
		fmt.Fprintf(w, "| %s | %s | `%s %s` | %s | %s | %s |\n", status(res), res.Principal, res.Method, res.Path,
			code(res.RefStatus), code(res.CandStatus), strings.ReplaceAll(note, "|", `\|`))
	}
	for _, res := range r.Results {
		if len(res.Diffs) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## %s — %s `%s %s`\n\n", res.Case, res.Principal, res.Method, res.Path)
		for i, d := range res.Diffs {
			if i == maxDiffs(o) {
				fmt.Fprintf(w, "- … %d more\n", len(res.Diffs)-i)
				break
			}
			fmt.Fprintf(w, "- **%s** `%s`%s\n", d.Kind, d.Path, values(d, o))
		}
	}
	return nil
}

// WriteJSON writes the report as JSON, redacting values unless ShowValues.
func (r *Report) WriteJSON(w io.Writer, o RenderOptions) error {
	out := *r
	if !o.ShowValues {
		out.Results = make([]Result, len(r.Results))
		for i, res := range r.Results {
			res.Diffs = append([]Difference(nil), res.Diffs...)
			for j, d := range res.Diffs {
				res.Diffs[j].Ref, res.Diffs[j].Cand = redact(d, d.Ref), redact(d, d.Cand)
			}
			out.Results[i] = res
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func status(r Result) string {
	switch {
	case r.Skipped != "":
		return "SKIP"
	case r.Passed():
		return "ok"
	}
	return "FAIL"
}

func statusPair(r Result) string {
	if r.RefStatus == 0 && r.CandStatus == 0 {
		return ""
	}
	return fmt.Sprintf("  [%s %s]", code(r.RefStatus), code(r.CandStatus))
}

func code(c int) string {
	if c == 0 {
		return "—"
	}
	return fmt.Sprint(c)
}

func maxDiffs(o RenderOptions) int {
	if o.MaxDiffs == 0 {
		return 20
	}
	return o.MaxDiffs
}

func values(d Difference, o RenderOptions) string {
	show := func(v any) string {
		if o.ShowValues {
			b, _ := json.Marshal(v)
			s := string(b)
			if len(s) > 120 {
				s = s[:117] + "..."
			}
			return s
		}
		return fmt.Sprint(redact(d, v))
	}
	switch d.Kind {
	case DiffMissing:
		return "  ref=" + show(d.Ref)
	case DiffExtra:
		return "  cand=" + show(d.Cand)
	}
	return "  ref=" + show(d.Ref) + " cand=" + show(d.Cand)
}

// redact keeps values that describe the response rather than its data —
// status codes, lengths, type names, media types — and hashes everything else.
func redact(d Difference, v any) any {
	switch d.Kind {
	case DiffStatus, DiffLength:
		return v
	case DiffType:
		if s, ok := v.(string); ok && isTypeName(s) {
			return s
		}
		return typeName(v)
	case DiffHeader:
		if strings.EqualFold(d.Path, "header.Content-Type") {
			return v
		}
	}
	return describe(v)
}

func isTypeName(s string) bool {
	switch s {
	case "null", "boolean", "number", "string", "array", "object":
		return true
	}
	return false
}
