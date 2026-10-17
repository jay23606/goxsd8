package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// answer is one scripted reply to a page request: raw stdout, or err.
type answer struct {
	raw string
	err error
}

// rows renders n listing entries numbered from, from+1, …, as gh prints them.
func rows(from, n int) answer {
	entries := make([]string, 0, n)
	for i := range n {
		entries = append(entries, fmt.Sprintf(`{"number":%d,"title":"t%d","state":"open","body":"b","labels":[{"name":"ready","color":"x"}]}`, from+i, from+i))
	}
	return answer{raw: "[" + strings.Join(entries, ",") + "]"}
}

// fake is a page source scripted per (page, attempt): script[p][a] answers
// attempt a+1 of page p, the last entry repeating for later attempts, and an
// unscripted page answers []. It records every page requested, in order.
type fake struct {
	script map[int][]answer
	calls  []int
}

func (f *fake) fetch(page int) ([]byte, error) {
	attempt := 0
	for _, c := range f.calls {
		if c == page {
			attempt++
		}
	}
	f.calls = append(f.calls, page)
	as, ok := f.script[page]
	if !ok {
		return []byte("[]"), nil
	}
	a := as[min(attempt, len(as)-1)]
	if a.err != nil {
		return nil, a.err
	}
	return []byte(a.raw), nil
}

// feed runs the command against f with a fresh -dir pre-seeded with stale
// outputs, returning the dir, stdout, stderr and run's error.
func feed(t *testing.T, f *fake) (dir, stdout, stderr string, err error) {
	t.Helper()
	dir = t.TempDir()
	for _, name := range []string{issuesFile, gapIssuesFile} {
		if werr := os.WriteFile(filepath.Join(dir, name), []byte("stale"), 0o644); werr != nil {
			t.Fatal(werr)
		}
	}
	var out, errOut bytes.Buffer
	err = run(&out, &errOut, []string{"-dir", dir}, f.fetch, func() {})
	return dir, out.String(), errOut.String(), err
}

// numbers reads the issue numbers of the issues.json in dir.
func numbers(t *testing.T, dir string) []int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, issuesFile))
	if err != nil {
		t.Fatal(err)
	}
	var got []struct{ Number int }
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	nums := make([]int, 0, len(got))
	for _, g := range got {
		nums = append(nums, g.Number)
	}
	return nums
}

func span(from, to int) []int {
	var s []int
	for n := from; n <= to; n++ {
		s = append(s, n)
	}
	return s
}

// assertNoOutputs checks dir holds neither output file, stale or new.
func assertNoOutputs(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{issuesFile, gapIssuesFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s present after a failed run (stat err %v)", name, err)
		}
	}
}

func TestGhArgs(t *testing.T) {
	got := ghArgs(7)
	want := []string{"api", "repos/kud360/goxsd8/issues?state=all&per_page=100&page=7"}
	if !slices.Equal(got, want) {
		t.Errorf("ghArgs(7) = %q, want %q", got, want)
	}
	if slices.Contains(got, "--paginate") {
		t.Errorf("ghArgs(7) = %q carries --paginate", got)
	}
}

// TestWalkRecovers covers the walks that must end at exit 0 with the whole
// list: each row names the pages the source serves and the numbers the feed
// must then carry.
func TestWalkRecovers(t *testing.T) {
	cases := []struct {
		name      string
		script    map[int][]answer
		want      []int
		wantCalls []int
	}{{
		name:      "honest list",
		script:    map[int][]answer{1: {rows(1, 100)}, 2: {rows(101, 100)}, 3: {rows(201, 37)}},
		want:      span(1, 237),
		wantCalls: []int{1, 2, 3, 3, 4, 4},
	}, {
		name:      "exact multiple of per_page, no phantom page",
		script:    map[int][]answer{1: {rows(1, 100)}, 2: {rows(101, 100)}},
		want:      span(1, 200),
		wantCalls: []int{1, 2, 3, 3},
	}, {
		name:      "transient short page mid-list",
		script:    map[int][]answer{1: {rows(1, 100)}, 2: {rows(101, 40), rows(101, 100)}, 3: {rows(201, 50)}},
		want:      span(1, 250),
		wantCalls: []int{1, 2, 2, 3, 3, 4, 4},
	}, {
		name:      "transient empty page mid-list",
		script:    map[int][]answer{1: {rows(1, 100)}, 2: {{raw: "[]"}, rows(101, 100)}, 3: {rows(201, 50)}},
		want:      span(1, 250),
		wantCalls: []int{1, 2, 2, 3, 3, 4, 4},
	}, {
		name:      "retry returning fewer on the last page keeps the larger",
		script:    map[int][]answer{1: {rows(1, 100)}, 2: {rows(101, 37), rows(101, 20)}},
		want:      span(1, 137),
		wantCalls: []int{1, 2, 2, 3, 3},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{script: tc.script}
			dir, stdout, _, err := feed(t, f)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := numbers(t, dir); !slices.Equal(got, tc.want) {
				t.Errorf("issues.json numbers = %d rows %v…, want %d rows", len(got), got[:min(3, len(got))], len(tc.want))
			}
			if !slices.Equal(f.calls, tc.wantCalls) {
				t.Errorf("pages requested = %v, want %v", f.calls, tc.wantCalls)
			}
			wantOut := filepath.Join(dir, issuesFile) + "\n" + filepath.Join(dir, gapIssuesFile) + "\n"
			if stdout != wantOut {
				t.Errorf("stdout = %q, want %q", stdout, wantOut)
			}
		})
	}
}

