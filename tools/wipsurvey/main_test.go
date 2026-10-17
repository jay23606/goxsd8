package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

// TestClassify exercises the pure classification function directly: no
// git, no process calls, no filesystem. Each case mutates exactly one
// input away from a baseline that would otherwise land on a different
// verdict, so a broken condition in classify flips the case it covers.
func TestClassify(t *testing.T) {
	tip := func(agoMinutes int) *time.Time {
		t := fixedNow.Add(-time.Duration(agoMinutes) * time.Minute)
		return &t
	}
	// Both clocks are read the same way — minutes before fixedNow — and the
	// alias keeps each case saying which clock it is setting.
	heartbeat := tip

	// A case that omits anc gets ancestryUnresolved — git declined to
	// answer — which reaches the same verdict it did before this tool could
	// ask about ancestry at all, with a reason that says the ancestry was
	// undecided (#806). A case that omits diff likewise gets
	// diffUnresolved, which names no remedy.
	cases := []struct {
		name        string
		branch      string
		tip         *time.Time
		anc         ancestry
		diff        netDiff
		issue       *issueState
		wantVerdict verdict
		wantReason  string // substring the reason must contain
		// wantCell is the LEASE AGE cell the row classify returns must
		// render to. Empty means the case does not speak to the column —
		// no real cell is ever empty, so absent and asserted stay
		// distinguishable without a second field.
		wantCell string
	}{
		{
			name:        "fresh tip is live",
			branch:      "wip/issue-1",
			tip:         tip(5),
			issue:       nil,
			wantVerdict: live,
			wantReason:  "within",
		},
		{
			name:        "tip exactly at the TTL boundary is live",
			branch:      "wip/issue-2",
			tip:         tip(120), // exactly claimTTL (2h)
			issue:       nil,
			wantVerdict: live,
			wantReason:  "within",
		},
		{
			name:        "tip one minute past the TTL boundary is expired",
			branch:      "wip/issue-3",
			tip:         tip(121),
			issue:       nil,
			wantVerdict: expired,
			wantReason:  "past",
		},
		{
			name:        "old tip is expired",
			branch:      "wip/issue-4",
			tip:         tip(600),
			issue:       nil,
			wantVerdict: expired,
			wantReason:  "past",
		},
		{
			name:        "closed issue retires a fresh-tip branch",
			branch:      "wip/issue-5",
			tip:         tip(1),
			issue:       &issueState{number: 5, closed: true},
			wantVerdict: retired,
			wantReason:  "closed",
			// The branch may have commits of its own, so its tip's age is
			// its own to print.
			wantCell: "1m0s",
		},
		{
			name:        "closed issue retires even with no fetched tip",
			branch:      "wip/issue-6",
			tip:         nil,
			issue:       &issueState{number: 6, closed: true},
			wantVerdict: retired,
			wantReason:  "closed",
			wantCell:    "unknown",
		},
		{
			name:        "needs-replan label retires regardless of tip age",
			branch:      "wip/issue-7",
			tip:         tip(1),
			issue:       &issueState{number: 7, needsReplan: true},
			wantVerdict: retired,
			wantReason:  "needs-replan",
		},
		{
			name:        "open issue with fresh tip is live, not retired",
			branch:      "wip/issue-8",
			tip:         tip(1),
			issue:       &issueState{number: 8, closed: false, needsReplan: false},
			wantVerdict: live,
			wantReason:  "within",
		},
		{
			name:        "missing issue data on a fresh tip reports lease-only live",
			branch:      "wip/issue-9",
			tip:         tip(1),
			issue:       nil,
			wantVerdict: live,
			wantReason:  "lease-only",
		},
		{
			name:        "missing issue data on an old tip reports lease-only expired",
			branch:      "wip/issue-10",
			tip:         tip(600),
			issue:       nil,
			wantVerdict: expired,
			wantReason:  "lease-only",
		},
		{
			name:        "absent tip with no issue data is unknown, not expired",
			branch:      "wip/issue-11",
			tip:         nil,
			issue:       nil,
			wantVerdict: unknown,
			wantReason:  "not fetched",
		},
		{
			name:        "absent tip with an open issue is still unknown",
			branch:      "wip/issue-12",
			tip:         nil,
			issue:       &issueState{number: 12, closed: false, needsReplan: false},
			wantVerdict: unknown,
			wantReason:  "not fetched",
		},
		{
			// The #722 shape: a lease pushed minutes ago at main's tip,
			// where the tip's own age is hours past the TTL.
			name:        "zero-commit branch with a stale borrowed tip is claimed, not expired",
			branch:      "wip/issue-13",
			tip:         tip(600),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 13, closed: false, needsReplan: false},
			wantVerdict: claimed,
			wantReason:  "no commits of its own",
		},
		{
			name:        "zero-commit branch with a fresh borrowed tip is claimed, not live",
			branch:      "wip/issue-14",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 14, closed: false, needsReplan: false},
			wantVerdict: claimed,
			wantReason:  "tip age is main's",
		},
		{
			name:        "closed issue retires a zero-commit branch",
			branch:      "wip/issue-15",
			tip:         tip(600),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 15, closed: true},
			wantVerdict: retired,
			wantReason:  "closed",
			// Retirement does not make main's 10h the branch's own lease
			// age; the cell must say whose it is (#809).
			wantCell: "main's",
		},
		{
			name:        "needs-replan retires a zero-commit branch",
			branch:      "wip/issue-16",
			tip:         tip(600),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 16, needsReplan: true},
			wantVerdict: retired,
			wantReason:  "needs-replan",
			wantCell:    "main's",
		},
		{
			name:        "unfetched tip beats the ancestry answer",
			branch:      "wip/issue-17",
			tip:         nil,
			anc:         ancestryNoCommits,
			issue:       nil,
			wantVerdict: unknown,
			wantReason:  "not fetched",
		},
		{
			name:        "branch with commits of its own is still dated from its tip",
			branch:      "wip/issue-18",
			tip:         tip(600),
			anc:         ancestryOwnCommits,
			issue:       &issueState{number: 18, closed: false, needsReplan: false},
			wantVerdict: expired,
			wantReason:  "past",
		},
		{
			name:        "branch with commits of its own and a fresh tip is live",
			branch:      "wip/issue-19",
			tip:         tip(5),
			anc:         ancestryOwnCommits,
			issue:       &issueState{number: 19, closed: false, needsReplan: false},
			wantVerdict: live,
			wantReason:  "within",
		},
		{
			// git could not decide (exit 128, or no main on the remote):
			// fall back to the tip age rather than invent a second unknown.
			name:        "unresolved ancestry falls back to the tip age",
			branch:      "wip/issue-20",
			tip:         tip(600),
			anc:         ancestryUnresolved,
			issue:       &issueState{number: 20, closed: false, needsReplan: false},
			wantVerdict: expired,
			wantReason:  "past",
		},
		{
			name:        "unresolved ancestry says so on an expired row",
			branch:      "wip/issue-22",
			tip:         tip(600),
			anc:         ancestryUnresolved,
			issue:       &issueState{number: 22, closed: false, needsReplan: false},
			wantVerdict: expired,
			wantReason:  "ancestry against main undecided",
		},
		{
			name:        "unresolved ancestry says so on a live row too",
			branch:      "wip/issue-23",
			tip:         tip(5),
			anc:         ancestryUnresolved,
			issue:       &issueState{number: 23, closed: false, needsReplan: false},
			wantVerdict: live,
			wantReason:  "ancestry against main undecided",
		},
		{
			name:        "unresolved ancestry names the fetch that would decide it",
			branch:      "wip/issue-24",
			tip:         tip(600),
			anc:         ancestryUnresolved,
			issue:       nil,
			wantVerdict: expired,
			wantReason:  "run `git fetch origin`",
		},
		{
			name:        "zero-commit branch with no issue data still says lease-only",
			branch:      "wip/issue-21",
			tip:         tip(600),
			anc:         ancestryNoCommits,
			issue:       nil,
			wantVerdict: claimed,
			wantReason:  "lease-only",
		},
		{
			// The empty claim's own clock: a fresh heartbeat holds the lease
			// even though the borrowed tip is ten hours old.
			name:        "zero-commit branch with a fresh heartbeat is live",
			branch:      "wip/issue-25",
			tip:         tip(600),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 25, commentsRead: true, heartbeat: heartbeat(30)},
			wantVerdict: live,
			wantReason:  "lease dated by its newest RESUME:/TAKEOVER: comment, posted 30m0s ago",
		},
		{
			name:        "zero-commit branch with a heartbeat exactly at the TTL is live",
			branch:      "wip/issue-26",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 26, commentsRead: true, heartbeat: heartbeat(120)},
			wantVerdict: live,
			wantReason:  "within",
		},
		{
			// The #981 sighting: the takeover comment that dated the lease was
			// 4h18m old, so the claim was takeable.
			name:        "zero-commit branch with a heartbeat past the TTL is takeable",
			branch:      "wip/issue-27",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 27, commentsRead: true, heartbeat: heartbeat(258)},
			wantVerdict: expired,
			wantReason:  "takeable",
			// The heartbeat's own age, not the borrowed tip's 1m.
			wantCell: "4h18m0s",
		},
		{
			// Two takeovers are still a plain takeable claim: below the
			// threshold the reason is what it has always been.
			name:        "zero-commit branch taken over twice reads as any other takeable claim",
			branch:      "wip/issue-1437",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			diff:        diffEmpty,
			issue:       &issueState{number: 1437, commentsRead: true, heartbeat: heartbeat(258), takeovers: 2},
			wantVerdict: expired,
			wantReason:  "past the 2h0m0s claim TTL -- takeable",
		},
		{
			name:        "zero-commit branch at the takeover threshold names the remedy",
			branch:      "wip/issue-1437",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			diff:        diffEmpty,
			issue:       &issueState{number: 1437, commentsRead: true, heartbeat: heartbeat(258), takeovers: 3},
			wantVerdict: expired,
			wantReason:  "already TAKEOVER: comment #3 with no diff ever produced -- relabel the issue needs-replan instead of resuming",
		},
		{
			// The count printed is the live one, so the reason reads the same
			// at cycle 3 and at cycle 10 rather than freezing at the
			// threshold.
			name:        "zero-commit branch past the takeover threshold names the live count",
			branch:      "wip/issue-1437",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			diff:        diffEmpty,
			issue:       &issueState{number: 1437, commentsRead: true, heartbeat: heartbeat(258), takeovers: 5},
			wantVerdict: expired,
			wantReason:  "already TAKEOVER: comment #5 with no diff ever produced",
		},
		{
			// wip/issue-1332's measured shape: heartbeat commits and
			// merge-forwards, so the branch has commits of its own and takes
			// the tip-age path, and a net diff of nothing.
			name:        "branch of heartbeat commits with an empty diff names the remedy on the tip-age path",
			branch:      "wip/issue-1332",
			tip:         tip(240),
			anc:         ancestryOwnCommits,
			diff:        diffEmpty,
			issue:       &issueState{number: 1332, commentsRead: true, heartbeat: heartbeat(240), takeovers: 12},
			wantVerdict: expired,
			wantReason:  "past the 2h0m0s claim TTL, already TAKEOVER: comment #12 with no diff ever produced -- relabel the issue needs-replan instead of resuming",
		},
		{
			// A LIVE tip-age verdict never carries the remedy: the claim is
			// someone's right now, whatever its history of hand-offs. The
			// verdict is what this row pins; TestClassifyRemedyThreshold
			// pins the reason byte for byte, which wantReason's substring
			// match cannot do.
			name:        "fresh tip with an empty diff and many takeovers stays a plain LIVE",
			branch:      "wip/issue-1332",
			tip:         tip(5),
			anc:         ancestryOwnCommits,
			diff:        diffEmpty,
			issue:       &issueState{number: 1332, commentsRead: true, heartbeat: heartbeat(5), takeovers: 12},
			wantVerdict: live,
			wantReason:  "wip/issue-1332: tip pushed 5m0s ago, within the 2h0m0s claim TTL",
		},
		{
			// /develop pushes the claim before it posts anything, so this is
			// also the shape of an issue being grounded right now: only an
			// AGED heartbeat may demote a claim (#981).
			name:        "zero-commit branch whose thread carries no heartbeat stays claimed",
			branch:      "wip/issue-28",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 28, commentsRead: true},
			wantVerdict: claimed,
			wantReason:  "a claim is born undated, so this is not a lapsed lease",
			wantCell:    "main's",
		},
		{
			name:        "comments not supplied leaves the empty claim undated",
			branch:      "wip/issue-29",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 29},
			wantVerdict: claimed,
			wantReason:  "supply this issue's comments",
		},
		{
			name:        "a closed issue retires an empty claim with a fresh heartbeat",
			branch:      "wip/issue-30",
			tip:         tip(1),
			anc:         ancestryNoCommits,
			issue:       &issueState{number: 30, closed: true, commentsRead: true, heartbeat: heartbeat(1)},
			wantVerdict: retired,
			wantReason:  "closed",
			// Retirement short-circuits the thread's clock too, so nothing
			// dates this row and the tip it holds is main's.
			wantCell: "main's",
		},
		{
			// A branch with commits of its own is dated by its tip and nothing
			// else: a stale tip is EXPIRED however fresh the thread is.
			name:        "a heartbeat does not extend a branch that has its own tip",
			branch:      "wip/issue-31",
			tip:         tip(600),
			anc:         ancestryOwnCommits,
			issue:       &issueState{number: 31, commentsRead: true, heartbeat: heartbeat(1)},
			wantVerdict: expired,
			wantReason:  "tip pushed",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotVerdict, gotLease, gotReason := classify(c.branch, c.tip, c.anc, c.diff, fixedNow, c.issue)
			if gotVerdict != c.wantVerdict {
				t.Errorf("classify(%q) verdict = %s, want %s (reason: %s)", c.branch, gotVerdict, c.wantVerdict, gotReason)
			}
			if !strings.Contains(gotReason, c.wantReason) {
				t.Errorf("classify(%q) reason = %q, want substring %q", c.branch, gotReason, c.wantReason)
			}
			if c.wantCell == "" {
				return
			}
			got := leaseAgeCell(row{lease: gotLease, anc: c.anc, verdict: gotVerdict})
			if got != c.wantCell {
				t.Errorf("classify(%q) LEASE AGE cell = %q, want %q", c.branch, got, c.wantCell)
			}
		})
	}
}

