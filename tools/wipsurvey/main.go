// Command wipsurvey classifies every wip/issue-<N> branch on the origin
// remote as LIVE, CLAIMED, EXPIRED, RETIRED, or UNKNOWN, so a /develop
// session does not have to re-derive by hand which branches are contended
// and which are dead. Issue #399 measured seven consecutive develop
// passes each re-deriving the same three unchanged facts about the same
// dead branches; PRINCIPLES 27 turns exactly that shape of repetitive,
// deterministic work into a tool. Every other head the remote carries is
// reported too, unclassified: a survey that silently skips a namespace
// cannot be told from one that looked and found it empty.
//
// Classification needs two sources that neither shows on its own:
//   - the branch namespace, for how recently a branch's tip was pushed
//     (WORKFLOW.md's 2-hour claim TTL, PRINCIPLES 28); and
//   - the GitHub issue thread, for whether the branch's owner issue has
//     since closed or been retired with needs-replan — retirement is
//     recorded on the issue, never in the branch namespace, so a survey
//     built from git alone cannot see it.
//
// The branch set itself must come from the remote, not from local
// remote-tracking refs. `git for-each-ref refs/remotes/origin/wip/*`
// returns every branch this checkout has EVER seen, including ones GitHub
// auto-deleted at merge that nobody has since `git fetch --prune`d away —
// reporting those as LIVE or EXPIRED invents in-flight work out of stale
// local state, exactly backwards for a tool whose purpose is telling a
// session what still exists. `git ls-remote --heads origin` asks the
// remote directly instead, matching the WIP discovery index WORKFLOW.md
// and PRINCIPLES 28 both name authoritative.
//
// That one round trip asks for every head, not for a pattern, because
// WORKFLOW.md's branch scheme names TWO namespaces — wip/* and parked/* —
// and a report covering one of them cannot say which one it covered. Five
// PLAN.md stamps reported a parked/* count taken from a refspec that
// never carried parked/*, and the count was right every time, which is
// the hazard rather than the reprieve (#1627). So wip/issue-<N> refs are
// the classified rows, and two further sections report the rest: the
// parked/* branches WORKFLOW.md keeps for human triage, and every head
// outside both namespaces. Each section prints a "none" line when its
// namespace is empty, so a zero this tool measured never reads like a
// namespace nobody looked at.
//
// A head outside both namespaces holds no lease and is cleanup a human
// does, with one state worth counting: a ref AHEAD of main carries
// commits main does not have. A merged session branch whose auto-delete
// did not fire and a session branch stranding the only copy of a landing
// look identical by name, and one of the latter stood behind an unmerged
// PR for a day with a /backlog pass's whole log entry on it, in a
// namespace no survey read (#1627). Each such row prints `git rev-list
// --left-right --count <main>...<ref>`'s pair and says in words when
// ahead is nonzero: STRANDED PASS when one of those commits' subjects
// opens `meta: backlog `, AHEAD OF MAIN otherwise. A pass whose docs PR
// never merges strands its PLAN.md stamp and LOG entry, and those PRs are
// closed rather than left open, so only the ref still shows it (#1705). A
// checkout that never fetched the ref's tip cannot count either side, and
// neither can a shallow one, which counts only its visible history and
// reads every stale head as far ahead; that row prints the counts as
// undecided rather than as a number nothing measured — the posture the
// tip age already takes.
//
// A maintenance command's short-lived branch is such a head between its
// push and its squash-merge, so it prints in that section, ahead>0 — a
// /backlog pass's as STRANDED PASS — while it is landing. Nothing is owed
// on it and nothing distinguishes it there from a branch whose merge never
// happened, which is the same reading a human does on every row of the
// section.
//
// ls-remote reports only a SHA per branch, not a date, so each tip's
// commit time still comes from the local object store (`git log -1
// --format=%cI <sha>`). When that SHA has never been fetched into this
// checkout, the tool does not guess an age for it: a guessed age could
// read as EXPIRED (resumable), the dangerous direction for a lease it has
// no data behind. It reports the branch UNKNOWN instead and says to
// fetch.
//
// A tip age is only evidence about the branch that authored it. A
// wip/issue-<N> branch pushed as a bare claim, with no commits yet, points
// at whatever main pointed at when the session started, so its tip age
// measures the PREVIOUS LANDING and not the claim: a claim made minutes
// ago reports as hours old, EXPIRED, and therefore resumable, and the
// session that acts on it races a live holder (#722). Such a branch is an
// ancestor of main, which `git merge-base --is-ancestor` decides against
// the main SHA the same ls-remote round trip already fetched. Those
// branches are reported CLAIMED: the branch name establishes a claim,
// nothing establishes its age, so the branch is off-limits for retirement
// and is not resumable on age evidence alone.
//
// What dates such a claim is the issue thread's own clock, and only part
// of it: WORKFLOW.md's lease invariant dates an empty claim from the
// newest comment whose body opens with a heartbeat marker (RESUME: or
// TAKEOVER:), because those are the comments that assert a session is
// still holding the branch. A grounding, a verdict, a mason's account or
// a planning note records work already done and dates nothing — dating a
// lease from one locks the issue for a further TTL after its session died
// (#981). Supply each issue's comments in the input and this tool applies
// that rule, reporting the branch LIVE or EXPIRED with the heartbeat's
// age. Omit them — or find a thread with no heartbeat ever posted on it,
// which is how every claim starts life — and the branch stays CLAIMED for
// a reader to settle by hand. Only an aged heartbeat makes an empty claim
// EXPIRED; an absent one is not a lapsed one.
//
// A claim taken over again and again that still adds nothing to main is
// EXPIRED every cycle, and EXPIRED is the verdict /develop prefers to
// pick, so such a branch is routed back to the next session forever.
// What "adds nothing" means is the net `origin/main...tip` diff, not the
// commit count: WORKFLOW.md's lease invariant has each takeover push a
// heartbeat commit, so a branch that has been taken over even once has
// commits of its own and an empty diff, and a rule keyed on the commit
// count would fire only where that invariant was breached. From
// takeoverRemedyThreshold TAKEOVER: comments on, the EXPIRED reason of a
// branch whose diff is empty names the remedy — relabel the issue
// needs-replan — inline, the way the UNKNOWN reason names `git fetch
// origin`. Counting those cycles by eye across sessions is the work
// PRINCIPLES 27 turns into a tool (#1437). That count is zero for an
// input that carried no comments, so such a row says so rather than
// reading as a thread with no takeovers on it (#1493).
//
// Both the ancestry test and the diff test need main's commit object in
// this checkout, while the SHA they test against comes live from
// ls-remote, so a checkout that last fetched before the most recent
// landing cannot decide either. A row whose ancestry is undecided falls
// back to its tip age and its reason says so, which is what makes a LIVE
// or EXPIRED there provisional rather than settled (#806); a row whose
// diff is undecided simply names no remedy.
//
// The report ends with docs/PLAN.md's Working band, so a /develop session
// picks the first startable row from this output rather than by joining
// the band to the issue list by hand (#2399). Each `| N | #M |` row of the
// band's tier tables prints in band order with its issue's state from the
// fed list — OPEN, CLOSED, UNKNOWN when no list was fed, UNLISTED when the
// fed list lacks it — and its wip/issue-<M> branch with that branch's
// verdict, or "none" and "unclaimed" when the remote carries no such
// branch. The -plan flag names another roadmap file. A band section that
// is missing or unreadable prints an UNREAD line, never an empty band, and
// the run exits 2 after the report.
//
// Usage:
//
//	git fetch origin   # git fetch --unshallow origin, in a shallow clone
//	gh issue list --state all --json number,state,labels,comments | go tool wipsurvey
//	gh issue list --state all --json number,state,labels | go tool wipsurvey  # empty claims stay CLAIMED
//	go tool wipsurvey < /dev/null   # issue data omitted: lease-only report
//	go tool wipsurvey -plan path/to/PLAN.md < issues.json
//
// Every fed row must carry `state`: an absent one would read as OPEN, and a
// branch whose issue is closed could never report RETIRED. A fed list with
// any row lacking it is discarded whole — the report still prints, every
// branch judged lease-only and every band row UNKNOWN — and the run exits 2
// naming the first such issue (#1604).
//
// Exit status is 0 for a normal report (whatever the branches' verdicts
// turn out to be) and 2 for an operational error: git is not on PATH, the
// remote is unreachable, stdin carries malformed JSON, a fed row carries
// no `state`, or the Working band could not be read.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// claimTTL is the lease window WORKFLOW.md's branch scheme gives a pushed
// wip/ branch before another session may resume it: "tip newer than the
// claim TTL (2 hours) -> LIVE ... tip older than the TTL -> EXPIRED."
const claimTTL = 2 * time.Hour

