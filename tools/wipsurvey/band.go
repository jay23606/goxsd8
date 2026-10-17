package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
)

// defaultPlanPath is the roadmap whose Working band the report prints,
// resolved against the working directory, so it names docs/PLAN.md only
// when `go tool wipsurvey` runs from the module root.
const defaultPlanPath = "docs/PLAN.md"

// bandHeading is the line that opens docs/PLAN.md's Working band section.
const bandHeading = "### Working band"

// bandSectionEnd matches the heading that closes the band section: the next
// heading of level three or above. A level-four heading inside the band is
// band content.
var bandSectionEnd = regexp.MustCompile(`^#{1,3} `)

// bandRowPattern matches a band row by its first two cells, `| N | #M |`,
// anchored at the line's start, so a `|` or a `#ref` in the row's "why
// here" cell neither adds a row nor shifts one, and a bullet naming issues
// below the band is never read as a row (#2399).
var bandRowPattern = regexp.MustCompile(`^\| *(\d+) *\| *#(\d+) *\|`)

// errNoBand is what parseBand's error wraps when the text carries no band
// section: a band nobody read must not print as an empty band.
var errNoBand = errors.New("no Working band section was read, so no row is reported")

// bandRow is one row of the Working band: its rank, as the band numbers it,
// and the issue it names.
type bandRow struct {
	rank  int
	issue int
}

// readBand reads the Working band rows from the file at path ([parseBand]).
func readBand(path string) ([]bandRow, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the Working band: %w", err)
	}
	rows, err := parseBand(string(text))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

// parseBand returns the band rows of a PLAN.md's text, in band order: every
// line between the `### Working band` heading and the next heading of level
// three or above whose first two cells match bandRowPattern. Rows of every
// tier table count, and nothing else in the section does. A text with no
// such heading returns an error wrapping errNoBand; a section with the
// heading and no row returns no rows and no error, which the report prints
// as an empty band. It is pure, so tests exercise it against fixture text.
func parseBand(text string) ([]bandRow, error) {
	var rows []bandRow
	found := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == bandHeading {
			found = true
			continue
		}
		if !found {
			continue
		}
		if bandSectionEnd.MatchString(line) {
			break
		}
		m := bandRowPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rank, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("parsing band row rank from %q: %w", line, err)
		}
		issue, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("parsing band row issue from %q: %w", line, err)
		}
		rows = append(rows, bandRow{rank: rank, issue: issue})
	}
	if !found {
		return nil, fmt.Errorf("no %q heading: %w", bandHeading, errNoBand)
	}
	return rows, nil
}

// issueStatus is a band row's issue state as the fed list gives it.
type issueStatus string

const (
	statusOpen   issueStatus = "OPEN"
	statusClosed issueStatus = "CLOSED"
	// statusUnknown means no issue list was fed: stdin was empty, or the
	// fed list was discarded for a row missing `state` (#1604).
	statusUnknown issueStatus = "UNKNOWN"
	// statusUnlisted means a list was fed and this issue is not in it.
	statusUnlisted issueStatus = "UNLISTED"
)

// bandStatus is issue n's state from issues, readIssues's result: nil when
// no list was fed, which no row may read as OPEN.
func bandStatus(n int, issues map[int]issueState) issueStatus {
	if issues == nil {
		return statusUnknown
	}
	state, ok := issues[n]
	if !ok {
		return statusUnlisted
	}
	if state.closed {
		return statusClosed
	}
	return statusOpen
}

// bandLine is one row of the report's band section. lease is the classified
// wip/issue-<M> row of the same issue, nil when the remote carries no such
// branch.
type bandLine struct {
	row    bandRow
	status issueStatus
	lease  *row
}

// bandLines joins each band row, in band order, to its issue's state and to
// its branch's classified row. rows is looked up, never iterated into the
// output.
func bandLines(band []bandRow, issues map[int]issueState, rows []row) []bandLine {
	byIssue := make(map[int]*row, len(rows))
	for i := range rows {
		byIssue[rows[i].issue] = &rows[i]
	}
	lines := make([]bandLine, 0, len(band))
	for _, b := range band {
		lines = append(lines, bandLine{row: b, status: bandStatus(b.issue, issues), lease: byIssue[b.issue]})
	}
	return lines
}

// bandSection is what the report's band section renders: the file it read,
// its lines, and the error that kept it from reading them, if any.
type bandSection struct {
	path  string
	lines []bandLine
	err   error
}

// renderBand writes the band section: ROW, ISSUE, STATE, BRANCH, VERDICT per
// band row, in band order. A row with no wip/issue-<M> head prints "none"
// and "unclaimed" rather than blank cells. A section with no row prints a
// "none" line, and a band that was not read prints an UNREAD line instead,
// so neither reads like the other.
//
// docs/ROUTINES.md's "Supplying the thread's comments" selects lease rows
// by a verdict in the fourth field; this section's fourth field is BRANCH,
// so no band line is selected.
func renderBand(w io.Writer, s bandSection) error {
	if _, err := fmt.Fprintf(w, "\nWORKING BAND -- %s's rows in band order; STATE from the fed issue list (UNKNOWN: none fed; UNLISTED: not in it)\n", s.path); err != nil {
		return fmt.Errorf("writing band section heading: %w", err)
	}
	if s.err != nil {
		if _, err := fmt.Fprintf(w, "UNREAD -- %v\n", s.err); err != nil {
			return fmt.Errorf("writing unread band section: %w", err)
		}
		return nil
	}
	if len(s.lines) == 0 {
		if _, err := fmt.Fprintf(w, "none -- %s's band section was read and carries no `| N | #M |` row\n", s.path); err != nil {
			return fmt.Errorf("writing empty band section: %w", err)
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ROW\tISSUE\tSTATE\tBRANCH\tVERDICT"); err != nil {
		return fmt.Errorf("writing band table header: %w", err)
	}
	for _, l := range s.lines {
		branch, class := "none", "unclaimed"
		if l.lease != nil {
			branch, class = l.lease.branch, string(l.lease.verdict)
		}
		if _, err := fmt.Fprintf(tw, "%d\t#%d\t%s\t%s\t%s\n", l.row.rank, l.row.issue, l.status, branch, class); err != nil {
			return fmt.Errorf("writing band table row for issue #%d: %w", l.row.issue, err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing band table: %w", err)
	}
	return nil
}