// TestClassifyNeverFallsThroughToExpired guards specifically against the
// hazard the coordinator's correction called out: a nil tip must never
// silently read as EXPIRED, because EXPIRED means resumable and that is
// the dangerous direction for a branch this tool has no age data for.
func TestClassifyNeverFallsThroughToExpired(t *testing.T) {
	got, _, _ := classify("wip/issue-99", nil, ancestryUnresolved, diffUnresolved, fixedNow, nil)
	if got == expired {
		t.Fatalf("classify with a nil tip returned EXPIRED; an absent tip must never be treated as an old one")
	}
	if got != unknown {
		t.Fatalf("classify with a nil tip and no issue data = %s, want UNKNOWN", got)
	}
}

// TestClassifyNeverExpiresABorrowedTip guards the #722 hazard at every tip
// age: a branch with no commits of its own has no age of its own, so no
// arrangement of the TIP clock may make it EXPIRED — the verdict /develop
// reads as "resumable" and /backlog reads as grounds for needs-replan.
// Only the thread's own clock decides such a branch, and this test hands
// classify no heartbeat to read on any thread: absent issue data, an issue
// whose comments were not supplied, and a thread that was read and carries
// no heartbeat. None of the three is a lapsed lease (#981).
func TestClassifyNeverExpiresABorrowedTip(t *testing.T) {
	for _, agoMinutes := range []int{0, 1, 119, 120, 121, 600, 100000} {
		tip := fixedNow.Add(-time.Duration(agoMinutes) * time.Minute)
		for _, issue := range []*issueState{nil, {number: 98}, {number: 98, commentsRead: true}} {
			got, lease, reason := classify("wip/issue-98", &tip, ancestryNoCommits, diffEmpty, fixedNow, issue)
			if got != claimed {
				t.Errorf("classify with a %dm-old borrowed tip = %s, want CLAIMED (reason: %s)", agoMinutes, got, reason)
			}
			if lease != nil {
				t.Errorf("classify with a %dm-old borrowed tip dated the lease %v; a borrowed tip dates nothing", agoMinutes, *lease)
			}
		}
	}
}

// TestClassifyEmptyClaimIgnoresTheBorrowedTip pins the same hazard on the
// path that CAN return EXPIRED for an empty claim: once the thread dates
// the lease, the tip's age must not reach the arithmetic at all, in either
// direction.
func TestClassifyEmptyClaimIgnoresTheBorrowedTip(t *testing.T) {
	fresh := fixedNow.Add(-30 * time.Minute)
	stale := fixedNow.Add(-500 * time.Minute)
	cases := []struct {
		name      string
		heartbeat time.Time
		want      verdict
	}{
		{"heartbeat within the TTL", fresh, live},
		{"heartbeat past the TTL", stale, expired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, tipAgoMinutes := range []int{0, 1, 119, 120, 121, 600, 100000} {
				tip := fixedNow.Add(-time.Duration(tipAgoMinutes) * time.Minute)
				issue := &issueState{number: 97, commentsRead: true, heartbeat: &c.heartbeat}
				got, lease, reason := classify("wip/issue-97", &tip, ancestryNoCommits, diffEmpty, fixedNow, issue)
				if got != c.want {
					t.Errorf("tip %dm old, %s: verdict = %s, want %s (reason: %s)", tipAgoMinutes, c.name, got, c.want, reason)
				}
				if lease == nil || *lease != fixedNow.Sub(c.heartbeat) {
					t.Errorf("tip %dm old, %s: lease age = %v, want the heartbeat's %v", tipAgoMinutes, c.name, lease, fixedNow.Sub(c.heartbeat))
				}
			}
		})
	}
}

// remedy is the clause #1437's rule appends to an EXPIRED reason, built
// the way the test expects to read it rather than the way classify builds
// it, so a reworded clause fails rather than following along.
func remedy(takeovers int) string {
	return fmt.Sprintf(", already TAKEOVER: comment #%d with no diff ever produced -- relabel the issue needs-replan instead of resuming", takeovers)
}

// TestClassifyRemedyThreshold pins #1437's rule on BOTH paths that return
// EXPIRED, with full-string reasons so the clause's absence is pinned as
// hard as its presence — TestClassify's wantReason is a substring match
// and cannot do that.
//
// The tip-age path is the load-bearing one: WORKFLOW.md's lease invariant
// has every takeover push a heartbeat commit, so a branch taken over even
// once has commits of its own and never reaches classifyEmptyClaim again.
// wip/issue-1332, this issue's own evidence branch, is that shape —
// commits present, diff empty, twelve takeovers — and a rule that fired
// only on the zero-commit path said nothing on it at any cycle.
func TestClassifyRemedyThreshold(t *testing.T) {
	heartbeat := fixedNow.Add(-258 * time.Minute)
	borrowedTip := fixedNow.Add(-1 * time.Minute)
	ownTip := fixedNow.Add(-258 * time.Minute)
	const emptyClaim = "wip/issue-1437: no commits of its own; newest RESUME:/TAKEOVER: comment posted 4h18m0s ago, past the 2h0m0s claim TTL -- takeable"
	const tipAge = "wip/issue-1437: tip pushed 4h18m0s ago, past the 2h0m0s claim TTL"
	const undecided = "wip/issue-1437: tip pushed 4h18m0s ago, past the 2h0m0s claim TTL; ancestry against main undecided, so this age may be main's rather than the claim's -- run `git fetch origin`"

	paths := []struct {
		name string
		tip  time.Time
		anc  ancestry
		diff netDiff
		// base is the reason with no remedy clause on it — what this path
		// printed before #1437 and what it must still print below the
		// threshold. wantRemedy says whether the clause is appended at and
		// above the threshold.
		base       string
		wantRemedy bool
	}{
		{
			name:       "zero commits, empty diff",
			tip:        borrowedTip,
			anc:        ancestryNoCommits,
			diff:       diffEmpty,
			base:       emptyClaim,
			wantRemedy: true,
		},
		{
			name:       "own commits, empty diff",
			tip:        ownTip,
			anc:        ancestryOwnCommits,
			diff:       diffEmpty,
			base:       tipAge,
			wantRemedy: true,
		},
		{
			// The remedy is about producing nothing, not about being
			// contended: a branch holding real work keeps the plain reason
			// however often it has changed hands.
			name:       "own commits, non-empty diff",
			tip:        ownTip,
			anc:        ancestryOwnCommits,
			diff:       diffNonEmpty,
			base:       tipAge,
			wantRemedy: false,
		},
		{
			// git could not resolve the diff — an unfetched object, or no
			// main on the remote. Undecided is not "produced nothing".
			name:       "own commits, undecided diff",
			tip:        ownTip,
			anc:        ancestryOwnCommits,
			diff:       diffUnresolved,
			base:       tipAge,
			wantRemedy: false,
		},
		{
			// The age itself is already provisional here (#806), so no
			// remedy is named on top of it.
			name:       "undecided ancestry, empty diff",
			tip:        ownTip,
			anc:        ancestryUnresolved,
			diff:       diffEmpty,
			base:       undecided,
			wantRemedy: false,
		},
	}

	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			for _, takeovers := range []int{0, 1, 2, 3, 4, 5, 10} {
				issue := &issueState{number: 1437, commentsRead: true, heartbeat: &heartbeat, takeovers: takeovers}
				got, _, reason := classify("wip/issue-1437", &p.tip, p.anc, p.diff, fixedNow, issue)
				if got != expired {
					t.Errorf("%d takeovers: verdict = %s, want EXPIRED, not a new verdict (reason: %s)", takeovers, got, reason)
				}
				want := p.base
				if p.wantRemedy && takeovers >= 3 {
					want += remedy(takeovers)
				}
				if reason != want {
					t.Errorf("%d takeovers: reason = %q, want %q", takeovers, reason, want)
				}
			}
		})
	}
}