// takeoverRemedyThreshold is how many TAKEOVER: comments the thread of a
// claim whose branch still shows no net diff against main must carry
// before its EXPIRED reason stops reporting the branch as merely takeable
// and names the remedy instead. Three: one or two cycles are explained by
// a container restart or a grounding that ran long, and WORKFLOW.md's
// Parking section already parks on a third subagent round lost to a
// restart, so both empty-diff failure modes fire on the same count
// (#1437).
const takeoverRemedyThreshold = 3

func main() {
	planPath := flag.String("plan", defaultPlanPath, "roadmap whose Working band rows the report prints")
	flag.Parse()
	if err := run(os.Stdout, os.Stderr, os.Stdin, time.Now(), *planPath); err != nil {
		fmt.Fprintf(os.Stderr, "wipsurvey: %v\n", err)
		os.Exit(2)
	}
}

// run drives the survey end to end: discover every head from the remote,
// read optional issue data from stdin, classify the wip/issue-<N>
// branches, read the Working band from the roadmap at planPath, and render
// the report ([renderReport]). A wip/ branch whose name does not fit the
// wip/issue-<N> shape is skipped with a warning to stderr rather than
// aborting the whole report — an unexpected refs/heads/wip/* branch is
// evidence worth a warning, not a reason to withhold every other row. A
// fed list missing `state` is the same kind of evidence, and stronger: run
// discards that list and hands its error to renderReport, which owns when
// it is returned. A band that cannot be read is handed over the same way.
func run(stdout, stderr io.Writer, stdin io.Reader, now time.Time, planPath string) error {
	buckets, err := remoteRefs()
	if err != nil {
		return err
	}

	var parsed []branchRef
	for _, ref := range buckets.wips {
		br, err := parseWipRef(ref.sha, ref.ref)
		if err != nil {
			// Best-effort diagnostic: a write failure here would mean stderr
			// itself is broken, which cannot change what this loop still
			// owes the caller — skip the unparseable ref and keep surveying
			// the rest.
			_, _ = fmt.Fprintf(stderr, "wipsurvey: skipping %s: %v\n", ref.ref, err)
			continue
		}
		parsed = append(parsed, br)
	}

	// A feed missing `state` is not fatal here: renderReport returns it.
	issues, feedErr := readIssues(stdin)
	if feedErr != nil && !errors.Is(feedErr, errMissingState) {
		return feedErr
	}

	rows := make([]row, 0, len(parsed))
	for _, br := range parsed {
		tip, err := gitTip(br.sha)
		if err != nil {
			return fmt.Errorf("resolving tip for %s: %w", br.branch, err)
		}
		anc, err := gitAncestry(br.sha, buckets.mainSHA)
		if err != nil {
			return fmt.Errorf("resolving ancestry for %s: %w", br.branch, err)
		}
		diff, err := gitEmptyDiff(br.sha, buckets.mainSHA)
		if err != nil {
			return fmt.Errorf("resolving net diff for %s: %w", br.branch, err)
		}
		var issue *issueState
		if state, ok := issues[br.issue]; ok {
			issue = &state
		}
		got, lease, reason := classify(br.branch, tip, anc, diff, now, issue)
		rows = append(rows, row{issue: br.issue, branch: br.branch, lease: lease, anc: anc, verdict: got, reason: reason})
	}
	sortRows(rows)

	shallow, err := gitShallow()
	if err != nil {
		return err
	}
	others := make([]otherRow, 0, len(buckets.others))
	for _, ref := range buckets.others {
		counts, err := gitAheadBehind(ref.sha, buckets.mainSHA, shallow)
		if err != nil {
			return fmt.Errorf("resolving ahead/behind for %s: %w", ref.ref, err)
		}
		others = append(others, otherRow{branch: branchName(ref.ref), counts: counts})
	}
	sortOtherRows(others)

	// A band that cannot be read is not fatal here: renderReport returns it.
	bandRows, bandErr := readBand(planPath)
	band := bandSection{path: planPath, lines: bandLines(bandRows, issues, rows), err: bandErr}

	return renderReport(stdout, rows, branchNames(buckets.parked), others, band, feedErr)
}

