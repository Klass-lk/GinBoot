// Command ginboot-parity sends the same requests to two services and reports
// where their responses differ. See package parity for the configuration.
//
//	ginboot-parity -config parity.yaml [-cases 'cases/B1*.yaml'] [-ref URL] [-cand URL]
//	               [-only name] [-principal name] [-format text|markdown|json] [-out file]
//	               [-show-values] [-failures-only]
//
// It exits 0 when every case matched, 1 when any differed or failed, and 2 on
// a usage or configuration error.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/klass-lk/ginboot/parity"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ginboot-parity", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "parity.yaml", "configuration file")
	casesGlob := fs.String("cases", "", "use only the cases in files matching this glob (relative to the config file)")
	ref := fs.String("ref", "", "override the reference base URL")
	cand := fs.String("cand", "", "override the candidate base URL")
	only := fs.String("only", "", "run only cases whose name contains this text")
	principal := fs.String("principal", "", "run only as this principal")
	format := fs.String("format", "text", "report format: text, markdown or json")
	out := fs.String("out", "", "also write the report to this file")
	showValues := fs.Bool("show-values", false, "print response values in differences (they may contain personal data)")
	failuresOnly := fs.Bool("failures-only", false, "list only failing results")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := parity.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *casesGlob != "" {
		cfg.Cases = nil
		if err := cfg.AddCases(filepath.Join(filepath.Dir(*configPath), *casesGlob)); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	if *ref != "" {
		cfg.Reference.BaseURL = *ref
	}
	if *cand != "" {
		cfg.Candidate.BaseURL = *cand
	}
	if cfg.Reference.BaseURL == "" || cfg.Candidate.BaseURL == "" {
		fmt.Fprintln(stderr, "ginboot-parity: reference and candidate base URLs are required")
		return 2
	}
	filter := func(c parity.Case, p string) bool {
		return (*only == "" || strings.Contains(c.Name, *only)) && (*principal == "" || p == *principal)
	}
	// Resolve only the principals the selected cases need.
	var selected []parity.Case
	for _, c := range cfg.Cases {
		var ps []string
		for _, p := range principalsOf(c) {
			if filter(c, p) {
				ps = append(ps, p)
			}
		}
		if len(ps) > 0 {
			c.Principals = ps
			selected = append(selected, c)
		}
	}
	cfg.Cases = selected

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	principals, err := cfg.ResolvePrincipals(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	report, err := parity.Run(ctx, cfg.Reference.Target(), cfg.Candidate.Target(), principals, cfg.Cases, cfg.Rules,
		parity.Options{AllowMethods: cfg.AllowMethods})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	opts := parity.RenderOptions{ShowValues: *showValues, FailuresOnly: *failuresOnly}
	render := func(w io.Writer) error {
		switch *format {
		case "markdown", "md":
			return report.WriteMarkdown(w, opts)
		case "json":
			return report.WriteJSON(w, opts)
		default:
			return report.WriteText(w, opts)
		}
	}
	if err := render(stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *out != "" {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		defer f.Close()
		if err := render(f); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	if !report.Passed() {
		return 1
	}
	return 0
}

func principalsOf(c parity.Case) []string {
	if len(c.Principals) == 0 {
		return []string{parity.Anonymous}
	}
	return c.Principals
}