// TestRemedyClauseNeedsIssueData pins the clause off a row with no issue
// data at all: stdin was empty or carried no entry for this branch, so
// nothing has been counted and nothing may be concluded from the count.
func TestRemedyClauseNeedsIssueData(t *testing.T) {
	if got := remedyClause(ancestryOwnCommits, diffEmpty, nil); got != "" {
		t.Fatalf("remedyClause with no issue data = %q, want the empty string", got)
	}
}

// missingComments is the clause #1493's rule appends to a tip-age EXPIRED
// reason, written out here rather than read from the code, so a reworded
// clause fails instead of following along.
const missingComments = "; no comments supplied for this issue, so its TAKEOVER: count is zero because nothing was counted, not because nothing happened -- supply them before taking the claim over"

// TestClassifyExpiredNamesMissingComments pins #1493's rule with
// full-string reasons, so the clause's absence is pinned as hard as its
// presence.
//
// The rule fires where remedyClause reads a takeover count of zero that
// nothing was counted into, and declines where remedyClause declines on
// evidence: an undecided ancestry or an undecided diff leaves the question
// open rather than answered no, and a row whose comments WERE supplied is
// reporting a real absence of takeovers, not a missing input.
func TestClassifyExpiredNamesMissingComments(t *testing.T) {
	tip := fixedNow.Add(-258 * time.Minute)
	const tipAge = "wip/issue-1493: tip pushed 4h18m0s ago, past the 2h0m0s claim TTL"
	const leaseOnly = "; no issue data for this branch, lease-only"
	const undecided = "; ancestry against main undecided, so this age may be main's rather than the claim's -- run `git fetch origin`"

	cases := []struct {
		name  string
		anc   ancestry
		diff  netDiff
		issue *issueState
		want  string
	}{
		{
			// stdin was empty, or carried no entry for this branch: the row
			// already says the verdict is lease-only, which names missing
			// issue data without saying what to supply.
			name: "no issue data at all",
			anc:  ancestryOwnCommits,
			diff: diffEmpty,
			want: tipAge + leaseOnly + missingComments,
		},
		{
			// The input ROUTINES.md's closing caveat admits as supported: an
			// issue with no `comments` key. Before #1493 this row was
			// byte-identical to the one below it, which is the whole defect.
			name:  "issue supplied, comments key absent",
			anc:   ancestryOwnCommits,
			diff:  diffEmpty,
			issue: &issueState{number: 1493},
			want:  tipAge + missingComments,
		},
		{
			name:  "comments supplied, thread carries no takeover",
			anc:   ancestryOwnCommits,
			diff:  diffEmpty,
			issue: &issueState{number: 1493, commentsRead: true},
			want:  tipAge,
		},
		{
			name:  "comments supplied, takeovers below the threshold",
			anc:   ancestryOwnCommits,
			diff:  diffEmpty,
			issue: &issueState{number: 1493, commentsRead: true, takeovers: 2},
			want:  tipAge,
		},
		{
			// The age is already provisional here (#806), so no note about
			// an input that would not settle it is added on top.
			name: "undecided ancestry with no issue data",
			anc:  ancestryUnresolved,
			diff: diffEmpty,
			want: tipAge + undecided + leaseOnly,
		},
		{
			name:  "undecided ancestry with the comments key absent",
			anc:   ancestryUnresolved,
			diff:  diffEmpty,
			issue: &issueState{number: 1493},
			want:  tipAge + undecided,
		},
		{
			name:  "undecided diff with the comments key absent",
			anc:   ancestryOwnCommits,
			diff:  diffUnresolved,
			issue: &issueState{number: 1493},
			want:  tipAge,
		},
		{
			// A branch holding real work is not waiting on a takeover count
			// at all, so its missing comments change no verdict here.
			name:  "non-empty diff with the comments key absent",
			anc:   ancestryOwnCommits,
			diff:  diffNonEmpty,
			issue: &issueState{number: 1493},
			want:  tipAge,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, reason := classify("wip/issue-1493", &tip, c.anc, c.diff, fixedNow, c.issue)
			if got != expired {
				t.Fatalf("verdict = %s, want EXPIRED (reason: %s)", got, reason)
			}
			if reason != c.want {
				t.Errorf("reason = %q, want %q", reason, c.want)
			}
		})
	}
}

// TestClassifyCommentsReadChangesTheExpiredRow pins the distinction the
// clause exists to draw: the row whose comments were supplied and the row
// whose comments were not must not print the same bytes (#1493), and a
// LIVE claim carries no such note whatever its input, because nothing is
// being concluded from the count there.
func TestClassifyCommentsReadChangesTheExpiredRow(t *testing.T) {
	stale := fixedNow.Add(-258 * time.Minute)
	fresh := fixedNow.Add(-5 * time.Minute)
	unread := &issueState{number: 1493}
	read := &issueState{number: 1493, commentsRead: true}

	_, _, unreadReason := classify("wip/issue-1493", &stale, ancestryOwnCommits, diffEmpty, fixedNow, unread)
	_, _, readReason := classify("wip/issue-1493", &stale, ancestryOwnCommits, diffEmpty, fixedNow, read)
	if unreadReason == readReason {
		t.Errorf("comments-unread and comments-read EXPIRED rows are identical: %q", unreadReason)
	}

	got, _, reason := classify("wip/issue-1493", &fresh, ancestryOwnCommits, diffEmpty, fixedNow, unread)
	if got != live {
		t.Fatalf("verdict = %s, want LIVE (reason: %s)", got, reason)
	}
	if strings.Contains(reason, "no comments supplied") {
		t.Errorf("LIVE reason = %q, names a missing input nothing is being concluded from", reason)
	}
}

// TestClassifyMarksOnlyUndecidedAncestry pins the marker to the rows it
// belongs on. An age-based verdict earns it when git could not place the
// branch against main, because the age it rests on may then be the
// borrowed one #722 is about; a verdict whose ancestry git did decide must
// NOT carry it, or the marker stops distinguishing anything (#806).
func TestClassifyMarksOnlyUndecidedAncestry(t *testing.T) {
	const marker = "ancestry against main undecided"
	tips := []struct {
		name string
		tip  time.Time
	}{
		{"fresh", fixedNow.Add(-5 * time.Minute)},
		{"stale", fixedNow.Add(-10 * time.Hour)},
	}
	decided := []struct {
		name string
		anc  ancestry
	}{
		{"ownCommits", ancestryOwnCommits},
		{"noCommits", ancestryNoCommits},
	}

	for _, tc := range tips {
		got, _, reason := classify("wip/issue-1", &tc.tip, ancestryUnresolved, diffUnresolved, fixedNow, nil)
		if !strings.Contains(reason, marker) {
			t.Errorf("classify(%s tip, unresolved) = %s, reason %q lacks %q", tc.name, got, reason, marker)
		}
		for _, d := range decided {
			got, _, reason := classify("wip/issue-1", &tc.tip, d.anc, diffUnresolved, fixedNow, nil)
			if strings.Contains(reason, marker) {
				t.Errorf("classify(%s tip, %s) = %s, reason %q calls an ancestry git decided undecided", tc.name, d.name, got, reason)
			}
		}
	}
}

// TestAncestryFromExit pins the exit-status mapping `git merge-base
// --is-ancestor` defines, including the 128 that means git declined to
// answer: mapping that to "has its own commits" would silently restore the
// borrowed-age EXPIRED (#722), so it must land on unresolved instead.
func TestAncestryFromExit(t *testing.T) {
	cases := []struct {
		code int
		want ancestry
	}{
		{0, ancestryNoCommits},
		{1, ancestryOwnCommits},
		{128, ancestryUnresolved},
		{129, ancestryUnresolved},
	}
	for _, c := range cases {
		if got := ancestryFromExit(c.code); got != c.want {
			t.Errorf("ancestryFromExit(%d) = %v, want %v", c.code, got, c.want)
		}
	}
}

// TestGitAncestryWithoutMain checks the no-main-on-the-remote path
// resolves to unresolved without running git at all — the survey still
// reports, dated from tip ages.
func TestGitAncestryWithoutMain(t *testing.T) {
	got, err := gitAncestry("aaaa111", "")
	if err != nil {
		t.Fatalf("gitAncestry with no main SHA: unexpected error: %v", err)
	}
	if got != ancestryUnresolved {
		t.Fatalf("gitAncestry with no main SHA = %v, want ancestryUnresolved", got)
	}
}

