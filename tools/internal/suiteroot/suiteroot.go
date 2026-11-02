// Package suiteroot names the W3C suite checkout the corpus-reading tools
// default to, and renders the corpus-absent notice they print when it is not
// there. It is the one home of both for `go tool gencensus`, `go tool
// suiteindex`, `go tool casejoin` and `go tool lanepartition` (STYLE T4);
// lanepartition takes only the root, and fails on an absent catalog instead
// of printing the notice.
//
// [PrintAbsent] returns its write's error, and every caller passes that error
// up to its own exit: a notice that did not reach its reader is a failure,
// whatever the mode it reports.
package suiteroot

import (
	"fmt"
	"io"
)

// Default is the W3C suite submodule at its fixed path in the tree, as written
// from the module root. It is the root each tool reads when none is given, and
// the path the corpus-absent notice tells the reader to initialize.
const Default = "testdata/xsdtests"

// PrintAbsent writes the corpus-absent notice to w: a first line naming the
// tool, the subject that is absent (`<root> <reason>`, or `no suite index at
// <path>`) and the activity there is nothing to do (`census`, `join`), then
// the command that initializes the suite at [Default], whatever root the
// caller was given.
func PrintAbsent(w io.Writer, tool, subject, activity string) error {
	_, err := fmt.Fprintf(w, "%s: %s — corpus-absent mode, nothing to %s.\n"+
		"Initialize the suite with `git submodule update --init %s` (#659).\n", tool, subject, activity, Default)
	return err
}
