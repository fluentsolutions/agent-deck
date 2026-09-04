package session

import (
	"testing"
	"time"
)

// grpHeader builds a group header row at the nesting level implied by its path.
func grpHeader(path string) Item {
	return Item{Type: ItemTypeGroup, Path: path, Level: GetGroupLevel(path)}
}

// grpSession builds a top-level session row inside the given group.
func grpSession(path, id string, lastAccessed, createdAt time.Time) Item {
	return Item{
		Type:  ItemTypeSession,
		Level: GetGroupLevel(path) + 1,
		Path:  path,
		Session: &Instance{
			ID: id, Title: id,
			LastAccessedAt: lastAccessed,
			CreatedAt:      createdAt,
		},
	}
}

// grpSubSession builds a sub-session row hanging off the preceding session.
func grpSubSession(path, id, parentID string, lastAccessed time.Time) Item {
	return Item{
		Type:         ItemTypeSession,
		Level:        GetGroupLevel(path) + 2,
		Path:         path,
		IsSubSession: true,
		Session: &Instance{
			ID: id, Title: id,
			ParentSessionID: parentID,
			LastAccessedAt:  lastAccessed,
		},
	}
}

// layout renders the ordered result as "group:<path>" / session id lines, so a
// failure shows the shape of the list rather than a diff of structs.
func layout(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch it.Type {
		case ItemTypeGroup:
			out = append(out, "group:"+it.Path)
		case ItemTypeSession:
			if it.Session != nil {
				out = append(out, it.Session.ID)
			}
		}
	}
	return out
}

