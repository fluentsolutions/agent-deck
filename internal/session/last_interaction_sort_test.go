package session

import (
	"testing"
	"time"
)

// mkSession builds a session row for the sort tests. lastAccessed is the
// "user touched it then" clock; the zero time means never touched.
func mkSession(id string, lastAccessed, createdAt time.Time) Item {
	return Item{
		Type:  ItemTypeSession,
		Level: 1,
		Path:  "group",
		Session: &Instance{
			ID:             id,
			Title:          id,
			LastAccessedAt: lastAccessed,
			CreatedAt:      createdAt,
		},
	}
}

// sameOrder reports whether two ordered ID lists match exactly.
func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sessionIDs lists the session rows of a sorted list in order, using "──" to
// mark the divider so section boundaries are visible in test failures.
func sessionIDs(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch it.Type {
		case ItemTypeSession:
			if it.Session == nil {
				out = append(out, "<creating>")
				continue
			}
			out = append(out, it.Session.ID)
		case ItemTypeDivider:
			out = append(out, "──")
		}
	}
	return out
}

func TestSortByLastInteraction_MostRecentlyTouchedFirst(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	created := now.Add(-72 * time.Hour)

	items := []Item{
		{Type: ItemTypeGroup, Path: "group"},
		mkSession("stale", now.Add(-3*time.Hour), created),
		mkSession("freshest", now.Add(-1*time.Minute), created),
		mkSession("middling", now.Add(-30*time.Minute), created),
	}

	got := sessionIDs(SortByLastInteraction(items))
	want := []string{"freshest", "middling", "stale"}
	if !sameOrder(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortByLastInteraction_NeverTouchedSortLastBehindDivider(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	items := []Item{
		mkSession("never-b", time.Time{}, now.Add(-2*time.Hour)),
		mkSession("touched", now.Add(-5*time.Hour), now.Add(-90*time.Hour)),
		mkSession("never-a", time.Time{}, now.Add(-1*time.Hour)),
	}

	got := sessionIDs(SortByLastInteraction(items))
	// Touched first; then the divider; then the never-touched block, newest
	// session first (the CreatedAt tie-break).
	want := []string{"touched", "──", "never-a", "never-b"}
	if !sameOrder(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortByLastInteraction_DividerLabelledNeverOpened(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	items := []Item{
		mkSession("touched", now, now),
		mkSession("never", time.Time{}, now),
	}

	out := SortByLastInteraction(items)
	var label string
	for _, it := range out {
		if it.Type == ItemTypeDivider {
			label = it.DividerLabel
		}
	}
	if label != dividerNeverOpened {
		t.Fatalf("divider label = %q, want %q", label, dividerNeverOpened)
	}
}

func TestSortByLastInteraction_NoDividerWhenEitherSectionEmpty(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	allTouched := SortByLastInteraction([]Item{
		mkSession("a", now, now),
		mkSession("b", now.Add(-time.Hour), now),
	})
	if got := sessionIDs(allTouched); !sameOrder(got, []string{"a", "b"}) {
		t.Fatalf("all-touched order = %v, want [a b] with no divider", got)
	}

	noneTouched := SortByLastInteraction([]Item{
		mkSession("a", time.Time{}, now),
		mkSession("b", time.Time{}, now.Add(-time.Hour)),
	})
	if got := sessionIDs(noneTouched); !sameOrder(got, []string{"a", "b"}) {
		t.Fatalf("none-touched order = %v, want [a b] with no divider", got)
	}
}

func TestSortByLastInteraction_PinsOverrideRecency(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	pinnedTopStale := mkSession("pin-top", now.Add(-100*time.Hour), now)
	pinnedTopStale.Session.Pin = PinTop
	pinnedBottomFresh := mkSession("pin-bottom", now, now)
	pinnedBottomFresh.Session.Pin = PinBottom

	items := []Item{
		mkSession("normal", now.Add(-time.Minute), now),
		pinnedBottomFresh,
		pinnedTopStale,
	}

	got := sessionIDs(SortByLastInteraction(items))
	want := []string{"pin-top", "normal", "──", "pin-bottom"}
	if !sameOrder(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortByLastInteraction_DividerSaysPinnedBottomWhenNothingIsUntouched(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	pinned := mkSession("pin-bottom", now.Add(-time.Hour), now)
	pinned.Session.Pin = PinBottom

	out := SortByLastInteraction([]Item{mkSession("normal", now, now), pinned})
	var label string
	for _, it := range out {
		if it.Type == ItemTypeDivider {
			label = it.DividerLabel
		}
	}
	if label != dividerPinnedBottom {
		t.Fatalf("divider label = %q, want %q", label, dividerPinnedBottom)
	}
}

func TestSortByLastInteraction_TieBreakIsDeterministic(t *testing.T) {
	same := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	createdOld := same.Add(-48 * time.Hour)
	createdNew := same.Add(-1 * time.Hour)

	// Equal LastAccessedAt -> newest CreatedAt first.
	byCreated := sessionIDs(SortByLastInteraction([]Item{
		mkSession("older", same, createdOld),
		mkSession("newer", same, createdNew),
	}))
	if !sameOrder(byCreated, []string{"newer", "older"}) {
		t.Fatalf("CreatedAt tie-break = %v, want [newer older]", byCreated)
	}

	// Equal LastAccessedAt and CreatedAt -> title ascending.
	byTitle := sessionIDs(SortByLastInteraction([]Item{
		mkSession("zebra", same, createdOld),
		mkSession("alpha", same, createdOld),
	}))
	if !sameOrder(byTitle, []string{"alpha", "zebra"}) {
		t.Fatalf("title tie-break = %v, want [alpha zebra]", byTitle)
	}
}

func TestSortByLastInteraction_DropsGroupHeadersAndFlattensRows(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	sub := mkSession("sub", now.Add(-time.Minute), now)
	sub.Level = 3
	sub.IsSubSession = true
	sub.IsLastSubSession = true
	sub.RootGroupNum = 2

	items := []Item{
		{Type: ItemTypeGroup, Path: "group", Level: 0},
		{Type: ItemTypeGroup, Path: "group/nested", Level: 1},
		mkSession("top", now, now),
		sub,
	}

	out := SortByLastInteraction(items)
	for _, it := range out {
		if it.Type == ItemTypeGroup {
			t.Fatalf("group header survived the flat sort: %+v", it)
		}
	}
	for _, it := range out {
		if it.Type != ItemTypeSession {
			continue
		}
		if it.Level != 0 {
			t.Errorf("session %s has Level %d, want 0 (flat list has no nesting)", it.Session.ID, it.Level)
		}
		if it.IsSubSession || it.IsLastSubSession || it.RootGroupNum != 0 {
			t.Errorf("session %s kept group-tree flags: %+v", it.Session.ID, it)
		}
	}
}

func TestSortByLastInteraction_ClosesEachSectionWithALastRow(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	out := SortByLastInteraction([]Item{
		mkSession("touched-a", now, now),
		mkSession("touched-b", now.Add(-time.Hour), now),
		mkSession("never-a", time.Time{}, now),
		mkSession("never-b", time.Time{}, now.Add(-time.Hour)),
	})

	var lastFlagged []string
	for _, it := range out {
		if it.Type == ItemTypeSession && it.IsLastInGroup {
			lastFlagged = append(lastFlagged, it.Session.ID)
		}
	}
	// Exactly one closing row per section: the last touched row and the last
	// never-touched row. Anything else draws stray tree connectors.
	if !sameOrder(lastFlagged, []string{"touched-b", "never-b"}) {
		t.Fatalf("IsLastInGroup set on %v, want [touched-b never-b]", lastFlagged)
	}
}

func TestSortByLastInteraction_CreatingPlaceholdersGoFirst(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	placeholder := Item{Type: ItemTypeSession, CreatingID: "new", CreatingTitle: "new"}

	got := sessionIDs(SortByLastInteraction([]Item{
		mkSession("touched", now, now),
		placeholder,
	}))
	if !sameOrder(got, []string{"<creating>", "touched"}) {
		t.Fatalf("order = %v, want [<creating> touched]", got)
	}
}

func TestSortByLastInteraction_IgnoresAgentActivity(t *testing.T) {
	// The point of the feature: a session the agent is hammering, that the user
	// has not touched in two days, must stay below one the user opened a minute
	// ago. Nothing in the sort reads status or any activity clock, so a
	// "running" session with an old LastAccessedAt stays put.
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	busy := mkSession("busy-agent", now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	busy.Session.Status = StatusRunning
	quiet := mkSession("user-just-opened", now.Add(-time.Minute), now.Add(-500*time.Hour))
	quiet.Session.Status = StatusStopped

	got := sessionIDs(SortByLastInteraction([]Item{busy, quiet}))
	if !sameOrder(got, []string{"user-just-opened", "busy-agent"}) {
		t.Fatalf("order = %v, want [user-just-opened busy-agent]", got)
	}
}

func TestGroupViewLastInteraction_IsInTheHotkeyCycle(t *testing.T) {
	// Cycling with "t" is (mode+1)%GroupViewModeCount; the new mode must be
	// reachable and must not have displaced the existing ones.
	if GroupViewModeCount != 4 {
		t.Fatalf("GroupViewModeCount = %d, want 4", GroupViewModeCount)
	}
	want := map[GroupViewMode]string{
		GroupViewNormal:          "Normal",
		GroupViewActiveTop:       "Active on top",
		GroupViewPopulatedTop:    "Populated on top",
		GroupViewLastInteraction: "Last interaction",
	}
	for mode, label := range want {
		if got := mode.Label(); got != label {
			t.Errorf("mode %d label = %q, want %q", mode, got, label)
		}
	}
	seen := map[GroupViewMode]bool{}
	mode := GroupViewNormal
	for i := 0; i < GroupViewModeCount; i++ {
		seen[mode] = true
		mode = GroupViewMode((int(mode) + 1) % GroupViewModeCount)
	}
	if mode != GroupViewNormal {
		t.Errorf("cycling %d times did not return to Normal, ended at %d", GroupViewModeCount, mode)
	}
	if len(seen) != GroupViewModeCount {
		t.Errorf("cycle visited %d modes, want %d", len(seen), GroupViewModeCount)
	}
}

func TestLastInteractionAt_ReadsLastAccessedAt(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if got := LastInteractionAt(&Instance{LastAccessedAt: now}); !got.Equal(now) {
		t.Errorf("LastInteractionAt = %v, want %v", got, now)
	}
	if got := LastInteractionAt(&Instance{}); !got.IsZero() {
		t.Errorf("LastInteractionAt on an untouched session = %v, want zero", got)
	}
	if got := LastInteractionAt(nil); !got.IsZero() {
		t.Errorf("LastInteractionAt(nil) = %v, want zero", got)
	}
}