// renderReport writes the whole report to w — the classified table, the
// parked/* section, the section for heads in neither namespace, then the
// Working band — and returns the first write error. Absent one it renders
// every section and only then returns feedErr, the error run was handed by
// readIssues, joined with the band's read error, so a fed list missing
// `state` or a band that could not be read still prints the rest of the
// report before main exits 2 (#1604).
func renderReport(w io.Writer, rows []row, parked []string, others []otherRow, band bandSection, feedErr error) error {
	if err := renderTable(w, rows); err != nil {
		return err
	}
	if err := renderParked(w, parked); err != nil {
		return err
	}
	if err := renderOther(w, others); err != nil {
		return err
	}
	if err := renderBand(w, band); err != nil {
		return err
	}
	return errors.Join(feedErr, band.err)
}

// refSHA is one line of `git ls-remote --heads` output: a remote head's
// SHA paired with its full ref name.
type refSHA struct {
	sha string
	ref string
}

// remoteRefs asks the origin remote directly for every head it carries,
// main's included — one network call for the whole report, so neither the
// ancestry test nor the two sections for the other heads costs extra
// remote access. See the package doc comment for why this — not the local
// refs/remotes/origin/* cache — is the authoritative branch set, and why
// the request carries no pattern.
func remoteRefs() (refBuckets, error) {
	out, err := exec.Command("git", "ls-remote", "--heads", "origin").Output()
	if err != nil {
		return refBuckets{}, fmt.Errorf("running git ls-remote --heads origin: %w", err)
	}
	refs, errs := parseLsRemote(string(out))
	if len(errs) > 0 {
		return refBuckets{}, fmt.Errorf("parsing git ls-remote output: %w", errors.Join(errs...))
	}
	return partitionRefs(refs), nil
}

// refBuckets is the remote head namespace split the way the report reads
// it: the wip/* branches to classify, the parked/* branches WORKFLOW.md
// keeps for human triage, every head in neither namespace, and main's
// SHA. main is the ancestry test's right-hand side rather than a surveyed
// branch, so it is in none of the three slices; mainSHA is empty when the
// remote reports no refs/heads/main, which leaves every ancestry and every
// ahead/behind unresolved and the report dated from tip ages alone.
type refBuckets struct {
	wips    []refSHA
	parked  []refSHA
	others  []refSHA
	mainSHA string
}

// partitionRefs splits ls-remote's refs into those four. Membership is by
// ref prefix alone: a refs/heads/wip/ ref is a lease candidate even when
// its name is malformed, which run reports as a warning rather than
// silently rehoming into the section for heads outside the scheme. It is
// pure — no git or process calls — so tests exercise it directly.
func partitionRefs(refs []refSHA) refBuckets {
	var buckets refBuckets
	for _, ref := range refs {
		if ref.ref == "refs/heads/main" {
			buckets.mainSHA = ref.sha
			continue
		}
		if strings.HasPrefix(ref.ref, "refs/heads/wip/") {
			buckets.wips = append(buckets.wips, ref)
			continue
		}
		if strings.HasPrefix(ref.ref, "refs/heads/parked/") {
			buckets.parked = append(buckets.parked, ref)
			continue
		}
		buckets.others = append(buckets.others, ref)
	}
	return buckets
}

// branchName is a full ref name as the report prints it: refs/heads/ is
// what makes a ref a head, not what tells two heads apart.
func branchName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// branchNames names each ref and orders the result, which is the order the
// section renders in (STYLE D1) — ls-remote's own order is the remote's to
// change between two runs of an unchanged tool.
func branchNames(refs []refSHA) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, branchName(ref.ref))
	}
	sort.Strings(names)
	return names
}

// parseLsRemote parses `git ls-remote --heads` output, one "<sha>\t<ref>"
// pair per line. It is pure text parsing — no git or process calls — so
// tests exercise it directly against literal ls-remote output instead of
// a repository.
func parseLsRemote(output string) ([]refSHA, []error) {
	var refs []refSHA
	var errs []error
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		sha, ref, ok := strings.Cut(line, "\t")
		if !ok {
			errs = append(errs, fmt.Errorf("malformed ls-remote line %q: expected \"<sha>\\t<ref>\"", line))
			continue
		}
		refs = append(refs, refSHA{sha: sha, ref: ref})
	}
	return refs, errs
}

// branchRef is one refs/heads/wip/issue-<N> branch resolved to its issue
// number.
type branchRef struct {
	sha    string
	branch string
	issue  int
}

// wipBranchPattern matches the one-issue-one-branch shape WORKFLOW.md
// mandates: "wip/issue-<N>", N a decimal integer with no leading zero.
var wipBranchPattern = regexp.MustCompile(`^wip/issue-([1-9][0-9]*)$`)

// parseWipRef validates that ref is a refs/heads/wip/issue-<N> branch and
// extracts its branch name and issue number. It is pure — no git or
// process calls — so tests exercise it directly, including the malformed
// shapes it must reject.
func parseWipRef(sha, ref string) (branchRef, error) {
	branch, ok := strings.CutPrefix(ref, "refs/heads/")
	if !ok {
		return branchRef{}, fmt.Errorf("ref %q is not under refs/heads/", ref)
	}
	m := wipBranchPattern.FindStringSubmatch(branch)
	if m == nil {
		return branchRef{}, fmt.Errorf("branch %q does not match wip/issue-<N>", branch)
	}
	issue, err := strconv.Atoi(m[1])
	if err != nil {
		return branchRef{}, fmt.Errorf("parsing issue number from branch %q: %w", branch, err)
	}
	return branchRef{sha: sha, branch: branch, issue: issue}, nil
}

// gitTip resolves sha's committer time from the local object store. It
// returns a nil time, with no error, when git reports the object is not
// present locally (the SHA ls-remote reported has never been fetched into
// this checkout) — that is an expected state the caller turns into an
// UNKNOWN verdict, not a failure this tool aborts on. A nil time is never
// returned alongside a non-nil error: callers only need to check err to
// know which case they are in.
func gitTip(sha string) (*time.Time, error) {
	out, err := exec.Command("git", "log", "-1", "--format=%cI", sha, "--").Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("running git log -1 for %s: %w", sha, err)
	}
	ts := strings.TrimSpace(string(out))
	tip, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return nil, fmt.Errorf("parsing committer date %q for %s: %w", ts, sha, err)
	}
	return &tip, nil
}

// ancestry is what this checkout could establish about a branch tip's
// position relative to main: whether the branch carries commits of its
// own. Its zero value is ancestryUnresolved, so a caller with no answer
// carries none.
type ancestry int

