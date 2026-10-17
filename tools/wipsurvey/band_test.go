package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// bandFixture is a roadmap shaped like docs/PLAN.md's: a table before the
// band that is not band content, three tier tables whose "why here" cells
// carry a `|` and #refs, a level-four heading inside the band, the bullets
// named below the band, and a section after it whose table is not band
// content either.
const bandFixture = `# Roadmap

### Persona consultations

| # | issue | why here |
|---|---|---|
| 1 | #500 | before the band, not a row |

### Working band

This is ordered for a /develop session.

**Tier A: lanes.**

| # | issue | why here |
|---|---|---|
| 1 | #10 | names #99 and | 7 | #98 | inside its cell |

**Tier B.**

| # | issue | why here |
|---|---|---|
| 2 | #20 | closed since the stamp |
|3|#30|unspaced cells |

#### A sub-heading inside the band

| 4 | #40 | missing from the fed list |

**Named below the band, on purpose.**
- **Other:** #50 (not a row) and #60.
- | 5 | #70 | a bullet is not a row

### Next planning action

| 6 | #80 | after the band, not a row |
`

// bandFixtureRows is bandFixture's band, in band order.
var bandFixtureRows = []bandRow{{1, 10}, {2, 20}, {3, 30}, {4, 40}}

// TestParseBand pins which lines are band rows: only `| N | #M |` lines
// between the heading and the next heading of level three or above, in
// file order.
func TestParseBand(t *testing.T) {
	got, err := parseBand(bandFixture)
	if err != nil {
		t.Fatalf("parseBand: %v", err)
	}
	if !reflect.DeepEqual(got, bandFixtureRows) {
		t.Errorf("parseBand = %+v, want %+v", got, bandFixtureRows)
	}
}

// TestParseBandMissingSection pins that a roadmap with no band section is
// an error, never an empty band, and that a band section with no row is an
// empty band, never an error.
func TestParseBandMissingSection(t *testing.T) {
	cut := strings.Index(bandFixture, "### Working band")
	end := strings.Index(bandFixture, "### Next planning action")
	noBand := bandFixture[:cut] + bandFixture[end:]
	rows, err := parseBand(noBand)
	if !errors.Is(err, errNoBand) || rows != nil {
		t.Errorf("parseBand without the band = %+v, %v; want no rows and errNoBand", rows, err)
	}

	rows, err = parseBand("### Working band\n\nNothing ranked.\n\n### Next planning action\n| 1 | #2 | x |\n")
	if err != nil || len(rows) != 0 {
		t.Errorf("parseBand of an empty band = %+v, %v; want no rows and no error", rows, err)
	}
}

