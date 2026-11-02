package suiteroot

import (
	"errors"
	"strings"
	"testing"
)

// TestPrintAbsent pins the notice byte for byte: the tools' corpus-absent
// output is read by agents and quoted in LOG entries, so a drift in it is a
// change in what every caller prints.
func TestPrintAbsent(t *testing.T) {
	var out strings.Builder
	if err := PrintAbsent(&out, "casejoin", "no suite index at /nowhere/suite.xml", "join"); err != nil {
		t.Fatalf("PrintAbsent: %v", err)
	}
	want := "casejoin: no suite index at /nowhere/suite.xml — corpus-absent mode, nothing to join.\n" +
		"Initialize the suite with `git submodule update --init testdata/xsdtests` (#659).\n"
	if out.String() != want {
		t.Errorf("notice =\n%s\nwant\n%s", out.String(), want)
	}
}

// failWriter refuses every write, standing in for a closed pipe.
type failWriter struct{}

var errRefused = errors.New("refused")

func (failWriter) Write([]byte) (int, error) { return 0, errRefused }

// TestPrintAbsentWriteError pins the package's write-error policy: the
// notice's write error is returned, not dropped.
func TestPrintAbsentWriteError(t *testing.T) {
	if err := PrintAbsent(failWriter{}, "gencensus", "x does not exist", "census"); !errors.Is(err, errRefused) {
		t.Errorf("PrintAbsent over a failing writer = %v, want %v", err, errRefused)
	}
}