// TestWalkAborts covers every failed fetch and failed check: each must return
// an error opening with wantPrefix and leave -dir holding neither file, the
// stale ones feed pre-seeds included.
func TestWalkAborts(t *testing.T) {
	page1 := rows(1, 100)
	cases := []struct {
		name       string
		script     map[int][]answer
		wantPrefix string
	}{{
		name:       "persistently short non-last page",
		script:     map[int][]answer{1: {page1}, 2: {rows(101, 40)}, 3: {rows(141, 50)}},
		wantPrefix: "page 2 came back with 40 rows and is not the last page",
	}, {
		name:       "failed fetch mid-list",
		script:     map[int][]answer{1: {page1}, 2: {{err: errors.New("HTTP 502")}}, 3: {rows(201, 5)}},
		wantPrefix: "fetching page 2: HTTP 502",
	}, {
		name:       "failed re-request",
		script:     map[int][]answer{1: {page1}, 2: {rows(101, 5), {err: errors.New("HTTP 502")}}},
		wantPrefix: "fetching page 2: HTTP 502",
	}, {
		name:       "proxy error object instead of an array",
		script:     map[int][]answer{1: {page1}, 2: {{raw: `{"message":"Numeric-ID repository paths are not supported"}`}}},
		wantPrefix: "fetching page 2: the answer is not a JSON array",
	}, {
		name:       "null instead of an array",
		script:     map[int][]answer{1: {{raw: "null"}}},
		wantPrefix: "fetching page 1: the answer is not a JSON array",
	}, {
		name:       "page 1 served for page 2, all full",
		script:     map[int][]answer{1: {page1}, 2: {page1}, 3: {rows(201, 100)}},
		wantPrefix: "page 2 repeats #1 from page 1",
	}, {
		name:       "a page repeating its own number",
		script:     map[int][]answer{1: {{raw: `[{"number":1},{"number":1}]`}}},
		wantPrefix: "page 1 repeats #1 from page 1",
	}, {
		name:       "no rows at all",
		script:     map[int][]answer{},
		wantPrefix: "the walk found no rows at all",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, stdout, stderr, err := feed(t, &fake{script: tc.script})
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Fatalf("run err = %v, want prefix %q", err, tc.wantPrefix)
			}
			if errors.Is(err, errUsage) {
				t.Errorf("run err = %v wraps errUsage, want exit 1", err)
			}
			if stdout != "" || stderr != "" {
				t.Errorf("stdout %q, stderr %q, want both empty on a failed walk", stdout, stderr)
			}
			assertNoOutputs(t, dir)
		})
	}
}