// TestNetDiffFromExit pins the exit-status mapping `git diff --quiet`
// defines, including the 128 that means git declined to answer: mapping
// that to "empty" would have the survey tell a session to retire an issue
// whose branch this checkout simply has not fetched.
func TestNetDiffFromExit(t *testing.T) {
	cases := []struct {
		code int
		want netDiff
	}{
		{0, diffEmpty},
		{1, diffNonEmpty},
		{128, diffUnresolved},
		{129, diffUnresolved},
	}
	for _, c := range cases {
		if got := netDiffFromExit(c.code); got != c.want {
			t.Errorf("netDiffFromExit(%d) = %v, want %v", c.code, got, c.want)
		}
	}
}

// TestGitEmptyDiffWithoutMain checks the no-main-on-the-remote path
// resolves to unresolved without running git at all — the survey still
// reports, naming no remedy.
func TestGitEmptyDiffWithoutMain(t *testing.T) {
	got, err := gitEmptyDiff("aaaa111", "")
	if err != nil {
		t.Fatalf("gitEmptyDiff with no main SHA: unexpected error: %v", err)
	}
	if got != diffUnresolved {
		t.Fatalf("gitEmptyDiff with no main SHA = %v, want diffUnresolved", got)
	}
}

// TestGitEmptyDiffUnfetchedObject shells git for real against a SHA no
// checkout has: git exits 128, and the mapping must report that as
// undecided rather than as an error that aborts the whole survey.
func TestGitEmptyDiffUnfetchedObject(t *testing.T) {
	const absent = "0000000000000000000000000000000000000001"
	got, err := gitEmptyDiff(absent, absent)
	if err != nil {
		t.Fatalf("gitEmptyDiff on an unfetched object: unexpected error: %v", err)
	}
	if got != diffUnresolved {
		t.Fatalf("gitEmptyDiff on an unfetched object = %v, want diffUnresolved", got)
	}
}

// TestParseAheadBehind exercises the pure rev-list output parser: no git
// call, just the text `git rev-list --left-right --count <main>...<ref>`
// would have produced.
//
// Which side is which is the case that matters. rev-list prints the LEFT
// count first, and the left side is main, so the first number is what the
// ref is BEHIND and the second is what it is AHEAD. Reading them the other
// way round would report a merged session branch as carrying hundreds of
// stranded commits and a genuinely stranded one as carrying none — the
// exact inversion of the state the section exists to flag (#1627).
func TestParseAheadBehind(t *testing.T) {
	t.Run("the left count is behind and the right is ahead", func(t *testing.T) {
		got, err := parseAheadBehind("13\t1\n")
		if err != nil {
			t.Fatalf("parseAheadBehind: %v", err)
		}
		if got.behind != 13 {
			t.Errorf("behind = %d, want 13 (the left count, what main has and the ref does not)", got.behind)
		}
		if got.ahead != 1 {
			t.Errorf("ahead = %d, want 1 (the right count, what the ref has and main does not)", got.ahead)
		}
	})

	t.Run("a merged branch reads ahead zero", func(t *testing.T) {
		got, err := parseAheadBehind("270\t0\n")
		if err != nil {
			t.Fatalf("parseAheadBehind: %v", err)
		}
		if got.ahead != 0 || got.behind != 270 {
			t.Errorf("parseAheadBehind(\"270\\t0\") = %+v, want ahead 0 behind 270", got)
		}
	})

	t.Run("an identical ref reads zero both ways", func(t *testing.T) {
		got, err := parseAheadBehind("0\t0\n")
		if err != nil {
			t.Fatalf("parseAheadBehind: %v", err)
		}
		if got.ahead != 0 || got.behind != 0 {
			t.Errorf("parseAheadBehind(\"0\\t0\") = %+v, want both zero", got)
		}
	})

	malformed := []struct {
		name   string
		output string
	}{
		{name: "empty output", output: ""},
		{name: "one count only", output: "13\n"},
		{name: "three counts", output: "13\t1\t7\n"},
		{name: "non-numeric behind", output: "main\t1\n"},
		{name: "non-numeric ahead", output: "13\tahead\n"},
	}
	for _, c := range malformed {
		t.Run(c.name+" is an error, not a zero", func(t *testing.T) {
			got, err := parseAheadBehind(c.output)
			if err == nil {
				t.Fatalf("parseAheadBehind(%q) = %+v, want an error: a count nothing parsed is not a count of zero", c.output, got)
			}
		})
	}
}

