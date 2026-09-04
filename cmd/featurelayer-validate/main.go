// Command featurelayer-validate checks a featurelayer JSON config the
// way the library will at startup: it decodes the document strictly
// and runs the same validation NewSnapshot runs, then prints either a
// one-line summary or every problem found. It exits 0 only when the
// config would load. Meant for CI:
//
//	featurelayer-validate features.json
//	cat features.json | featurelayer-validate
//	featurelayer-validate -           # explicit stdin
//
// Exit status: 0 valid, 1 invalid or unreadable, 2 usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	featurelayer "github.com/bernardoforcillo/featurelayer"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is main without the process boundary, so tests can drive it.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("featurelayer-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	quiet := fs.Bool("q", false, "print nothing on success")
	fs.Usage = func() {
		say(stderr, "usage: featurelayer-validate [-q] [path | -]\n")
		say(stderr, "Validates a featurelayer JSON config; reads stdin when no path (or \"-\") is given.\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}

	var (
		snap *featurelayer.Snapshot
		err  error
		name = "stdin"
	)
	if fs.NArg() == 1 && fs.Arg(0) != "-" {
		name = fs.Arg(0)
		snap, err = featurelayer.LoadFile(name)
	} else {
		snap, err = featurelayer.LoadJSON(stdin)
	}
	if err != nil {
		report(stderr, name, err)
		return 1
	}
	if !*quiet {
		say(stdout, "%s: ok — %d features, %d segments, %d flags, %d plans, %d addons\n",
			name, len(snap.Features()), len(snap.Segments()), countFlags(snap), len(snap.Plans()), len(snap.AddOns()))
	}
	return 0
}

// report prints one line per problem. Validation failures arrive as an
// errors.Join of *ValidationError; anything else (unreadable file,
// malformed JSON) is a single line.
func report(w io.Writer, name string, err error) {
	errs := []error{err}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		errs = u.Unwrap()
	}
	n := 0
	for _, e := range errs {
		var ve *featurelayer.ValidationError
		if errors.As(e, &ve) {
			n++
			say(w, "%s: %s: %s\n", name, ve.Path, ve.Msg)
			continue
		}
		say(w, "%s: %v\n", name, e)
	}
	if n > 0 {
		say(w, "%s: invalid — %d problem(s)\n", name, n)
	}
}

// say writes to a CLI stream. A write error on stdout/stderr has no
// useful handling in a command that is about to exit, so it is
// deliberately dropped.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// countFlags counts flags through the features: Snapshot exposes flags
// by feature key only.
func countFlags(snap *featurelayer.Snapshot) int {
	n := 0
	for _, f := range snap.Features() {
		if _, ok := snap.Flag(f.Key); ok {
			n++
		}
	}
	return n
}