// TestReadBand reads the fixture from a file, the way run reads the -plan
// path, and names the path when the file carries no band.
func TestReadBand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PLAN.md")
	if err := os.WriteFile(path, []byte(bandFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readBand(path)
	if err != nil || !reflect.DeepEqual(got, bandFixtureRows) {
		t.Errorf("readBand = %+v, %v; want %+v", got, err, bandFixtureRows)
	}

	if err := os.WriteFile(path, []byte("# Roadmap\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBand(path); !errors.Is(err, errNoBand) || !strings.HasPrefix(err.Error(), path+": ") {
		t.Errorf("readBand without a band = %v; want errNoBand prefixed with %q", err, path)
	}

	if _, err := readBand(filepath.Join(dir, "absent.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("readBand of a missing file = %v; want os.ErrNotExist", err)
	}
}

// bandFeed is the fixture issue list: #10 open, #20 closed, #30 open with
// needs-replan, and #40 absent.
const bandFeed = `[{"number":10,"state":"OPEN","labels":[]},
	{"number":20,"state":"CLOSED","labels":[]},
	{"number":30,"state":"OPEN","labels":[{"name":"needs-replan"}]}]`

// TestBandReport renders the fixture band against the fixture issue list
// and one claimed branch, and checks every line of the section.
func TestBandReport(t *testing.T) {
	issues, err := readIssues(strings.NewReader(bandFeed))
	if err != nil {
		t.Fatalf("readIssues: %v", err)
	}
	band, err := parseBand(bandFixture)
	if err != nil {
		t.Fatalf("parseBand: %v", err)
	}
	rows := []row{
		{issue: 10, branch: "wip/issue-10", anc: ancestryNoCommits, verdict: claimed, reason: "x"},
		{issue: 77, branch: "wip/issue-77", anc: ancestryOwnCommits, verdict: expired, reason: "x"},
	}
	var buf bytes.Buffer
	if err := renderBand(&buf, bandSection{path: "PLAN.md", lines: bandLines(band, issues, rows)}); err != nil {
		t.Fatalf("renderBand: %v", err)
	}
	want := []string{
		"",
		"WORKING BAND -- PLAN.md's rows in band order; STATE from the fed issue list (UNKNOWN: none fed; UNLISTED: not in it)",
		"ROW  ISSUE  STATE     BRANCH        VERDICT",
		"1    #10    OPEN      wip/issue-10  CLAIMED",
		"2    #20    CLOSED    none          unclaimed",
		"3    #30    OPEN      none          unclaimed",
		"4    #40    UNLISTED  none          unclaimed",
		"",
	}
	if got := strings.Split(buf.String(), "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("band section =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestBandStatusWithoutAList pins that no list — empty stdin, or a feed
// discarded for a row missing `state` — reads UNKNOWN on every row, never
// OPEN, and that a fed empty list reads UNLISTED. Each case pins
// readIssues' error too, so the stateless feed reads UNKNOWN through
// errMissingState and not through some other parse failure.
func TestBandStatusWithoutAList(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		wantErr  error
		want     issueStatus
	}{
		{"empty stdin", "", nil, statusUnknown},
		{"stateless feed", `[{"number":10,"labels":[]}]`, errMissingState, statusUnknown},
		{"empty list", `[]`, nil, statusUnlisted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues, err := readIssues(strings.NewReader(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("readIssues error = %v, want %v", err, tc.wantErr)
			}
			if got := bandStatus(10, issues); got != tc.want {
				t.Errorf("bandStatus = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestBandSectionNeverReadsEmpty pins the two lines that are not rows: an
// unread band says UNREAD and carries no "none" line, and a band read with
// no row says "none".
func TestBandSectionNeverReadsEmpty(t *testing.T) {
	var buf bytes.Buffer
	_, readErr := readBand(filepath.Join(t.TempDir(), "absent.md"))
	if err := renderBand(&buf, bandSection{path: "absent.md", err: readErr}); err != nil {
		t.Fatalf("renderBand: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "\nUNREAD -- reading the Working band: ") || strings.Contains(got, "none --") {
		t.Errorf("unread band section =\n%s\nwant an UNREAD line and no none line", got)
	}

	buf.Reset()
	if err := renderBand(&buf, bandSection{path: "PLAN.md"}); err != nil {
		t.Fatalf("renderBand: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "\nnone -- PLAN.md's band section was read") {
		t.Errorf("empty band section =\n%s\nwant a none line", got)
	}
}

// TestRenderReportReturnsBandError pins that an unread band still renders
// every section before its error is returned, joined with the feed's.
func TestRenderReportReturnsBandError(t *testing.T) {
	errFeed := errors.New("feed")
	var buf bytes.Buffer
	err := renderReport(&buf, nil, nil, nil, bandSection{path: "PLAN.md", err: errNoBand}, errFeed)
	if !errors.Is(err, errNoBand) || !errors.Is(err, errFeed) {
		t.Errorf("renderReport err = %v, want both the band's and the feed's", err)
	}
	if !strings.Contains(buf.String(), "UNREAD -- ") {
		t.Errorf("report =\n%s\nwant the UNREAD line", buf.String())
	}
}

// TestBandLinesEscapeTheCommentsRecipe runs docs/ROUTINES.md's "Supplying
// the thread's comments" selector, `$4 ~ /^(CLAIMED|EXPIRED)$/ {print $1}`,
// over a whole report and checks it yields only the lease table's issues,
// never a band row's rank.
func TestBandLinesEscapeTheCommentsRecipe(t *testing.T) {
	band := []bandRow{{1, 10}, {2, 11}}
	rows := []row{
		{issue: 10, branch: "wip/issue-10", anc: ancestryNoCommits, verdict: claimed, reason: "x"},
		{issue: 11, branch: "wip/issue-11", anc: ancestryOwnCommits, verdict: expired, reason: "x"},
	}
	var buf bytes.Buffer
	if err := renderReport(&buf, rows, nil, nil, bandSection{path: "PLAN.md", lines: bandLines(band, nil, rows)}, nil); err != nil {
		t.Fatalf("renderReport: %v", err)
	}
	var got []string
	for _, line := range strings.Split(buf.String(), "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && (f[3] == "CLAIMED" || f[3] == "EXPIRED") {
			got = append(got, f[0])
		}
	}
	if want := []string{"10", "11"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recipe selects %v, want %v:\n%s", got, want, buf.String())
	}
}