// TestGitAheadBehindWithoutMain checks the no-main-on-the-remote path
// resolves to undecided without running git at all — the survey still
// reports, with the counts unstated rather than invented.
func TestGitAheadBehindWithoutMain(t *testing.T) {
	got, err := gitAheadBehind("aaaa111", "", false)
	if err != nil {
		t.Fatalf("gitAheadBehind with no main SHA: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("gitAheadBehind with no main SHA = %+v, want nil", got)
	}
}

// TestGitAheadBehindUnfetchedObject shells git for real against a SHA no
// checkout has: git exits 128, and that must report as undecided rather
// than as an error aborting the whole survey, or as the ahead=0 that would
// claim nothing is stranded on a ref nobody counted.
func TestGitAheadBehindUnfetchedObject(t *testing.T) {
	const absent = "0000000000000000000000000000000000000001"
	got, err := gitAheadBehind(absent, absent, false)
	if err != nil {
		t.Fatalf("gitAheadBehind on an unfetched object: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("gitAheadBehind on an unfetched object = %+v, want nil", got)
	}
}

// gitOut runs git in dir for a temp-repo fixture and returns its trimmed
// output, isolated from the user's and the system's git config so a
// signing or hooks setting there cannot change what the fixture builds.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=wipsurvey test", "GIT_AUTHOR_EMAIL=wipsurvey@example.invalid",
		"GIT_COMMITTER_NAME=wipsurvey test", "GIT_COMMITTER_EMAIL=wipsurvey@example.invalid",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitEmpty commits nothing under subject in dir and returns the new
// commit's SHA.
func commitEmpty(t *testing.T, dir, subject string) string {
	t.Helper()
	gitOut(t, dir, "commit", "--quiet", "--allow-empty", "-m", subject)
	return gitOut(t, dir, "rev-parse", "HEAD")
}

// TestGitAheadBehindNamesTheStrandedPass shells git for real against a
// temp repo carrying #1705's two shapes: a head whose commits ahead of main
// include a /backlog pass under a later commit, and one ahead by a
// recovery commit that only mentions a backlog LOG entry. The first must
// come back naming the pass, the second must not, and neither may be taken
// at all once the checkout is called shallow.
func TestGitAheadBehindNamesTheStrandedPass(t *testing.T) {
	dir := t.TempDir()
	gitOut(t, dir, "init", "--quiet", "--initial-branch=main")
	mainSHA := commitEmpty(t, dir, "docs: the landing main is at")
	commitEmpty(t, dir, "meta: backlog 2026-09-24")
	strandedSHA := commitEmpty(t, dir, "docs: a later commit on the same head")
	gitOut(t, dir, "checkout", "--quiet", "-b", "recovery", mainSHA)
	recoverySHA := commitEmpty(t, dir, "meta: recover stranded 2026-09-21 backlog LOG entry (#1649)")
	t.Chdir(dir)

	got, err := gitAheadBehind(strandedSHA, mainSHA, false)
	if err != nil {
		t.Fatalf("gitAheadBehind on the stranded head: %v", err)
	}
	want := aheadBehind{ahead: 2, behind: 0, backlogSubject: "meta: backlog 2026-09-24"}
	if got == nil || *got != want {
		t.Errorf("gitAheadBehind on the stranded head = %+v, want %+v", got, want)
	}

	got, err = gitAheadBehind(recoverySHA, mainSHA, false)
	if err != nil {
		t.Fatalf("gitAheadBehind on the recovery head: %v", err)
	}
	want = aheadBehind{ahead: 1, behind: 0}
	if got == nil || *got != want {
		t.Errorf("gitAheadBehind on the recovery head = %+v, want %+v", got, want)
	}

	for _, sha := range []string{strandedSHA, recoverySHA} {
		got, err := gitAheadBehind(sha, mainSHA, true)
		if err != nil {
			t.Fatalf("gitAheadBehind in a shallow checkout: unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("gitAheadBehind(%s) in a shallow checkout = %+v, want nil: a shallow count is not a count", sha, got)
		}
	}
}

// TestGitShallow checks the guard reads a real clone's depth, in both
// directions: a full repo is not shallow, and a --depth 1 clone of it is.
func TestGitShallow(t *testing.T) {
	full := t.TempDir()
	gitOut(t, full, "init", "--quiet", "--initial-branch=main")
	commitEmpty(t, full, "docs: first")
	commitEmpty(t, full, "docs: second")
	shallow := t.TempDir()
	gitOut(t, shallow, "clone", "--quiet", "--depth", "1", "file://"+full, ".")

	for _, c := range []struct {
		dir  string
		want bool
	}{{full, false}, {shallow, true}} {
		t.Chdir(c.dir)
		got, err := gitShallow()
		if err != nil {
			t.Fatalf("gitShallow in %s: %v", c.dir, err)
		}
		if got != c.want {
			t.Errorf("gitShallow in %s = %v, want %v", c.dir, got, c.want)
		}
	}
}

// TestParseShallow checks rev-parse's two answers parse and that anything
// else — git before 2.15 echoing the flag it does not know — is an error,
// never the "not shallow" that would take every count the guard withholds.
func TestParseShallow(t *testing.T) {
	for _, c := range []struct {
		output string
		want   bool
	}{{"true\n", true}, {"false\n", false}} {
		got, err := parseShallow(c.output)
		if err != nil || got != c.want {
			t.Errorf("parseShallow(%q) = %v, %v; want %v, nil", c.output, got, err, c.want)
		}
	}
	for _, output := range []string{"", "--is-shallow-repository\n", "yes\n"} {
		if got, err := parseShallow(output); err == nil {
			t.Errorf("parseShallow(%q) = %v, nil; want an error", output, got)
		}
	}
}

// TestPartitionRefs checks main is taken out of the surveyed branch set —
// it is the ancestry test's right-hand side, never a row — and that the
// three namespaces WORKFLOW.md's branch scheme distinguishes each land in
// their own bucket. The whole of #1627 is that a ref in a namespace with
// no bucket is a ref nothing reports.
func TestPartitionRefs(t *testing.T) {
	refs := []refSHA{
		{sha: "aaaa111", ref: "refs/heads/wip/issue-1"},
		{sha: "mmmm999", ref: "refs/heads/main"},
		{sha: "pppp333", ref: "refs/heads/parked/untriaged-20260918-101500"},
		{sha: "cccc444", ref: "refs/heads/claude/dazzling-cerf-u3xy2k"},
		{sha: "bbbb222", ref: "refs/heads/wip/issue-2"},
	}
	got := partitionRefs(refs)
	if got.mainSHA != "mmmm999" {
		t.Errorf("mainSHA = %q, want %q", got.mainSHA, "mmmm999")
	}
	if len(got.wips) != 2 || got.wips[0].ref != "refs/heads/wip/issue-1" || got.wips[1].ref != "refs/heads/wip/issue-2" {
		t.Errorf("wips = %+v, want the two wip refs in ls-remote order", got.wips)
	}
	if len(got.parked) != 1 || got.parked[0].ref != "refs/heads/parked/untriaged-20260918-101500" {
		t.Errorf("parked = %+v, want the one parked ref", got.parked)
	}
	if len(got.others) != 1 || got.others[0].ref != "refs/heads/claude/dazzling-cerf-u3xy2k" {
		t.Errorf("others = %+v, want the one ref in neither namespace", got.others)
	}

	t.Run("no main on the remote yields an empty SHA", func(t *testing.T) {
		got := partitionRefs([]refSHA{{sha: "aaaa111", ref: "refs/heads/wip/issue-1"}})
		if got.mainSHA != "" {
			t.Errorf("mainSHA = %q, want empty", got.mainSHA)
		}
		if len(got.wips) != 1 {
			t.Errorf("wips = %+v, want the one wip ref", got.wips)
		}
	})

	t.Run("a parked ref is never surveyed as a lease", func(t *testing.T) {
		got := partitionRefs([]refSHA{{sha: "pppp333", ref: "refs/heads/parked/untriaged-20260918-101500"}})
		if len(got.wips) != 0 {
			t.Errorf("wips = %+v, want none: a parked branch is not a claim on an issue", got.wips)
		}
		if len(got.others) != 0 {
			t.Errorf("others = %+v, want none: parked/* has a section of its own", got.others)
		}
		if len(got.parked) != 1 {
			t.Errorf("parked = %+v, want the one parked ref", got.parked)
		}
	})

	t.Run("a malformed wip ref stays a lease candidate", func(t *testing.T) {
		// run warns on it; rehoming it into the other-branch section would
		// silence exactly the evidence that warning exists for.
		got := partitionRefs([]refSHA{{sha: "aaaa111", ref: "refs/heads/wip/issue-abc"}})
		if len(got.wips) != 1 || len(got.others) != 0 {
			t.Errorf("partitionRefs = %+v, want the malformed ref in wips", got)
		}
	})

	t.Run("a head whose name only starts like a namespace is an other", func(t *testing.T) {
		got := partitionRefs([]refSHA{
			{sha: "aaaa111", ref: "refs/heads/wipfoo"},
			{sha: "pppp333", ref: "refs/heads/parkedfoo"},
			{sha: "mmmm999", ref: "refs/heads/maintenance"},
		})
		if len(got.wips) != 0 || len(got.parked) != 0 || len(got.others) != 3 {
			t.Errorf("partitionRefs = %+v, want all three as others", got)
		}
		if got.mainSHA != "" {
			t.Errorf("mainSHA = %q, want empty: refs/heads/maintenance is not main", got.mainSHA)
		}
	})
}

// TestBranchNames checks the parked section's rows are named without their
// refs/heads/ prefix and ordered by name, not by whatever order the remote
// listed them in (STYLE D1).
func TestBranchNames(t *testing.T) {
	got := branchNames([]refSHA{
		{sha: "cccc333", ref: "refs/heads/parked/untriaged-20260920-090000"},
		{sha: "aaaa111", ref: "refs/heads/parked/untriaged-20260918-101500"},
		{sha: "bbbb222", ref: "refs/heads/parked/untriaged-20260919-120000"},
	})
	want := []string{
		"parked/untriaged-20260918-101500",
		"parked/untriaged-20260919-120000",
		"parked/untriaged-20260920-090000",
	}
	if len(got) != len(want) {
		t.Fatalf("branchNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("branchNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := branchNames(nil); len(got) != 0 {
		t.Errorf("branchNames(nil) = %v, want no names", got)
	}
}

// TestParseWipRef exercises the pure branch-name parser, including the
// malformed shapes it must reject.
func TestParseWipRef(t *testing.T) {
	cases := []struct {
		name       string
		sha, ref   string
		wantBranch string
		wantIssue  int
		wantErr    bool
	}{
		{name: "valid", sha: "abc123", ref: "refs/heads/wip/issue-636", wantBranch: "wip/issue-636", wantIssue: 636},
		{name: "single digit", sha: "abc123", ref: "refs/heads/wip/issue-1", wantBranch: "wip/issue-1", wantIssue: 1},
		{name: "not under refs/heads", sha: "abc123", ref: "refs/tags/wip/issue-1", wantErr: true},
		{name: "not a wip branch", sha: "abc123", ref: "refs/heads/main", wantErr: true},
		{name: "missing issue number", sha: "abc123", ref: "refs/heads/wip/issue-", wantErr: true},
		{name: "non-numeric issue", sha: "abc123", ref: "refs/heads/wip/issue-abc", wantErr: true},
		{name: "leading zero rejected", sha: "abc123", ref: "refs/heads/wip/issue-007", wantErr: true},
		{name: "zero rejected", sha: "abc123", ref: "refs/heads/wip/issue-0", wantErr: true},
		{name: "other wip-prefixed branch", sha: "abc123", ref: "refs/heads/wip/issue-42/extra", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseWipRef(c.sha, c.ref)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseWipRef(%q, %q) = %+v, want error", c.sha, c.ref, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWipRef(%q, %q) unexpected error: %v", c.sha, c.ref, err)
			}
			if got.branch != c.wantBranch || got.issue != c.wantIssue || got.sha != c.sha {
				t.Errorf("parseWipRef(%q, %q) = %+v, want branch=%s issue=%d sha=%s", c.sha, c.ref, got, c.wantBranch, c.wantIssue, c.sha)
			}
		})
	}
}

// TestParseLsRemote exercises the pure ls-remote output parser: no git
// call, just the text `git ls-remote --heads` would have produced.
func TestParseLsRemote(t *testing.T) {
	t.Run("multiple well-formed lines", func(t *testing.T) {
		out := "aaaa111\trefs/heads/wip/issue-1\n" +
			"bbbb222\trefs/heads/wip/issue-2\n"
		refs, errs := parseLsRemote(out)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(refs) != 2 {
			t.Fatalf("got %d refs, want 2: %+v", len(refs), refs)
		}
		if refs[0].sha != "aaaa111" || refs[0].ref != "refs/heads/wip/issue-1" {
			t.Errorf("refs[0] = %+v", refs[0])
		}
		if refs[1].sha != "bbbb222" || refs[1].ref != "refs/heads/wip/issue-2" {
			t.Errorf("refs[1] = %+v", refs[1])
		}
	})

	t.Run("empty output yields no refs and no errors", func(t *testing.T) {
		refs, errs := parseLsRemote("")
		if refs != nil || errs != nil {
			t.Fatalf("parseLsRemote(\"\") = %+v, %+v, want nil, nil", refs, errs)
		}
	})

	t.Run("line without a tab is reported, not silently dropped", func(t *testing.T) {
		out := "aaaa111\trefs/heads/wip/issue-1\n" +
			"not-tab-separated\n"
		refs, errs := parseLsRemote(out)
		if len(refs) != 1 {
			t.Fatalf("got %d refs, want 1 (the well-formed line): %+v", len(refs), refs)
		}
		if len(errs) != 1 {
			t.Fatalf("got %d errors, want 1 for the malformed line: %v", len(errs), errs)
		}
	})
}

// TestReadIssues exercises the JSON parsing against the exact shape `gh
// issue list --state all --json number,state,labels` produces.
func TestReadIssues(t *testing.T) {
	t.Run("parses number, state, and needs-replan label", func(t *testing.T) {
		in := `[
			{"number":636,"state":"OPEN","labels":[{"name":"ready"}]},
			{"number":300,"state":"CLOSED","labels":[]},
			{"number":301,"state":"open","labels":[{"name":"needs-replan"},{"name":"kind/bug"}]}
		]`
		issues, err := readIssues(strings.NewReader(in))
		if err != nil {
			t.Fatalf("readIssues: %v", err)
		}
		if len(issues) != 3 {
			t.Fatalf("got %d issues, want 3: %+v", len(issues), issues)
		}
		if got := issues[636]; got.closed || got.needsReplan {
			t.Errorf("issue 636 = %+v, want open, no needs-replan", got)
		}
		if got := issues[300]; !got.closed {
			t.Errorf("issue 300 = %+v, want closed", got)
		}
		if got := issues[301]; !got.needsReplan {
			t.Errorf("issue 301 = %+v, want needs-replan", got)
		}
		// state matching must be case-insensitive: "open" (lowercase) above
		// must not be mistaken for closed.
		if got := issues[301]; got.closed {
			t.Errorf("issue 301 = %+v, want open despite lowercase state", got)
		}
	})

	t.Run("empty stdin reports lease-only, not an error", func(t *testing.T) {
		issues, err := readIssues(strings.NewReader(""))
		if err != nil {
			t.Fatalf("readIssues(empty): unexpected error: %v", err)
		}
		if issues != nil {
			t.Fatalf("readIssues(empty) = %+v, want nil map", issues)
		}
	})

	t.Run("malformed JSON is an error", func(t *testing.T) {
		_, err := readIssues(strings.NewReader("{not valid json"))
		if err == nil {
			t.Fatalf("readIssues(malformed): want error, got nil")
		}
	})

	t.Run("an absent comments key is not an empty thread", func(t *testing.T) {
		in := `[
			{"number":1,"state":"OPEN","labels":[]},
			{"number":2,"state":"OPEN","labels":[],"comments":[]}
		]`
		issues, err := readIssues(strings.NewReader(in))
		if err != nil {
			t.Fatalf("readIssues: %v", err)
		}
		if got := issues[1]; got.commentsRead {
			t.Errorf("issue 1 = %+v, want commentsRead false: nobody supplied its comments", got)
		}
		if got := issues[2]; !got.commentsRead || got.heartbeat != nil {
			t.Errorf("issue 2 = %+v, want commentsRead true with no heartbeat", got)
		}
	})

	t.Run("the newest heartbeat comment dates the issue", func(t *testing.T) {
		in := `[{"number":3,"state":"OPEN","labels":[],"comments":[
			{"body":"RESUME: next action is the gate.","createdAt":"2026-08-26T06:00:00Z"},
			{"body":"TAKEOVER: resuming this issue.","createdAt":"2026-08-26T08:22:50Z"},
			{"body":"GROUNDING: no XSD rule is in scope.","createdAt":"2026-08-26T09:15:00Z"}
		]}]`
		issues, err := readIssues(strings.NewReader(in))
		if err != nil {
			t.Fatalf("readIssues: %v", err)
		}
		want := time.Date(2026, 8, 26, 8, 22, 50, 0, time.UTC)
		got := issues[3]
		if got.heartbeat == nil || !got.heartbeat.Equal(want) {
			t.Errorf("issue 3 heartbeat = %v, want the 08:22:50Z takeover (the newer grounding dates nothing)", got.heartbeat)
		}
	})

	t.Run("only TAKEOVER: comments count toward the takeover count", func(t *testing.T) {
		in := `[{"number":4,"state":"OPEN","labels":[],"comments":[
			{"body":"TAKEOVER: taking the claim.","createdAt":"2026-08-26T06:00:00Z"},
			{"body":"RESUME: picking my own work back up.","createdAt":"2026-08-26T07:00:00Z"},
			{"body":"  TAKEOVER: leading whitespace is not a marker change.","createdAt":"2026-08-26T08:00:00Z"},
			{"body":"GROUNDING: no XSD rule is in scope.","createdAt":"2026-08-26T09:00:00Z"},
			{"body":"I will TAKEOVER: this later.","createdAt":"2026-08-26T10:00:00Z"},
			{"body":"takeover: lowercase is not the marker.","createdAt":"2026-08-26T11:00:00Z"}
		]},
		{"number":5,"state":"OPEN","labels":[],"comments":[
			{"body":"RESUME: next action is the gate.","createdAt":"2026-08-26T06:00:00Z"}
		]},
		{"number":6,"state":"OPEN","labels":[]}]`
		issues, err := readIssues(strings.NewReader(in))
		if err != nil {
			t.Fatalf("readIssues: %v", err)
		}
		if got := issues[4].takeovers; got != 2 {
			t.Errorf("issue 4 takeovers = %d, want 2: only the two comments opening TAKEOVER: count", got)
		}
		if got := issues[5].takeovers; got != 0 {
			t.Errorf("issue 5 takeovers = %d, want 0: a RESUME: is not a takeover", got)
		}
		if got := issues[6].takeovers; got != 0 {
			t.Errorf("issue 6 takeovers = %d, want 0: its comments were never supplied", got)
		}
	})
}

// TestStatelessFeedIsDiscardedNotReadAsOpen is #1604: a reshape that drops
// `state` decoded every row as OPEN, so no branch could report RETIRED from
// a feed that looked supplied. readIssues must refuse the whole list with an
// errMissingState error — the error run hands to [renderReport] — and
// classify, handed what run looks up from that result, must print no
// RETIRED row. #8 carries needs-replan in both feeds, so a map kept despite
// the error would retire it from a list the tool refused.
func TestStatelessFeedIsDiscardedNotReadAsOpen(t *testing.T) {
	tip := fixedNow.Add(-time.Hour)
	for _, tc := range []struct {
		name, in, wantPrefix string
	}{
		{
			name: "no row carries state",
			in: `[{"number":8,"labels":[{"name":"needs-replan"}]},
				{"number":7,"labels":[]}]`,
			wantPrefix: `no row of the fed issue list carries a "state" field (first: #8)`,
		},
		{
			name: "some rows carry state",
			in: `[{"number":7,"state":"CLOSED","labels":[]},
				{"number":8,"labels":[{"name":"needs-replan"}]},
				{"number":9,"labels":[]}]`,
			wantPrefix: `issue #8 is the first of 2 of 3 fed rows carrying no "state" field`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues, err := readIssues(strings.NewReader(tc.in))
			if !errors.Is(err, errMissingState) {
				t.Fatalf("readIssues err = %v, want one wrapping errMissingState so run exits 2", err)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, tc.wantPrefix) || !strings.HasSuffix(msg, `per docs/ROUTINES.md's "Survey input"`) {
				t.Errorf("readIssues err = %q, want prefix %q and a pointer to Survey input", msg, tc.wantPrefix)
			}
			if issues != nil {
				t.Fatalf("readIssues = %+v, want nil map: the refused list must not reach classify", issues)
			}

			var rows []row
			for _, n := range []int{7, 8} {
				var issue *issueState
				if state, ok := issues[n]; ok {
					issue = &state
				}
				branch := fmt.Sprintf("wip/issue-%d", n)
				got, lease, reason := classify(branch, &tip, ancestryOwnCommits, diffNonEmpty, fixedNow, issue)
				rows = append(rows, row{issue: n, branch: branch, lease: lease, anc: ancestryOwnCommits, verdict: got, reason: reason})
			}
			var buf bytes.Buffer
			if err := renderTable(&buf, rows); err != nil {
				t.Fatalf("renderTable: %v", err)
			}
			out := buf.String()
			if strings.Contains(out, string(retired)) || strings.Count(out, "lease-only") != 2 {
				t.Errorf("want two lease-only rows and no RETIRED from a refused feed:\n%s", out)
			}
		})
	}

	t.Run("an empty list is not a state-less feed", func(t *testing.T) {
		issues, err := readIssues(strings.NewReader(`[]`))
		if err != nil || len(issues) != 0 {
			t.Errorf("readIssues([]) = %+v, %v; want no issues and no error", issues, err)
		}
	})
}

// TestIsHeartbeat pins the criterion Acceptance item 1 asked to be
// checkable from what a comment carries: the marker the body opens with,
// never the author it is presumed to have.
func TestIsHeartbeat(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{"RESUME: next action is the gate.", true},
		{"TAKEOVER: resuming this issue. `wip/issue-981` carries no commits.", true},
		{"\n  RESUME: leading whitespace is not a marker change.", true},
		{"GROUNDING: no XSD/XPath/F&O rule is in scope.", false},
		{"MASON: implementation account. Commits: abc1234.", false},
		{"ARBITER: ACCEPT.", false},
		{"CARTOGRAPHER (`/backlog`, 2026-08-25): promoted four rows.", false},
		{"I will RESUME: this later.", false},
		{"resume: lowercase is not the marker.", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isHeartbeat(c.body); got != c.want {
			t.Errorf("isHeartbeat(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

// TestNewestHeartbeat checks the newest heartbeat is taken by timestamp
// and that non-heartbeat comments never win, however recent they are.
func TestNewestHeartbeat(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("parsing fixture time %q: %v", s, err)
		}
		return ts
	}
	comments := []ghComment{
		{Body: "GROUNDING: posted last, dates nothing.", CreatedAt: at("2026-08-26T10:00:00Z")},
		{Body: "TAKEOVER: taking the claim.", CreatedAt: at("2026-08-26T08:22:50Z")},
		{Body: "RESUME: earlier heartbeat.", CreatedAt: at("2026-08-26T06:00:00Z")},
	}
	got := newestHeartbeat(comments)
	if got == nil || !got.Equal(at("2026-08-26T08:22:50Z")) {
		t.Errorf("newestHeartbeat = %v, want the 08:22:50Z takeover", got)
	}

	if got := newestHeartbeat([]ghComment{comments[0]}); got != nil {
		t.Errorf("newestHeartbeat(grounding only) = %v, want nil", got)
	}
	if got := newestHeartbeat(nil); got != nil {
		t.Errorf("newestHeartbeat(nil) = %v, want nil", got)
	}
}

// TestEmptyClaimLeaseFixtures is the regression named by #981, driven end
// to end from the input shape so both halves of the rule are exercised:
// which comment dates the lease, and the TTL arithmetic on it.
//
// The negative is #884's: a grounding posted 44 minutes before the read
// must not hold the claim for a further two hours. What it leaves behind
// is CLAIMED and not EXPIRED, because a thread with no heartbeat on it is
// also every claim's first minutes. The positive is #993's takeover
// comment, which holds the lease while it is inside the TTL and stops
// holding it once it is not.
func TestEmptyClaimLeaseFixtures(t *testing.T) {
	const read = "2026-08-26T09:00:00Z"
	now, err := time.Parse(time.RFC3339, read)
	if err != nil {
		t.Fatalf("parsing the read time: %v", err)
	}
	borrowed := now.Add(-10 * time.Hour) // main's tip, ten hours old

	cases := []struct {
		name string
		json string
		want verdict
	}{
		{
			// A grounding does not hold the lease — but it does not lapse one
			// either, and a thread whose only comment is a grounding is
			// exactly what /develop's step 3 leaves behind, so the claim
			// stays CLAIMED for a reader to settle (#981).
			name: "a grounding posted 44 minutes ago neither holds nor lapses the lease",
			json: `[{"number":884,"state":"OPEN","labels":[{"name":"ready"}],"comments":[
				{"body":"GROUNDING: no XSD/XPath/F&O rule is in scope.","createdAt":"2026-08-26T08:16:00Z"}
			]}]`,
			want: claimed,
		},
		{
			name: "a takeover comment posted 38 minutes ago holds it",
			json: `[{"number":884,"state":"OPEN","labels":[{"name":"ready"}],"comments":[
				{"body":"TAKEOVER: resuming this issue.","createdAt":"2026-08-26T08:22:00Z"},
				{"body":"GROUNDING: no XSD/XPath/F&O rule is in scope.","createdAt":"2026-08-26T08:16:00Z"}
			]}]`,
			want: live,
		},
		{
			name: "the same takeover comment four hours later does not",
			json: `[{"number":884,"state":"OPEN","labels":[{"name":"ready"}],"comments":[
				{"body":"TAKEOVER: resuming this issue.","createdAt":"2026-08-26T04:22:00Z"},
				{"body":"GROUNDING: no XSD/XPath/F&O rule is in scope.","createdAt":"2026-08-26T08:16:00Z"}
			]}]`,
			want: expired,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issues, err := readIssues(strings.NewReader(c.json))
			if err != nil {
				t.Fatalf("readIssues: %v", err)
			}
			state := issues[884]
			got, _, reason := classify("wip/issue-884", &borrowed, ancestryNoCommits, diffEmpty, now, &state)
			if got != c.want {
				t.Fatalf("verdict = %s, want %s (reason: %s)", got, c.want, reason)
			}
		})
	}
}

// TestSortRowsAndRenderTable exercises row ordering and rendering
// together: rows are built out of issue-number order on purpose, so a
// broken sort would show up as a non-ascending ISSUE column.
func TestSortRowsAndRenderTable(t *testing.T) {
	age := 5 * time.Minute
	rows := []row{
		{issue: 10, branch: "wip/issue-10", lease: &age, verdict: live, reason: "wip/issue-10: tip pushed 5m0s ago, within the 2h0m0s claim TTL"},
		{issue: 2, branch: "wip/issue-2", lease: nil, verdict: unknown, reason: "wip/issue-2: tip not fetched -- run `git fetch origin`"},
		{issue: 5, branch: "wip/issue-5", lease: &age, verdict: retired, reason: "wip/issue-5: issue #5 is closed"},
	}
	sortRows(rows)

	if rows[0].issue != 2 || rows[1].issue != 5 || rows[2].issue != 10 {
		t.Fatalf("sortRows did not order by issue number: %+v", rows)
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, rows); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	got := buf.String()

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("got %d lines, want %d (header + %d rows):\n%s", len(lines), len(rows)+1, len(rows), got)
	}
	if !strings.HasPrefix(lines[0], "ISSUE") {
		t.Fatalf("first line %q is not the header", lines[0])
	}
	// Row order in the rendered text must follow the sorted order (2, 5, 10),
	// not the order the rows were constructed in (10, 2, 5).
	if !strings.Contains(lines[1], "wip/issue-2") || !strings.Contains(lines[1], "UNKNOWN") {
		t.Errorf("row 1 = %q, want issue 2 (UNKNOWN)", lines[1])
	}
	// A row nothing dated keeps reading "unknown" in the age column — the
	// only place an unfetched tip shows on a RETIRED row, where the REASON
	// speaks about the issue instead (#809 rests on that cell).
	if got := strings.Fields(lines[1])[2]; got != "unknown" {
		t.Errorf("row 1 age cell = %q, want `unknown` for an unfetched tip: %q", got, lines[1])
	}
	if !strings.Contains(lines[2], "wip/issue-5") || !strings.Contains(lines[2], "RETIRED") {
		t.Errorf("row 2 = %q, want issue 5 (RETIRED)", lines[2])
	}
	if !strings.Contains(lines[3], "wip/issue-10") || !strings.Contains(lines[3], "LIVE") {
		t.Errorf("row 3 = %q, want issue 10 (LIVE)", lines[3])
	}
}

// TestRenderTableClaimedAge checks a CLAIMED row does not print a
// duration in LEASE AGE. The duration would be real and would belong to
// main, and printing it there is what got acted on (#722). The row is
// built the way run builds one — from classify's own return values — so
// the test pins the whole chain rather than a hand-assembled row classify
// could never produce.
func TestRenderTableClaimedAge(t *testing.T) {
	borrowedTip := fixedNow.Add(-10 * time.Hour)
	got, lease, reason := classify("wip/issue-1", &borrowedTip, ancestryNoCommits, diffEmpty, fixedNow, nil)
	if got != claimed {
		t.Fatalf("classify with a 10h borrowed tip = %s, want CLAIMED", got)
	}
	rows := []row{
		{issue: 1, branch: "wip/issue-1", lease: lease, anc: ancestryNoCommits, verdict: got, reason: reason},
	}
	var buf bytes.Buffer
	if err := renderTable(&buf, rows); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, "main's") {
		t.Errorf("CLAIMED row does not name whose tip age it is:\n%s", rendered)
	}
	if strings.Contains(rendered, "10h0m0s") {
		t.Errorf("CLAIMED row prints main's age as the branch's:\n%s", rendered)
	}
	if !strings.Contains(rendered, "CLAIMED") {
		t.Errorf("CLAIMED row does not carry the verdict:\n%s", rendered)
	}
}