const (
	// ancestryUnresolved means git could not decide — the objects are not
	// in this checkout, or the remote reported no main. Callers fall back
	// to the tip age rather than inventing a second unknown verdict, and
	// say in the reason that they did.
	ancestryUnresolved ancestry = iota
	// ancestryOwnCommits means the branch is not an ancestor of main: its
	// tip is a commit the branch itself authored, so the tip age is the
	// claim's age.
	ancestryOwnCommits
	// ancestryNoCommits means the branch is an ancestor of main: it has
	// pushed no commits, its tip is a landing's, and its tip age says
	// nothing about the claim.
	ancestryNoCommits
)

// gitAncestry asks whether sha is an ancestor of mainSHA — whether the
// branch at sha has any commits of its own. It returns ancestryUnresolved,
// with no error, when the question cannot be answered here: mainSHA is
// empty, or git cannot resolve one of the two objects (an unfetched tip,
// which exits 128). That mirrors gitTip's posture on an unfetched SHA —
// report what is not known rather than guess, since guessing "has its own
// commits" restores exactly the borrowed-age EXPIRED this distinction
// exists to prevent.
func gitAncestry(sha, mainSHA string) (ancestry, error) {
	if mainSHA == "" {
		return ancestryUnresolved, nil
	}
	err := exec.Command("git", "merge-base", "--is-ancestor", sha, mainSHA).Run()
	if err == nil {
		return ancestryFromExit(0), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ancestryFromExit(exitErr.ExitCode()), nil
	}
	return ancestryUnresolved, fmt.Errorf("running git merge-base --is-ancestor %s %s: %w", sha, mainSHA, err)
}

// ancestryFromExit maps `git merge-base --is-ancestor`'s exit status: 0 is
// "is an ancestor", 1 is "is not", and every other status (128 for an
// object this checkout does not have) is git declining to answer. It is
// pure — no git or process calls — so tests exercise each status directly.
func ancestryFromExit(code int) ancestry {
	switch code {
	case 0:
		return ancestryNoCommits
	case 1:
		return ancestryOwnCommits
	}
	return ancestryUnresolved
}

// netDiff is what this checkout could establish about what a branch adds
// to main: whether the three-dot `origin/main...tip` diff is empty. It is
// a separate question from ancestry, and the gap between them is where
// #1437 lives — a branch of nothing but heartbeat commits and
// merge-forwards has commits of its own (ancestryOwnCommits) and an empty
// diff (diffEmpty). Its zero value is diffUnresolved, so a caller with no
// answer carries none.
type netDiff int

const (
	// diffUnresolved means git could not decide — the objects are not in
	// this checkout, or the remote reported no main. Callers name no
	// remedy on it: a claim is accused of having produced nothing only on
	// evidence, never on a guess (#806).
	diffUnresolved netDiff = iota
	// diffNonEmpty means the branch changes something against main,
	// whatever its commit count.
	diffNonEmpty
	// diffEmpty means the branch's whole history nets out to no change
	// against main: an unstarted claim, or one whose every commit is a
	// heartbeat or a merge-forward.
	diffEmpty
)

// gitEmptyDiff asks whether the branch at sha adds anything to mainSHA —
// whether `git diff --quiet <mainSHA>...<sha>` finds a net change. It
// returns diffUnresolved, with no error, when the question cannot be
// answered here: mainSHA is empty, or git cannot resolve one of the two
// objects (an unfetched tip, which exits 128). That mirrors gitAncestry's
// posture — report what is not known rather than guess, since guessing
// "empty" would have the survey tell a session to retire an issue whose
// branch may hold real work.
func gitEmptyDiff(sha, mainSHA string) (netDiff, error) {
	if mainSHA == "" {
		return diffUnresolved, nil
	}
	err := exec.Command("git", "diff", "--quiet", mainSHA+"..."+sha).Run()
	if err == nil {
		return netDiffFromExit(0), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return netDiffFromExit(exitErr.ExitCode()), nil
	}
	return diffUnresolved, fmt.Errorf("running git diff --quiet %s...%s: %w", mainSHA, sha, err)
}

// netDiffFromExit maps `git diff --quiet`'s exit status: 0 is "no
// differences", 1 is "differences found", and every other status (128 for
// an object this checkout does not have) is git declining to answer. It
// is pure — no git or process calls — so tests exercise each status
// directly.
func netDiffFromExit(code int) netDiff {
	switch code {
	case 0:
		return diffEmpty
	case 1:
		return diffNonEmpty
	}
	return diffUnresolved
}

// aheadBehind is how a head outside the surveyed namespaces stands
// against main. ahead is the count that matters: a ref main cannot reach
// carries commits main does not have, which is the one state on such a
// ref owed a human's attention rather than a deletion. backlogSubject is
// the subject of the newest of those commits a /backlog pass wrote, empty
// when none is: that ref strands a PLAN.md stamp and a LOG entry (#1705).
type aheadBehind struct {
	ahead          int
	behind         int
	backlogSubject string
}

// backlogPassPrefix opens the subject of every /backlog pass's commit. The
// trailing space is load-bearing: `meta: recover stranded 2026-09-21
// backlog LOG entry` is a recovery, not a pass.
const backlogPassPrefix = "meta: backlog "

// backlogSubject returns the first of subjects that a /backlog pass wrote,
// or "" when none is. Given `git log --format=%s <main>..<ref>`'s lines,
// newest first, that is the newest pass the ref strands. It is pure, so
// tests exercise it directly against literal subjects.
func backlogSubject(subjects []string) string {
	for _, s := range subjects {
		if strings.HasPrefix(s, backlogPassPrefix) {
			return s
		}
	}
	return ""
}

// gitAheadBehind counts how far the ref at sha stands from mainSHA in each
// direction, and names the /backlog pass among the commits ahead. It
// returns nil, with no error, when the question cannot be answered here:
// mainSHA is empty, the checkout is shallow, or git cannot resolve one of
// the two objects (a tip this checkout never fetched, which exits 128).
// That mirrors gitTip's and gitAncestry's posture, for this tool's usual
// reason — a zero printed where nothing was counted is exactly the reading
// this section exists to make impossible.
//
// A shallow checkout does not fail these range queries; it answers them
// from the visible history, so a stale head counts hundreds of commits
// ahead and its subjects include every old /backlog pass main's truncated
// history does not reach (docs/ROUTINES.md, #1705). Its counts are therefore undecided,
// never taken.
func gitAheadBehind(sha, mainSHA string, shallow bool) (*aheadBehind, error) {
	if mainSHA == "" || shallow {
		return nil, nil
	}
	out, err := exec.Command("git", "rev-list", "--left-right", "--count", mainSHA+"..."+sha).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("running git rev-list --left-right --count %s...%s: %w", mainSHA, sha, err)
	}
	counts, err := parseAheadBehind(string(out))
	if err != nil {
		return nil, fmt.Errorf("counting %s against main: %w", sha, err)
	}
	if counts.ahead == 0 {
		return &counts, nil
	}
	out, err = exec.Command("git", "log", "--format=%s", mainSHA+".."+sha, "--").Output()
	if errors.As(err, &exitErr) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("running git log --format=%%s %s..%s: %w", mainSHA, sha, err)
	}
	counts.backlogSubject = backlogSubject(strings.Split(string(out), "\n"))
	return &counts, nil
}