// TestReshape pins both output shapes: pull requests dropped, labels reduced
// to {name}, title and body carried into gapissues.json only, state passed
// through unchanged (absent written as null), a null body written as "", and
// `<`, `>` and `&` written as themselves.
func TestReshape(t *testing.T) {
	page := `[
	 {"number":3,"title":"a gap","state":"open","body":"see <x> & y.go","labels":[{"name":"kind/gap","color":"c"}],"user":{}},
	 {"number":2,"title":"a pr","state":"closed","body":"pr","labels":[],"pull_request":{"url":"u"}},
	 {"number":1,"title":"no body","body":null,"labels":[],"pull_request":null}
	]`
	dir, _, _, err := feed(t, &fake{script: map[int][]answer{1: {{raw: page}}}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantIssues := `[
  {
    "number": 3,
    "state": "open",
    "labels": [
      {
        "name": "kind/gap"
      }
    ]
  },
  {
    "number": 1,
    "state": null,
    "labels": []
  }
]
`
	wantGap := `[
  {
    "number": 3,
    "title": "a gap",
    "state": "open",
    "body": "see <x> & y.go",
    "labels": [
      {
        "name": "kind/gap"
      }
    ]
  },
  {
    "number": 1,
    "title": "no body",
    "state": null,
    "body": "",
    "labels": []
  }
]
`
	for _, c := range []struct{ name, want string }{{issuesFile, wantIssues}, {gapIssuesFile, wantGap}} {
		got, err := os.ReadFile(filepath.Join(dir, c.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("%s =\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

// TestStamp pins both arms, and that the stamp counts pull requests: the
// first case's #2 is a pull request, and a stamp taken after the filter
// would read "rows 2 distinct 2 min 1 max 3 missing numbers 1".
func TestStamp(t *testing.T) {
	cases := []struct{ name, page, want string }{{
		name: "no gaps, pull request counted",
		page: `[{"number":3},{"number":2,"pull_request":{}},{"number":1}]`,
		want: "rows 3 distinct 3 min 1 max 3 no gaps\n",
	}, {
		name: "missing numbers",
		page: `[{"number":7},{"number":4},{"number":2}]`,
		want: "rows 3 distinct 3 min 2 max 7 missing numbers 3\n",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, err := feed(t, &fake{script: map[int][]answer{1: {{raw: tc.page}}}})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if stderr != tc.want {
				t.Errorf("stderr = %q, want %q", stderr, tc.want)
			}
		})
	}
}

// TestDirRequired pins that a missing -dir is a usage error that requests no
// page and writes nothing into the working directory.
func TestDirRequired(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	f := &fake{script: map[int][]answer{1: {rows(1, 3)}}}
	for _, args := range [][]string{nil, {"-dir", ""}, {"-dir", cwd, "extra"}, {"-bogus"}} {
		err := run(&bytes.Buffer{}, &bytes.Buffer{}, args, f.fetch, func() {})
		if !errors.Is(err, errUsage) {
			t.Errorf("run(%q) err = %v, want errUsage", args, err)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("pages requested = %v, want none", f.calls)
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("working directory holds %d entries after usage errors, want 0", len(entries))
	}
}

// TestUsageErrorClearsNamedDir pins that a usage error whose command line
// still named a -dir leaves neither output file there, stale ones included,
// and requests no page.
func TestUsageErrorClearsNamedDir(t *testing.T) {
	for _, tail := range []string{"extra", "-bogus"} {
		t.Run(tail, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{issuesFile, gapIssuesFile} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("stale"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			f := &fake{script: map[int][]answer{1: {rows(1, 3)}}}
			args := []string{"-dir", dir, tail}
			err := run(&bytes.Buffer{}, &bytes.Buffer{}, args, f.fetch, func() {})
			if !errors.Is(err, errUsage) {
				t.Errorf("run(%q) err = %v, want errUsage", args, err)
			}
			if len(f.calls) != 0 {
				t.Errorf("run(%q) pages requested = %v, want none", args, f.calls)
			}
			assertNoOutputs(t, dir)
		})
	}
}

// TestDirCreated pins that an absent -dir is created and written into.
func TestDirCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	f := &fake{script: map[int][]answer{1: {rows(1, 3)}}}
	if err := run(&bytes.Buffer{}, &bytes.Buffer{}, []string{"-dir", dir}, f.fetch, func() {}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := numbers(t, dir); !slices.Equal(got, span(1, 3)) {
		t.Errorf("issues.json numbers = %v, want 1..3", got)
	}
}

// TestPauseBeforeReRequest pins that the pause runs once per re-request and
// before it: the source sees page 2's second attempt only after the pause.
func TestPauseBeforeReRequest(t *testing.T) {
	f := &fake{script: map[int][]answer{1: {rows(1, 100)}, 2: {rows(101, 4)}}}
	var at []int
	pause := func() { at = append(at, len(f.calls)) }
	if err := run(&bytes.Buffer{}, &bytes.Buffer{}, []string{"-dir", t.TempDir()}, f.fetch, pause); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := []int{2, 4}; !slices.Equal(at, want) {
		t.Errorf("pauses after call counts %v, want %v (pages requested %v)", at, want, f.calls)
	}
}

// TestWriteOutputsBothOrNeither pins that a failure placing the second file
// removes the first: gapissues.json's path is a non-empty directory, so its
// rename fails after issues.json's succeeded.
func TestWriteOutputsBothOrNeither(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, gapIssuesFile)
	if err := os.MkdirAll(filepath.Join(blocker, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeOutputs(dir, []byte("[]\n"), []byte("[]\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "placing gapissues.json") {
		t.Fatalf("writeOutputs err = %v, want a failure placing gapissues.json", err)
	}
	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 || entries[0].Name() != gapIssuesFile {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir holds %v, want only the blocking %s directory", names, gapIssuesFile)
	}
}