func TestSortGroupsByLastInteraction_GroupWithNewestTouchFloatsUp(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "a1", now.Add(-5*time.Hour), now),
		grpSession("alpha", "a2", now.Add(-9*time.Hour), now),
		grpHeader("beta"),
		grpSession("beta", "b1", now.Add(-1*time.Minute), now),
		grpSession("beta", "b2", now.Add(-8*time.Hour), now),
	}

	got := layout(SortGroupsByLastInteraction(items))
	// beta holds the most recent touch (b1), so the whole group rises, and
	// within each group the newer session leads.
	want := []string{"group:beta", "b1", "b2", "group:alpha", "a1", "a2"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_NestedChildLiftsItsParent(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "a1", now.Add(-2*time.Hour), now),
		grpHeader("beta"),
		grpSession("beta", "b1", now.Add(-6*time.Hour), now),
		grpHeader("beta/deep"),
		grpSession("beta/deep", "d1", now.Add(-1*time.Minute), now),
	}

	got := layout(SortGroupsByLastInteraction(items))
	// The freshest touch is inside beta/deep, so beta outranks alpha even
	// though beta's own session is older than alpha's. The child still renders
	// under its parent, and beta's own sessions still precede it.
	want := []string{"group:beta", "b1", "group:beta/deep", "d1", "group:alpha", "a1"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_HierarchyIsNeverBroken(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	// The child group is the stalest thing on the board; it must still render
	// under its parent rather than sinking below an unrelated group.
	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "a1", now, now),
		grpHeader("alpha/old"),
		grpSession("alpha/old", "o1", now.Add(-500*time.Hour), now),
		grpHeader("beta"),
		grpSession("beta", "b1", now.Add(-1*time.Hour), now),
	}

	out := SortGroupsByLastInteraction(items)
	got := layout(out)
	want := []string{"group:alpha", "a1", "group:alpha/old", "o1", "group:beta", "b1"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_SubSessionsTravelWithTheirParent(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "parent-old", now.Add(-9*time.Hour), now),
		grpSubSession("alpha", "child-of-old", "parent-old", now.Add(-1*time.Minute)),
		grpSession("alpha", "parent-mid", now.Add(-2*time.Hour), now),
	}

	got := layout(SortGroupsByLastInteraction(items))
	// The child carries the freshest touch, so its parent block leads — and the
	// child stays attached to it rather than being sorted on its own.
	want := []string{"group:alpha", "parent-old", "child-of-old", "parent-mid"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_IgnoresAgentActivity(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	busy := grpSession("alpha", "busy-agent", now.Add(-48*time.Hour), now)
	busy.Session.Status = StatusRunning
	quiet := grpSession("alpha", "user-just-opened", now.Add(-time.Minute), now)
	quiet.Session.Status = StatusStopped

	got := layout(SortGroupsByLastInteraction([]Item{grpHeader("alpha"), busy, quiet}))
	want := []string{"group:alpha", "user-just-opened", "busy-agent"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v (status must not lift a row)", got, want)
	}
}

func TestSortGroupsByLastInteraction_NeverTouchedSortLast(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	items := []Item{
		grpHeader("never"),
		grpSession("never", "n1", time.Time{}, now),
		grpHeader("touched"),
		grpSession("touched", "t1", time.Time{}, now),
		grpSession("touched", "t2", now.Add(-3*time.Hour), now),
	}

	got := layout(SortGroupsByLastInteraction(items))
	// The touched group leads; inside it the touched session leads; the
	// never-touched group sinks but keeps its rows.
	want := []string{"group:touched", "t2", "t1", "group:never", "n1"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_PinsStillWinInsideAGroup(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	pinTop := grpSession("alpha", "pin-top", now.Add(-100*time.Hour), now)
	pinTop.Session.Pin = PinTop
	pinBottom := grpSession("alpha", "pin-bottom", now, now)
	pinBottom.Session.Pin = PinBottom

	got := layout(SortGroupsByLastInteraction([]Item{
		grpHeader("alpha"),
		grpSession("alpha", "normal", now.Add(-time.Minute), now),
		pinBottom,
		pinTop,
	}))
	want := []string{"group:alpha", "pin-top", "normal", "pin-bottom"}
	if !sameOrder(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
}

func TestSortGroupsByLastInteraction_RewritesTreeConnectorFlags(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	// "first" is stalest and will move to the end of the group, so it must
	// acquire IsLastInGroup and "second" must lose it.
	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "first", now.Add(-9*time.Hour), now),
		grpSession("alpha", "second", now, now),
	}
	items[2].IsLastInGroup = true

	out := SortGroupsByLastInteraction(items)
	flags := map[string]bool{}
	for _, it := range out {
		if it.Type == ItemTypeSession && it.Session != nil {
			flags[it.Session.ID] = it.IsLastInGroup
		}
	}
	if flags["second"] {
		t.Error("'second' moved off the end of the group but kept IsLastInGroup")
	}
	if !flags["first"] {
		t.Error("'first' is now the last row of its group but IsLastInGroup was not set")
	}
}

func TestSortGroupsByLastInteraction_DropsAStaleDivider(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	items := []Item{
		grpHeader("alpha"),
		grpSession("alpha", "a1", now, now),
		{Type: ItemTypeDivider, DividerLabel: "idle / done"},
		grpSession("alpha", "a2", now.Add(-time.Hour), now),
	}

	for _, it := range SortGroupsByLastInteraction(items) {
		if it.Type == ItemTypeDivider {
			t.Fatal("a divider from a previous partition survived the grouped sort")
		}
	}
}

func TestGroupViewLastInteractionGrouped_IsInTheHotkeyCycle(t *testing.T) {
	if GroupViewModeCount != 5 {
		t.Fatalf("GroupViewModeCount = %d, want 5", GroupViewModeCount)
	}
	if got := GroupViewLastInteractionGrouped.Label(); got != "Last interaction (grouped)" {
		t.Errorf("label = %q, want %q", got, "Last interaction (grouped)")
	}
	// The two recency modes must be adjacent in the cycle, and cycling must
	// still return to Normal.
	if GroupViewLastInteractionGrouped != GroupViewLastInteraction+1 {
		t.Errorf("grouped mode (%d) should follow flat mode (%d) in the cycle",
			GroupViewLastInteractionGrouped, GroupViewLastInteraction)
	}
	mode := GroupViewNormal
	for i := 0; i < GroupViewModeCount; i++ {
		mode = GroupViewMode((int(mode) + 1) % GroupViewModeCount)
	}
	if mode != GroupViewNormal {
		t.Errorf("cycling %d times ended at %d, want Normal", GroupViewModeCount, mode)
	}
}