// gitShallow asks whether this checkout is shallow, which gitAheadBehind
// must know before it takes any count.
func gitShallow() (bool, error) {
	out, err := exec.Command("git", "rev-parse", "--is-shallow-repository").Output()
	if err != nil {
		return false, fmt.Errorf("running git rev-parse --is-shallow-repository: %w", err)
	}
	return parseShallow(string(out))
}

// parseShallow parses `git rev-parse --is-shallow-repository`'s single
// line. Anything but "true" or "false" is an error rather than a guess:
// git before 2.15 echoes the unknown flag back, and reading that as "not
// shallow" would take every count this guard exists to withhold.
func parseShallow(output string) (bool, error) {
	switch strings.TrimSpace(output) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("malformed rev-parse --is-shallow-repository output %q: expected \"true\" or \"false\"", output)
}

// parseAheadBehind parses `git rev-list --left-right --count
// <mainSHA>...<sha>`'s single line, "<behind>\t<ahead>": the LEFT count is
// what main reaches and the ref does not, so it is the ref's behind, and
// the right count is the ref's ahead. It is pure text parsing — no git or
// process calls — so tests exercise it directly against literal rev-list
// output.
func parseAheadBehind(output string) (aheadBehind, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return aheadBehind{}, fmt.Errorf("malformed rev-list --left-right --count output %q: expected \"<behind>\\t<ahead>\"", output)
	}
	behind, err := strconv.Atoi(fields[0])
	if err != nil {
		return aheadBehind{}, fmt.Errorf("parsing behind count from %q: %w", output, err)
	}
	ahead, err := strconv.Atoi(fields[1])
	if err != nil {
		return aheadBehind{}, fmt.Errorf("parsing ahead count from %q: %w", output, err)
	}
	return aheadBehind{ahead: ahead, behind: behind}, nil
}

// ghLabel is one label object in `gh issue list --json labels`'s shape.
type ghLabel struct {
	Name string `json:"name"`
}

// ghComment is one element of `gh issue list --json comments`'s comments
// array — the two fields wipsurvey reads from it.
type ghComment struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// ghIssue is one element of `gh issue list --state all --json
// number,state,labels,comments`'s JSON array — the exact fields wipsurvey
// reads from it. Comments is a pointer because absent and empty must not
// collapse: no `comments` key means the caller supplied no comment data
// and an empty claim cannot be dated at all, while `"comments": []` is the
// positive statement that the thread carries no heartbeat, which dates the
// claim as takeable. State is a pointer for the same reason: no `state` key
// means the reshape dropped the field, which readIssues refuses rather than
// reading as OPEN.
type ghIssue struct {
	Number   int          `json:"number"`
	State    *string      `json:"state"`
	Labels   []ghLabel    `json:"labels"`
	Comments *[]ghComment `json:"comments"`
}

// issueState is the subset of a GitHub issue's state classify needs:
// whether it is closed, whether it carries the needs-replan label
// WORKFLOW.md uses to retire an abandoned attempt in place, and what the
// thread says about an empty claim's lease.
type issueState struct {
	number      int
	closed      bool
	needsReplan bool
	// commentsRead records that the input carried this issue's comments,
	// so a nil heartbeat means "the thread has none" rather than "nobody
	// looked".
	commentsRead bool
	// heartbeat is the creation time of the newest comment asserting a
	// session still holds the branch, or nil when the thread carries none.
	heartbeat *time.Time
	// takeovers is how many TAKEOVER: comments the thread carries, which
	// remedyClause reads against takeoverRemedyThreshold. It is zero
	// when no comments were supplied for this issue, the same as for a
	// thread that carries none — commentsRead is what tells those apart.
	takeovers int
}

// heartbeatPrefixes are the comment markers WORKFLOW.md's lease invariant
// gives a session for asserting it is still holding an empty claim.
// Matching is against the body's first non-whitespace characters and is
// case-sensitive, because every marker in this corpus is shouted and a
// lease is not a thing to settle on a fuzzy match.
var heartbeatPrefixes = []string{"RESUME:", takeoverPrefix}

// takeoverPrefix is the marker a session posts when it takes a claim over
// from a previous holder. It is named apart from its fellow prefix
// because the repetition count keys on it alone: what that count measures
// is how many times the claim has changed hands, and a RESUME: is one
// session picking its own claim back up.
const takeoverPrefix = "TAKEOVER:"

// heartbeatMarkers names the prefixes in report text, from the one list
// that defines them.
var heartbeatMarkers = strings.Join(heartbeatPrefixes, "/")

// isHeartbeat reports whether a comment body opens with a heartbeat
// marker. It is pure text matching, so tests exercise it directly against
// the comment shapes the threads actually carry.
func isHeartbeat(body string) bool {
	for _, prefix := range heartbeatPrefixes {
		if hasMarker(body, prefix) {
			return true
		}
	}
	return false
}

// hasMarker reports whether a comment body opens with marker, on
// heartbeatPrefixes' terms: leading whitespace is skipped, and the match
// is otherwise exact.
func hasMarker(body, marker string) bool {
	return strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), marker)
}

// countTakeovers counts the thread's TAKEOVER: comments — how many times
// a session has claimed this branch from a previous holder. RESUME:
// comments do not count: they are the holder returning to its own claim,
// which is not a hand-off.
func countTakeovers(comments []ghComment) int {
	n := 0
	for _, c := range comments {
		if hasMarker(c.Body, takeoverPrefix) {
			n++
		}
	}
	return n
}

// newestHeartbeat returns the creation time of the newest heartbeat
// comment, or nil when the thread carries none. It compares timestamps
// rather than trusting input order, so a caller that concatenated pages
// out of order still gets the newest.
func newestHeartbeat(comments []ghComment) *time.Time {
	var newest *time.Time
	for _, c := range comments {
		if !isHeartbeat(c.Body) {
			continue
		}
		if newest == nil || c.CreatedAt.After(*newest) {
			at := c.CreatedAt
			newest = &at
		}
	}
	return newest
}

