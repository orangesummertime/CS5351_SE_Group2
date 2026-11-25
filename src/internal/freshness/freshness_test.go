package freshness

import (
	"errors"
	"testing"
)

// nfr05Cases is the frozen NFR-05 suite: at least 20 fixed cases covering
// update, delete, reopen-equivalent reassessment, concurrent-style mid-change
// snapshots, and negatives. The overshoot metric is the number of cases that
// still display as currently valid when they must not.
type nfr05Case struct {
	id        string
	category  string // update, delete, reopen, concurrent, negative, stable
	captured  Snapshot
	current   Current
	lookupErr error
	want      Status
	// mustNotDisplayCurrent is true for every case that is not a genuine match.
	mustNotDisplayCurrent bool
}

func nfr05Cases() []nfr05Case {
	snap := Snapshot{ContentID: "rss:article-1", ContentRevision: 2, ContentHash: "aaa111"}
	live := Current{Exists: true, Revision: 2, Hash: "aaa111"}
	return []nfr05Case{
		// Stable / positive: genuine current evidence may display as valid.
		{id: "E-NFR05-01", category: "stable", captured: snap, current: live, want: StatusCurrent, mustNotDisplayCurrent: false},
		// Update: hash changes at the same or later revision.
		{id: "E-NFR05-02", category: "update", captured: snap, current: Current{Exists: true, Revision: 3, Hash: "bbb222"}, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-03", category: "update", captured: snap, current: Current{Exists: true, Revision: 2, Hash: "bbb222"}, want: StatusExpired, mustNotDisplayCurrent: true},
		// Update: revision bumps even when the hash is unchanged (Save always increments).
		{id: "E-NFR05-04", category: "update", captured: snap, current: Current{Exists: true, Revision: 3, Hash: "aaa111"}, want: StatusExpired, mustNotDisplayCurrent: true},
		// Update: larger jump in revision and hash.
		{id: "E-NFR05-05", category: "update", captured: snap, current: Current{Exists: true, Revision: 9, Hash: "ccc333"}, want: StatusExpired, mustNotDisplayCurrent: true},
		// Delete / missing content.
		{id: "E-NFR05-06", category: "delete", captured: snap, current: Current{Exists: false}, want: StatusUnavailable, mustNotDisplayCurrent: true},
		{id: "E-NFR05-07", category: "delete", captured: Snapshot{ContentID: "rss:gone", ContentRevision: 1, ContentHash: "ddd"}, current: Current{}, want: StatusUnavailable, mustNotDisplayCurrent: true},
		// Reopen-equivalent: assess the stored snapshot again after an update
		// or delete; status must follow the live row, not the stored label.
		{id: "E-NFR05-08", category: "reopen", captured: snap, current: Current{Exists: true, Revision: 4, Hash: "eee"}, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-09", category: "reopen", captured: snap, current: Current{Exists: false}, want: StatusUnavailable, mustNotDisplayCurrent: true},
		{id: "E-NFR05-10", category: "reopen", captured: snap, current: live, want: StatusCurrent, mustNotDisplayCurrent: false},
		// Concurrent-style: a snapshot taken before a racing save must expire
		// once the later current row is observed.
		{id: "E-NFR05-11", category: "concurrent", captured: snap, current: Current{Exists: true, Revision: 3, Hash: "race-hash"}, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-12", category: "concurrent", captured: snap, current: Current{Exists: false}, want: StatusUnavailable, mustNotDisplayCurrent: true},
		// Legacy citations that only have C's SourceVersion.
		{id: "E-NFR05-13", category: "update", captured: Snapshot{ContentID: "rss:article-1", SourceVersion: "content-revision:2"}, current: Current{Exists: true, Revision: 3, Hash: "aaa111"}, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-14", category: "stable", captured: Snapshot{ContentID: "rss:article-1", SourceVersion: "content-revision:2"}, current: Current{Exists: true, Revision: 2, Hash: "aaa111"}, want: StatusCurrent, mustNotDisplayCurrent: false},
		// Negative: incomplete or forged snapshots must not display as current.
		{id: "E-NFR05-15", category: "negative", captured: Snapshot{ContentID: "rss:article-1"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-16", category: "negative", captured: Snapshot{ContentID: "rss:article-1", ContentRevision: 99, ContentHash: "forged"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-17", category: "negative", captured: Snapshot{ContentID: "rss:article-1", SourceVersion: "not-a-revision"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
		{id: "E-NFR05-18", category: "negative", captured: snap, current: live, lookupErr: errors.New("sqlite busy"), want: StatusUnavailable, mustNotDisplayCurrent: true},
		{id: "E-NFR05-19", category: "negative", captured: Snapshot{ContentID: "rss:article-1", ContentRevision: 2, ContentHash: "AAA111"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
		// Empty captured hash with matching revision is current only when no
		// hash was stored; a present but different hash still expires.
		{id: "E-NFR05-20", category: "update", captured: Snapshot{ContentID: "rss:article-1", ContentRevision: 2, ContentHash: "other"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
		// Extra cases beyond the required 20 for reopen after no-op and delete
		// of a legacy row.
		{id: "E-NFR05-21", category: "delete", captured: Snapshot{ContentID: "rss:legacy", SourceVersion: "content-revision:1"}, current: Current{Exists: false}, want: StatusUnavailable, mustNotDisplayCurrent: true},
		{id: "E-NFR05-22", category: "negative", captured: Snapshot{ContentID: "rss:article-1", SourceVersion: "content-revision:0"}, current: live, want: StatusExpired, mustNotDisplayCurrent: true},
	}
}

func TestNFR05FrozenCasesDisplayNoStaleCurrent(t *testing.T) {
	cases := nfr05Cases()
	if len(cases) < 20 {
		t.Fatalf("frozen set has %d cases, need at least 20", len(cases))
	}
	var seenUpdate, seenDelete, seenReopen, seenConcurrent bool
	overshoot := 0
	for _, tc := range cases {
		got := Assess(tc.captured, tc.current, tc.lookupErr)
		if got != tc.want {
			t.Errorf("%s (%s): Assess = %q, want %q", tc.id, tc.category, got, tc.want)
		}
		displayed := DisplayedAsCurrentlyValid(got)
		if tc.mustNotDisplayCurrent && displayed {
			overshoot++
			t.Errorf("%s (%s): stale evidence displayed as currently valid", tc.id, tc.category)
		}
		if !tc.mustNotDisplayCurrent && !displayed {
			t.Errorf("%s (%s): genuine current evidence was not displayed as valid", tc.id, tc.category)
		}
		switch tc.category {
		case "update":
			seenUpdate = true
		case "delete":
			seenDelete = true
		case "reopen":
			seenReopen = true
		case "concurrent":
			seenConcurrent = true
		}
	}
	if overshoot != 0 {
		t.Fatalf("NFR-05 overshoot = %d, want 0", overshoot)
	}
	if !seenUpdate || !seenDelete || !seenReopen || !seenConcurrent {
		t.Fatalf("suite must cover update, delete, reopen and concurrent; got update=%v delete=%v reopen=%v concurrent=%v", seenUpdate, seenDelete, seenReopen, seenConcurrent)
	}
}

func TestParseRevision(t *testing.T) {
	if got := ParseRevision("content-revision:7"); got != 7 {
		t.Fatalf("got %d", got)
	}
	if got := ParseRevision("content-revision:0"); got != 0 {
		t.Fatalf("zero should be rejected, got %d", got)
	}
	if got := ParseRevision("v2"); got != 0 {
		t.Fatalf("unrelated version should be 0, got %d", got)
	}
}

func TestWorstPrefersUnavailableOverExpiredAndCurrent(t *testing.T) {
	if got := Worst(StatusCurrent, StatusExpired); got != StatusExpired {
		t.Fatalf("got %q", got)
	}
	if got := Worst(StatusExpired, StatusUnavailable); got != StatusUnavailable {
		t.Fatalf("got %q", got)
	}
	if got := Worst("", StatusCurrent); got != StatusCurrent {
		t.Fatalf("got %q", got)
	}
}

// Negative: a client-supplied "current" label is irrelevant; only Assess decides.
func TestETNegativeForgedCurrentLabelIsIgnored(t *testing.T) {
	captured := Snapshot{ContentID: "rss:article-1", ContentRevision: 1, ContentHash: "old"}
	current := Current{Exists: true, Revision: 2, Hash: "new"}
	if got := Assess(captured, current, nil); DisplayedAsCurrentlyValid(got) {
		t.Fatal("updated content still displayed as currently valid")
	}
}