// TestRenderTableRetiredBorrowedAge is the same guard on the RETIRED rows.
// Retirement changes the verdict, not whose clock the tip is, so a retired
// branch that never committed must not print main's age under LEASE AGE
// either (#809).
func TestRenderTableRetiredBorrowedAge(t *testing.T) {
	borrowedTip := fixedNow.Add(-10 * time.Hour)
	issue := &issueState{number: 1, closed: true}
	got, lease, reason := classify("wip/issue-1", &borrowedTip, ancestryNoCommits, diffEmpty, fixedNow, issue)
	if got != retired {
		t.Fatalf("classify with a closed issue = %s, want RETIRED", got)
	}
	if lease != nil {
		t.Errorf("retired zero-commit branch dated its lease %v; a borrowed tip dates nothing", *lease)
	}
	var buf bytes.Buffer
	if err := renderTable(&buf, []row{{issue: 1, branch: "wip/issue-1", lease: lease, anc: ancestryNoCommits, verdict: got, reason: reason}}); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	cell := strings.Fields(strings.Split(buf.String(), "\n")[1])[2]
	if cell != "main's" {
		t.Errorf("RETIRED zero-commit LEASE AGE cell = %q, want `main's`", cell)
	}
}

// TestRenderTableHeartbeatAge checks an empty claim the thread dated
// prints the HEARTBEAT's age, under a header that says LEASE AGE rather
// than TIP AGE — the two clocks differ on exactly these rows, and the one
// that decided the verdict is the one the column owes the reader.
func TestRenderTableHeartbeatAge(t *testing.T) {
	hb := 44 * time.Minute
	rows := []row{
		{issue: 1, branch: "wip/issue-1", lease: &hb, anc: ancestryNoCommits, verdict: live, reason: "wip/issue-1: no commits of its own; lease dated by its newest RESUME:/TAKEOVER: comment"},
	}
	var buf bytes.Buffer
	if err := renderTable(&buf, rows); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "LEASE AGE") {
		t.Errorf("header does not say whose age the column is:\n%s", got)
	}
	if !strings.Contains(got, "44m0s") {
		t.Errorf("heartbeat-dated row does not print the heartbeat's age:\n%s", got)
	}

	// An empty claim no heartbeat ever dated has no age of its own to
	// print. It stays CLAIMED and says whose the tip is, rather than
	// borrowing main's age as a duration.
	undated := []row{
		{issue: 2, branch: "wip/issue-2", lease: nil, anc: ancestryNoCommits, verdict: claimed, reason: "wip/issue-2: no commits of its own and no RESUME:/TAKEOVER: comment ever posted"},
	}
	buf.Reset()
	if err := renderTable(&buf, undated); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if cell := strings.Fields(strings.Split(buf.String(), "\n")[1])[2]; cell != "main's" {
		t.Errorf("undated empty claim age cell = %q, want `main's`", cell)
	}
}

