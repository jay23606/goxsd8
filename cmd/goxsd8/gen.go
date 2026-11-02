package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/kud360/goxsd8/codegen"
)

// The two flags a gen pair is spelled with, named once for the flag set and
// for pairTargets' diagnoses.
const (
	schemaFlag = "schema"
	outFlag    = "out"
)

// genFlag is one occurrence of -schema or -out, in argument order. The flag
// package hands each repeatable flag its values without the interleaving, and
// the interleaving is the pairing, so both flags record into one sequence.
type genFlag struct {
	name, value string
}

// genPair is one -schema/-out pair as the user spelled it.
type genPair struct {
	schema, out string
}

// runGen implements `goxsd8 gen`: it checks every -out through
// codegen.CheckTargets before compiling any -schema, compiles the -schema of
// each -schema/-out pair alone, as parse compiles an argument, and, when no
// -out was refused and every -schema compiled, calls codegen.Generate once with
// one Target per pair in argument order. args excludes the subcommand name
// itself.
//
// runGen restates none of codegen's checks (STYLE D3): it charges each refusal
// CheckTargets reports exit 2, and hands Generate's error to reportGenerate.
// Each Target is named by its pair's label, so every error codegen returns
// already names the pair it came from, and any earlier pair it cites.
//
// The flags are parsed here rather than by run, for the reason runParse states.
func runGen(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	// Discarding the flag set's output silences its own usage rendering, which
	// would print a second, narrower contract next to doc.go's; helpPointer
	// names the real one instead.
	flags.SetOutput(io.Discard)
	var occurrences []genFlag
	for _, name := range []string{schemaFlag, outFlag} {
		flags.Func(name, "one half of a -schema/-out pair", func(v string) error {
			occurrences = append(occurrences, genFlag{name, v})
			return nil
		})
	}
	verbose := flags.Bool("v", false, "log the parser's assembly at debug level to stderr")
	// -q is defined because the contract makes it common to every subcommand,
	// and is read by nothing: gen writes no informational output.
	_ = flags.Bool("q", false, "accepted; gen has no informational output to suppress")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return subcommandUsageError(stderr, "gen", fmt.Sprintf(helpNotAFlagValueFmt, "gen"))
		}
		return subcommandUsageError(stderr, "gen", fmt.Sprintf("goxsd8: gen: %v", err))
	}
	if arg, ok := flagShapedIn(flags.Args()); ok {
		return subcommandUsageError(stderr, "gen", fmt.Sprintf(flagAfterPositionalFmt, "gen", arg))
	}
	if len(flags.Args()) > 0 {
		return subcommandUsageError(stderr, "gen", fmt.Sprintf("goxsd8: gen: %s: gen takes no positional argument, only -schema/-out pairs", flags.Arg(0)))
	}
	pairs, err := pairTargets(occurrences)
	if err != nil {
		return subcommandUsageError(stderr, "gen", fmt.Sprintf("goxsd8: gen: %v", err))
	}

	// A nil logger selects parser's silent default, for the reason parse.go's
	// GAP(cmd) marker records (#1185).
	var log *slog.Logger
	if *verbose {
		log = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	// Every -out is checked before any -schema is compiled, and every pair is
	// compiled and reported before Generate is called, as usage's gen block
	// states.
	targets := make([]codegen.Target, len(pairs))
	for i, p := range pairs {
		targets[i] = codegen.Target{
			Dir:     p.out,
			Package: filepath.Base(filepath.Clean(p.out)),
			Name:    p.label(),
		}
	}
	code := exitOK
	for _, err := range codegen.CheckTargets(targets) {
		code = worse(code, subcommandUsageError(stderr, "gen", "goxsd8: gen: "+err.Error()))
	}
	for i, p := range pairs {
		schema, c := compileArgument("gen", p.schema, log, stderr)
		code = worse(code, c)
		targets[i].Schema = schema
	}
	if code != exitOK {
		return code
	}
	return reportGenerate(codegen.Generate(targets), stderr)
}

// reportGenerate prints each element of codegen.Generate's error err on its
// own line on stderr and returns the exit code they charge, exitOK for a nil
// err. An element holding a *codegen.DeclineError, which errors.As retrieves,
// charges exit 4 with no synopsis or helpPointer under it; every other element
// charges exit 2 as subcommandUsageError prints it. errors.Is(e,
// errors.ErrUnsupported) does not route: it also answers true for an OS fault
// whose syscall.Errno is ENOSYS, ENOTSUP or EOPNOTSUPP, which Generate wraps
// with %w and which is no decline (#3090).
func reportGenerate(err error, stderr io.Writer) int {
	if err == nil {
		return exitOK
	}
	errs := []error{err}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		errs = j.Unwrap()
	}
	code := exitOK
	for _, e := range errs {
		line := "goxsd8: gen: " + e.Error()
		var de *codegen.DeclineError
		if !errors.As(e, &de) {
			code = worse(code, subcommandUsageError(stderr, "gen", line))
			continue
		}
		// A failed stderr write cannot change the outcome: the exit code is
		// settled either way, and stderr is the only channel this line has.
		_, _ = fmt.Fprintln(stderr, line)
		code = worse(code, exitUndecided)
	}
	return code
}

// pairTargets reads the -schema/-out occurrences as pairs, each -schema
// followed by its own -out, and fails on the first occurrence that breaks the
// alternation, or on none at all.
func pairTargets(occurrences []genFlag) ([]genPair, error) {
	var pairs []genPair
	for i := 0; i < len(occurrences); i += 2 {
		first := occurrences[i]
		if first.name != schemaFlag {
			return nil, fmt.Errorf("-%s %s stands without a -%s before it", outFlag, first.value, schemaFlag)
		}
		if i+1 == len(occurrences) || occurrences[i+1].name != outFlag {
			return nil, fmt.Errorf("-%s %s stands without its -%s", schemaFlag, first.value, outFlag)
		}
		pairs = append(pairs, genPair{schema: first.value, out: occurrences[i+1].value})
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no -%s/-%s pair given", schemaFlag, outFlag)
	}
	return pairs, nil
}

// label names a pair on a diagnosis as its user spelled it.
func (p genPair) label() string {
	return fmt.Sprintf("-%s %s -%s %s", schemaFlag, p.schema, outFlag, p.out)
}