// readIssues parses `gh issue list --state all --json
// number,state,labels,comments`'s JSON array from r into a lookup by issue
// number. Empty input — no stdin redirected, or an explicit `< /dev/null`
// — is not an error: it means the caller has no issue data to offer, and
// readIssues returns a nil map so every branch is judged lease-only and
// every band row's state is UNKNOWN ([bandStatus]). A fed list, even an
// empty one, returns a non-nil map. The comments field is likewise
// optional, per issue: without it that issue's branch can still be retired
// or dated from its own tip, only an empty claim goes undated.
//
// The state field is not optional: a list with any row lacking it returns
// a nil map and an error wrapping errMissingState ([missingState]); run
// judges every branch lease-only and hands that error to [renderReport].
func readIssues(r io.Reader) (map[int]issueState, error) {
	dec := json.NewDecoder(r)
	var raw []ghIssue
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing issue JSON from stdin: %w", err)
	}
	if err := missingState(raw); err != nil {
		return nil, err
	}

	issues := make(map[int]issueState, len(raw))
	for _, gi := range raw {
		state := issueState{
			number: gi.Number,
			closed: strings.EqualFold(*gi.State, "CLOSED"),
		}
		for _, l := range gi.Labels {
			if l.Name == "needs-replan" {
				state.needsReplan = true
			}
		}
		if gi.Comments != nil {
			state.commentsRead = true
			state.heartbeat = newestHeartbeat(*gi.Comments)
			state.takeovers = countTakeovers(*gi.Comments)
		}
		issues[gi.Number] = state
	}
	return issues, nil
}

// errMissingState is what every [missingState] error wraps, so run can tell
// a feed it must discard but can still report around from one it cannot
// read at all.
var errMissingState = errors.New(`every branch was judged lease-only; reshape the input per docs/ROUTINES.md's "Survey input"`)

// missingState returns an error naming the first fed issue, in fed order,
// whose row carries no `state` key, or nil when every row carries one — an
// empty list included, since it has no row to lack the key. It words a feed
// where no row has the key apart from one where only some do: the first is a
// reshape that never asked for the field, the second a join or merge that
// lost it for part of the list.
func missingState(raw []ghIssue) error {
	missing := 0
	first := 0
	for _, gi := range raw {
		if gi.State != nil {
			continue
		}
		if missing == 0 {
			first = gi.Number
		}
		missing++
	}
	if missing == 0 {
		return nil
	}
	if missing == len(raw) {
		return fmt.Errorf("no row of the fed issue list carries a \"state\" field (first: #%d),"+
			" so no issue can be told OPEN from CLOSED — %w", first, errMissingState)
	}
	return fmt.Errorf("issue #%d is the first of %d of %d fed rows carrying no \"state\" field,"+
		" so it cannot be told OPEN from CLOSED — %w", first, missing, len(raw), errMissingState)
}

// verdict is one of the outcomes classify produces for a wip/issue-<N>
// branch.
type verdict string

const (
	live    verdict = "LIVE"
	claimed verdict = "CLAIMED"
	expired verdict = "EXPIRED"
	retired verdict = "RETIRED"
	unknown verdict = "UNKNOWN"
)

// classify decides one branch's verdict, the age of the evidence dating
// its lease, and a short reason, from its remote tip time, its ancestry
// against main, whether its net diff against main is empty, and,
// optionally, the GitHub issue it implements. It is pure — no git or
// process calls — so tests exercise it directly without a repository or
// network access; run computes anc and diff and hands them in.
//
// The lease age is the tip's age for a branch with commits of its own and
// the newest heartbeat comment's age for one with none. It is nil where
// nothing in this input dates the lease: an unfetched tip, an empty claim
// no heartbeat dates, or a retired branch with no commits of its own —
// retirement does not turn a landing's tip into the branch's own lease
// (#809). It is what the report's LEASE AGE column prints, which renders
// the nil cases as "main's" or "unknown" rather than as a duration.
//
// tip is nil when the branch's SHA has never been fetched into this
// checkout's object store; classify then reports unknown rather than
// guessing an age, because a guessed age could read as expired
// (resumable) — the dangerous direction for a lease with no data behind
// it. issue is nil when no issue data was supplied (stdin was empty), when
// the fed list was discarded for a row missing `state` ([missingState]), or
// when the branch's issue number was not present in it; the reason then notes
// the verdict is lease-only.
//
// The order of the four questions is itself the contract:
//
//   - Retirement first: a closed or needs-replan issue retires its branch
//     outright, with or without a fetched tip and with or without commits
//     of its own, per WORKFLOW.md's "abandoned attempts are retired in
//     place ... never resumed." The verdict ignores the ancestry, but the
//     lease age reads it: a retired branch that never committed has no
//     more age of its own than a live one does.
//   - Then an unfetched tip, which is unknown.
//   - Then anc == ancestryNoCommits, which classifyEmptyClaim dates from
//     the issue thread: the tip belongs to a landing, not to this branch,
//     so no age question may be asked of it at all.
//   - Only then the tip age, against the claim TTL. ancestryUnresolved
//     lands here too — the age is the best evidence left, and it is the
//     evidence this tool reported before it could ask about ancestry — but
//     its reason says the ancestry was undecided, so the verdict reads as
//     provisional rather than settled (#806).
//
// Both paths that return EXPIRED end their reason with remedyClause,
// which is where #1437's repeated-takeover remedy is named. The tip-age
// path is the one a branch of heartbeat commits and merge-forwards takes,
// so leaving the clause off it would leave the rule silent on exactly the
// branches WORKFLOW.md's lease invariant produces.
//
// The tip-age EXPIRED reason also carries missingCommentsClause, which
// names the input remedyClause is reading a zero from when the input
// carried no comments (#1493). classifyEmptyClaim needs no such clause:
// it answers CLAIMED, naming the same missing input, before it can reach
// its own EXPIRED.
func classify(branch string, tip *time.Time, anc ancestry, diff netDiff, now time.Time, issue *issueState) (verdict, *time.Duration, string) {
	if issue != nil && issue.closed {
		return retired, retiredLease(tip, anc, now), fmt.Sprintf("%s: issue #%d is closed", branch, issue.number)
	}
	if issue != nil && issue.needsReplan {
		return retired, retiredLease(tip, anc, now), fmt.Sprintf("%s: issue #%d is labelled needs-replan", branch, issue.number)
	}
	if tip == nil {
		return unknown, nil, fmt.Sprintf("%s: tip not fetched -- run `git fetch origin`", branch)
	}

	leaseNote := ""
	if issue == nil {
		leaseNote = "; no issue data for this branch, lease-only"
	}
	if anc == ancestryNoCommits {
		return classifyEmptyClaim(branch, diff, now, issue, leaseNote)
	}
	ancestryNote := ""
	if anc == ancestryUnresolved {
		ancestryNote = "; ancestry against main undecided, so this age may be main's rather than the claim's -- run `git fetch origin`"
	}
	age := now.Sub(*tip)
	if age <= claimTTL {
		return live, &age, fmt.Sprintf("%s: tip pushed %s ago, within the %s claim TTL%s%s", branch, formatAge(age), formatAge(claimTTL), ancestryNote, leaseNote)
	}
	return expired, &age, fmt.Sprintf("%s: tip pushed %s ago, past the %s claim TTL%s%s%s%s", branch, formatAge(age), formatAge(claimTTL), ancestryNote, leaseNote, missingCommentsClause(anc, diff, issue), remedyClause(anc, diff, issue))
}