// TestRenderTableDeterministic guards STYLE D1: rendering the same rows
// twice must produce byte-identical output.
func TestRenderTableDeterministic(t *testing.T) {
	age := 30 * time.Minute
	rows := []row{
		{issue: 1, branch: "wip/issue-1", lease: &age, verdict: live, reason: "wip/issue-1: tip pushed 30m0s ago, within the 2h0m0s claim TTL"},
	}

	var a, b bytes.Buffer
	if err := renderTable(&a, rows); err != nil {
		t.Fatalf("renderTable (a): %v", err)
	}
	if err := renderTable(&b, rows); err != nil {
		t.Fatalf("renderTable (b): %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("renderTable is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a.String(), b.String())
	}
}

// TestRenderParkedEmptyNamespaceIsStated is #1627 arm 1's regression: an
// empty parked/* namespace must print that it was surveyed and found
// empty. A section omitted for want of rows and a section reporting zero
// read identically to whoever quotes the number afterwards, which is how
// five PLAN.md stamps came to report a parked/* count from a refspec that
// never carried parked/*.
func TestRenderParkedEmptyNamespaceIsStated(t *testing.T) {
	var buf bytes.Buffer
	if err := renderParked(&buf, nil); err != nil {
		t.Fatalf("renderParked: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "PARKED BRANCHES") {
		t.Errorf("empty parked namespace printed no section at all:\n%q", got)
	}
	if !strings.Contains(got, "refs/heads/parked/*") {
		t.Errorf("empty parked section does not name the namespace it surveyed:\n%q", got)
	}
	if !strings.Contains(got, "none") {
		t.Errorf("empty parked section does not state the count it measured:\n%q", got)
	}
}

// TestRenderParkedRows checks each parked branch gets its own row saying
// it holds no lease, in the order the caller supplied (STYLE D1).
func TestRenderParkedRows(t *testing.T) {
	branches := []string{
		"parked/untriaged-20260918-101500",
		"parked/untriaged-20260920-090000",
	}
	var buf bytes.Buffer
	if err := renderParked(&buf, branches); err != nil {
		t.Fatalf("renderParked: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "none") {
		t.Errorf("parked section with two rows still reports none:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// A blank separator line, the heading, the column header, two rows.
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5 (blank + heading + header + 2 rows):\n%s", len(lines), got)
	}
	for i, branch := range branches {
		line := lines[i+3]
		if !strings.Contains(line, branch) {
			t.Errorf("row %d = %q, want branch %q in supplied order", i, line, branch)
		}
		if !strings.Contains(line, "no lease") {
			t.Errorf("row %d = %q, does not say the branch holds no lease", i, line)
		}
	}

	var again bytes.Buffer
	if err := renderParked(&again, branches); err != nil {
		t.Fatalf("renderParked (again): %v", err)
	}
	if again.String() != got {
		t.Errorf("renderParked is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", got, again.String())
	}
}

// TestRenderOtherEmptyNamespaceIsStated is the same guard on the section
// for heads outside both namespaces: surveyed-and-empty must not read as
// never-looked-at.
func TestRenderOtherEmptyNamespaceIsStated(t *testing.T) {
	var buf bytes.Buffer
	if err := renderOther(&buf, nil); err != nil {
		t.Fatalf("renderOther: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "OTHER BRANCHES") {
		t.Errorf("empty other-branch namespace printed no section at all:\n%q", got)
	}
	if !strings.Contains(got, "none") {
		t.Errorf("empty other-branch section does not state the count it measured:\n%q", got)
	}
}

// The three NOTE cells the other-branch section prints, written out here
// rather than read from the code, so a reworded note fails instead of
// following along — the same discipline remedy() applies to #1437's
// clause.
const (
	aheadNote  = "AHEAD OF MAIN -- carries commits main does not have; triage before deleting"
	mergedNote = "nothing main does not already have"
	unsureNote = "ahead/behind undecided -- run `git fetch origin`, or `git fetch --unshallow origin` in a shallow clone"
)

// TestRenderOtherFlagsAhead is #1627 arm 2's regression, on the shape the
// issue measured: claude/dazzling-cerf-u3xy2k stood ahead=1 behind=13,
// one commit main did not have, carrying a /backlog pass's whole log
// entry. A row ahead of main must read differently from a merged one at a
// glance, and an undecided count must read as neither.
//
// The counts are fixtures, not a live remote: a test reading origin would
// stop being able to fail the moment that ref is deleted.
func TestRenderOtherFlagsAhead(t *testing.T) {
	rows := []otherRow{
		{branch: "claude/dazzling-cerf-u3xy2k", counts: &aheadBehind{ahead: 1, behind: 13}},
		{branch: "claude/eloquent-cerf-39rk64", counts: &aheadBehind{ahead: 0, behind: 270}},
		{branch: "claude/never-fetched-000000", counts: nil},
	}
	var buf bytes.Buffer
	if err := renderOther(&buf, rows); err != nil {
		t.Fatalf("renderOther: %v", err)
	}
	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// A blank separator line, the heading, the column header, three rows.
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6 (blank + heading + header + 3 rows):\n%s", len(lines), got)
	}

	stranded, merged, unfetched := lines[3], lines[4], lines[5]
	if !strings.Contains(stranded, "claude/dazzling-cerf-u3xy2k") {
		t.Fatalf("row 1 = %q, want the ahead=1 branch (rows render in the supplied order)", stranded)
	}
	if fields := strings.Fields(stranded); fields[1] != "1" || fields[2] != "13" {
		t.Errorf("ahead/behind cells = %q, want ahead 1 behind 13", stranded)
	}
	if !strings.Contains(stranded, aheadNote) {
		t.Errorf("ahead=1 row = %q, does not flag the one state that is dangerous", stranded)
	}
	if strings.Contains(merged, aheadNote) {
		t.Errorf("ahead=0 row = %q, carries the ahead-of-main flag: the flag then distinguishes nothing", merged)
	}
	if fields := strings.Fields(merged); fields[1] != "0" || fields[2] != "270" {
		t.Errorf("ahead/behind cells = %q, want ahead 0 behind 270", merged)
	}
	if !strings.Contains(unfetched, unsureNote) {
		t.Errorf("uncounted row = %q, does not say the counts are undecided", unfetched)
	}
	if fields := strings.Fields(unfetched); fields[1] != "?" || fields[2] != "?" {
		t.Errorf("uncounted row = %q, prints counts where nothing counted", unfetched)
	}

	var again bytes.Buffer
	if err := renderOther(&again, rows); err != nil {
		t.Fatalf("renderOther (again): %v", err)
	}
	if again.String() != got {
		t.Errorf("renderOther is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", got, again.String())
	}
}

// TestOtherNoteNeverCallsAnUncountedRefMerged pins the posture the rest of
// this tool takes on evidence it does not have: "ahead=0" is the claim
// that nothing is stranded on the ref, and an unfetched tip supports no
// such claim.
func TestOtherNoteNeverCallsAnUncountedRefMerged(t *testing.T) {
	if got := otherNote(otherRow{branch: "claude/x", counts: nil}); got == mergedNote {
		t.Fatalf("otherNote on an uncounted ref = %q, the note that says nothing is at risk", got)
	}
	if got := otherNote(otherRow{branch: "claude/x", counts: &aheadBehind{ahead: 0, behind: 5}}); got != mergedNote {
		t.Errorf("otherNote on a counted ahead=0 ref = %q, want %q", got, mergedNote)
	}
	if got := otherNote(otherRow{branch: "claude/x", counts: &aheadBehind{ahead: 2, behind: 5}}); got != aheadNote {
		t.Errorf("otherNote on an ahead=2 ref = %q, want %q", got, aheadNote)
	}
}

// TestBacklogSubject pins the prefix match that tells a stranded /backlog
// pass from every other commit ahead of main (#1705). A recovery commit
// that mentions a backlog LOG entry is not a pass, and neither is a subject
// that merely contains the words.
func TestBacklogSubject(t *testing.T) {
	cases := []struct {
		name     string
		subjects []string
		want     string
	}{
		{name: "no commits", subjects: nil, want: ""},
		{
			name:     "a pass's own commit",
			subjects: []string{"meta: backlog 2026-09-24"},
			want:     "meta: backlog 2026-09-24",
		},
		{
			name:     "the newest pass, below a later commit",
			subjects: []string{"docs: fix a typo (#1)", "meta: backlog 2026-09-24", "meta: backlog 2026-09-21"},
			want:     "meta: backlog 2026-09-24",
		},
		{
			name:     "a recovery commit is not a pass",
			subjects: []string{"meta: recover stranded 2026-09-21 backlog LOG entry (#1649)"},
			want:     "",
		},
		{
			name:     "the words without the prefix",
			subjects: []string{"docs: meta: backlog 2026-09-24 notes", "meta: backlogs"},
			want:     "",
		},
	}
	for _, c := range cases {
		if got := backlogSubject(c.subjects); got != c.want {
			t.Errorf("%s: backlogSubject(%q) = %q, want %q", c.name, c.subjects, got, c.want)
		}
	}
}

// TestOtherNoteStrandedPass checks a ref carrying a /backlog pass reads
// differently from a plain ahead row, that a row without one keeps the
// AHEAD OF MAIN note, and that an uncounted row — which gitAheadBehind
// returns for every ref in a shallow checkout — reads as neither (#1705).
func TestOtherNoteStrandedPass(t *testing.T) {
	if got := otherNote(otherRow{branch: "claude/x", counts: nil}); got != unsureNote {
		t.Errorf("otherNote on an uncounted ref = %q, want %q", got, unsureNote)
	}
	const stranded = "STRANDED PASS -- `meta: backlog 2026-09-24` never reached main; recover its PLAN.md and LOG write-up before deleting"
	pass := otherRow{branch: "claude/dazzling-cerf-c1o1wx", counts: &aheadBehind{ahead: 1, behind: 12, backlogSubject: "meta: backlog 2026-09-24"}}
	if got := otherNote(pass); got != stranded {
		t.Errorf("otherNote on a stranded pass = %q, want %q", got, stranded)
	}
	plain := otherRow{branch: "claude/x", counts: &aheadBehind{ahead: 3, behind: 12}}
	if got := otherNote(plain); got != aheadNote {
		t.Errorf("otherNote on an ahead row with no pass = %q, want %q", got, aheadNote)
	}
}

// TestSortOtherRows checks the section's order is the branch name and not
// the order the remote listed its heads in (STYLE D1).
func TestSortOtherRows(t *testing.T) {
	rows := []otherRow{
		{branch: "claude/zebra"},
		{branch: "claude/alpha"},
		{branch: "meta/audit-2026-09-06"},
	}
	sortOtherRows(rows)
	want := []string{"claude/alpha", "claude/zebra", "meta/audit-2026-09-06"}
	for i := range want {
		if rows[i].branch != want[i] {
			t.Fatalf("sortOtherRows = %+v, want %v", rows, want)
		}
	}
}

// TestReportNamesEveryNamespace checks the whole report renderReport writes
// for a remote carrying nothing but main still tells a reader which
// namespaces were surveyed. That is the one claim a /backlog stamp quotes a
// count from (#1627).
func TestReportNamesEveryNamespace(t *testing.T) {
	var buf bytes.Buffer
	if err := renderReport(&buf, nil, nil, nil, bandSection{}, nil); err != nil {
		t.Fatalf("renderReport: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"ISSUE", "PARKED BRANCHES", "OTHER BRANCHES", "WORKING BAND", "wip/*", "parked/*"} {
		if !strings.Contains(got, want) {
			t.Errorf("report over an empty remote does not mention %q:\n%s", want, got)
		}
	}
}

// TestRenderReportSectionsThenFeedError pins renderReport's composition
// (#1652): all four sections, in order, and the error it was handed
// returned only after them — #1604's state-less feed still prints its
// report before main exits 2.
func TestRenderReportSectionsThenFeedError(t *testing.T) {
	errSentinel := errors.New("sentinel")
	var buf bytes.Buffer
	if err := renderReport(&buf, nil, nil, nil, bandSection{}, errSentinel); !errors.Is(err, errSentinel) {
		t.Fatalf("renderReport err = %v, want the feed error it was handed", err)
	}
	got := buf.String()
	prev := -1
	for _, heading := range []string{"ISSUE", "PARKED BRANCHES", "OTHER BRANCHES", "WORKING BAND"} {
		at := strings.Index(got, heading)
		if at <= prev {
			t.Fatalf("heading %q at %d, want present and after the previous section's (%d):\n%s", heading, at, prev, got)
		}
		prev = at
	}

	if err := renderReport(&bytes.Buffer{}, nil, nil, nil, bandSection{}, nil); err != nil {
		t.Errorf("renderReport with no feed error = %v, want nil", err)
	}
}

// TestFormatAge checks the rounding renderTable and classify's reason
// text both rely on.
func TestFormatAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{90 * time.Second, "2m0s"}, // rounds up to the nearest minute
		{2 * time.Hour, "2h0m0s"},
	}
	for _, c := range cases {
		if got := formatAge(c.d); got != c.want {
			t.Errorf("formatAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