// remedyClause is the sentence an EXPIRED reason carries when the claim
// has already been taken over takeoverRemedyThreshold times and the
// branch still nets out to nothing: that take has been made that many
// times and produced nothing, so the reason names the remedy — relabel
// the issue needs-replan — beside the verdict (#1437). It names the live
// count rather than the threshold, so it reads the same at cycle 3 and at
// cycle 10 instead of only firing in hindsight.
//
// It is the empty string on every other row, including the two kinds of
// row where the question is open rather than answered no: a diff git
// could not decide, and an ancestry git could not decide, whose tip age
// is already provisional (#806). A remedy is named on evidence or not at
// all.
func remedyClause(anc ancestry, diff netDiff, issue *issueState) string {
	if anc == ancestryUnresolved || diff != diffEmpty {
		return ""
	}
	if issue == nil || issue.takeovers < takeoverRemedyThreshold {
		return ""
	}
	return fmt.Sprintf(", already %s comment #%d with no diff ever produced -- relabel the issue needs-replan instead of resuming", takeoverPrefix, issue.takeovers)
}

// missingCommentsClause is the sentence a tip-age EXPIRED reason carries
// when its branch nets out to nothing and the input carried no comments
// for the issue: remedyClause's takeover count is then zero for want of
// input, and a thread that was never fetched would otherwise print
// exactly what a thread carrying no takeovers prints (#1493). It names
// what is missing and what to supply, as classifyEmptyClaim's
// comment-less CLAIMED reason already does in the same position.
//
// Its first condition is remedyClause's: a row where the diff or the
// ancestry is undecided declines a remedy on the evidence, and declines
// this note for the same reason — the reader is not owed a note about an
// input whose answer would change nothing there. The two clauses are
// mutually exclusive, since a count that reaches the threshold can only
// have been read from comments.
func missingCommentsClause(anc ancestry, diff netDiff, issue *issueState) string {
	if anc == ancestryUnresolved || diff != diffEmpty {
		return ""
	}
	if issue != nil && issue.commentsRead {
		return ""
	}
	return fmt.Sprintf("; no comments supplied for this issue, so its %s count is zero because nothing was counted, not because nothing happened -- supply them before taking the claim over", takeoverPrefix)
}

// classifyEmptyClaim dates a branch that has pushed no commits of its own.
// Its tip is the landing it branched from, so the only clock left is
// WORKFLOW.md's: the newest thread comment opening with a heartbeat
// marker, against the same claim TTL. A thread whose newest heartbeat is
// past the TTL leaves the claim takeable, which is EXPIRED, the verdict
// /develop reads as resumable. No tip age reaches any of that arithmetic,
// so #722's hazard stays closed.
//
// A lapsed heartbeat and an absent one are opposite evidence, not the same
// evidence. /develop pushes every claim as a bare branch and posts nothing
// heartbeat-shaped until its first checkpoint, so a thread carrying no
// heartbeat at all is the shape of a claim made seconds ago just as much
// as of one abandoned — EXPIRED there hands a session the branch someone
// else is grounding right now (#981). Only an aged heartbeat demotes a
// claim. With none ever posted, and equally without this issue's comments
// in the input, the branch stays CLAIMED, the verdict that asks a reader
// to settle it from the thread rather than on age.
//
// Its EXPIRED reason ends with remedyClause, the same one classify's
// tip-age EXPIRED carries: this path is the zero-commit end of the
// empty-diff population, not a population of its own.
func classifyEmptyClaim(branch string, diff netDiff, now time.Time, issue *issueState, leaseNote string) (verdict, *time.Duration, string) {
	if issue == nil || !issue.commentsRead {
		return claimed, nil, fmt.Sprintf("%s: no commits of its own; tip age is main's, not the claim's -- do not retire on age; supply this issue's comments to date the lease, or settle it from the issue thread%s", branch, leaseNote)
	}
	if issue.heartbeat == nil {
		return claimed, nil, fmt.Sprintf("%s: no commits of its own and no %s comment ever posted -- a claim is born undated, so this is not a lapsed lease; settle it from the issue thread", branch, heartbeatMarkers)
	}
	age := now.Sub(*issue.heartbeat)
	if age <= claimTTL {
		return live, &age, fmt.Sprintf("%s: no commits of its own; lease dated by its newest %s comment, posted %s ago, within the %s claim TTL", branch, heartbeatMarkers, formatAge(age), formatAge(claimTTL))
	}
	return expired, &age, fmt.Sprintf("%s: no commits of its own; newest %s comment posted %s ago, past the %s claim TTL -- takeable%s", branch, heartbeatMarkers, formatAge(age), formatAge(claimTTL), remedyClause(ancestryNoCommits, diff, issue))
}

// retiredLease is a retired branch's lease age: its tip's, or nil when the
// branch pushed no commits of its own and that tip is therefore the
// landing it branched from. Retirement changes the verdict, not whose
// clock the tip is, and the LEASE AGE column is read (#809).
func retiredLease(tip *time.Time, anc ancestry, now time.Time) *time.Duration {
	if anc == ancestryNoCommits {
		return nil
	}
	return tipAge(tip, now)
}

// tipAge is tip's age at now, or nil when the tip was never fetched.
func tipAge(tip *time.Time, now time.Time) *time.Duration {
	if tip == nil {
		return nil
	}
	age := now.Sub(*tip)
	return &age
}

// formatAge renders a duration the way the report shows one: rounded to
// the minute, so two runs a few seconds apart render identically (STYLE
// D1) instead of drifting by the wall-clock second between them.
func formatAge(d time.Duration) string {
	return d.Round(time.Minute).String()
}

// row is one line of the survey's output table. lease is the age classify
// dated the verdict from, nil where nothing dated it; anc is what git
// could establish about the branch against main, which is what tells the
// two kinds of nothing apart when the column is rendered.
type row struct {
	issue   int
	branch  string
	lease   *time.Duration
	anc     ancestry
	verdict verdict
	reason  string
}

// sortRows orders rows by issue number, the report's deterministic order
// (STYLE D1); ties (which do not occur for well-formed input, since a
// branch name is unique per issue) break on branch name so the order
// never depends on git's or a map's iteration order.
func sortRows(rows []row) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].issue != rows[j].issue {
			return rows[i].issue < rows[j].issue
		}
		return rows[i].branch < rows[j].branch
	})
}

// renderTable writes rows as an aligned table: ISSUE, BRANCH, LEASE AGE,
// VERDICT, REASON. It does not sort — callers order rows first (run calls
// sortRows) — so the same rows always render the same text (STYLE D1).
//
// The column is the LEASE's age, not the tip's, because for a branch with
// no commits of its own those are different clocks and only the first
// decided the verdict.
func renderTable(w io.Writer, rows []row) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ISSUE\tBRANCH\tLEASE AGE\tVERDICT\tREASON"); err != nil {
		return fmt.Errorf("writing table header: %w", err)
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.issue, r.branch, leaseAgeCell(r), r.verdict, r.reason); err != nil {
			return fmt.Errorf("writing table row for issue #%d: %w", r.issue, err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing report table: %w", err)
	}
	return nil
}

// leaseAgeCell renders one row's LEASE AGE. A row nothing dated whose
// branch has no commits of its own reads "main's" rather than a duration:
// the number would be a real duration attached to the wrong branch, and a
// column scanned faster than it is read is the whole reason that age got
// acted on (#722). That is every CLAIMED row and the RETIRED rows that
// never committed (#809). Any other row with nothing dating it reads
// "unknown", and its REASON says which kind of nothing.
//
// A non-nil lease always prints: classify hands one back only from the
// branch's own clock — its tip, or the heartbeat that dates an empty
// claim — never from a borrowed tip.
func leaseAgeCell(r row) string {
	if r.lease != nil {
		return formatAge(*r.lease)
	}
	if r.anc == ancestryNoCommits {
		return "main's"
	}
	return "unknown"
}

// renderParked writes the parked/* section: one row per branch, each
// saying what WORKFLOW.md's scheme makes it — work kept for a human, not
// a claim on an issue — or a line saying the namespace was surveyed and
// is empty. The empty line is the point of the section rather than a
// courtesy: an omitted section and a measured zero read identically, and
// five PLAN.md stamps reported a parked/* count from a tool whose refspec
// could not produce one (#1627).
//
// It does not sort — callers order branches first (run calls branchNames)
// — so the same branches always render the same text (STYLE D1).
func renderParked(w io.Writer, branches []string) error {
	if _, err := fmt.Fprint(w, "\nPARKED BRANCHES -- unattributable work kept for human triage; no lease, no claim\n"); err != nil {
		return fmt.Errorf("writing parked section heading: %w", err)
	}
	if len(branches) == 0 {
		if _, err := fmt.Fprintln(w, "none -- refs/heads/parked/* was surveyed and the remote carries no such ref"); err != nil {
			return fmt.Errorf("writing empty parked section: %w", err)
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "BRANCH\tNOTE"); err != nil {
		return fmt.Errorf("writing parked table header: %w", err)
	}
	for _, branch := range branches {
		if _, err := fmt.Fprintf(tw, "%s\tno lease; not a claim -- triage by hand\n", branch); err != nil {
			return fmt.Errorf("writing parked table row for %s: %w", branch, err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing parked table: %w", err)
	}
	return nil
}

// otherRow is one line of the section for heads outside both namespaces.
// counts is nil where this checkout could not count either direction, so
// a number never stands in for a count nothing took.
type otherRow struct {
	branch string
	counts *aheadBehind
}

// sortOtherRows orders the section by branch name, the only key such a row
// has (STYLE D1): these refs carry no issue number and ls-remote's order
// is the remote's to change.
func sortOtherRows(rows []otherRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].branch < rows[j].branch })
}

// otherNote is one such row's NOTE cell, and the ahead>0 case is the one
// the section exists for, so it is the only one that shouts — naming a
// stranded /backlog pass apart from the rest, since that ref holds a write-up
// no later pass re-derives (#1705). An uncounted ref is reported as
// undecided and never as a zero: "ahead=0" is the whole claim that nothing
// is at risk on the ref, and this tool does not make that claim on a count
// it could not take.
func otherNote(r otherRow) string {
	if r.counts == nil {
		return "ahead/behind undecided -- run `git fetch origin`, or `git fetch --unshallow origin` in a shallow clone"
	}
	if r.counts.backlogSubject != "" {
		return "STRANDED PASS -- `" + r.counts.backlogSubject + "` never reached main; recover its PLAN.md and LOG write-up before deleting"
	}
	if r.counts.ahead > 0 {
		return "AHEAD OF MAIN -- carries commits main does not have; triage before deleting"
	}
	return "nothing main does not already have"
}

// renderOther writes the section for heads outside both namespaces:
// BRANCH, AHEAD, BEHIND, NOTE per row, or a line saying the remote carries
// none. A row whose counts are undecided prints "?" in both columns.
//
// It does not sort — callers order rows first (run calls sortOtherRows) —
// so the same rows always render the same text (STYLE D1).
func renderOther(w io.Writer, rows []otherRow) error {
	if _, err := fmt.Fprint(w, "\nOTHER BRANCHES -- outside wip/* and parked/*; no lease; human-triage cleanup\n"); err != nil {
		return fmt.Errorf("writing other-branch section heading: %w", err)
	}
	if len(rows) == 0 {
		if _, err := fmt.Fprintln(w, "none -- every head on the remote is under wip/*, parked/*, or main"); err != nil {
			return fmt.Errorf("writing empty other-branch section: %w", err)
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "BRANCH\tAHEAD\tBEHIND\tNOTE"); err != nil {
		return fmt.Errorf("writing other-branch table header: %w", err)
	}
	for _, r := range rows {
		ahead, behind := "?", "?"
		if r.counts != nil {
			ahead, behind = strconv.Itoa(r.counts.ahead), strconv.Itoa(r.counts.behind)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.branch, ahead, behind, otherNote(r)); err != nil {
			return fmt.Errorf("writing other-branch table row for %s: %w", r.branch, err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing other-branch table: %w", err)
	}
	return nil
}
