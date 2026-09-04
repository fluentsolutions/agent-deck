package session

import (
	"sort"
	"time"
)

// dividerNeverOpened captions the divider that separates sessions the user has
// actually touched from the ones they never have, in GroupViewLastInteraction.
const dividerNeverOpened = "never opened"

// dividerPinnedBottom captions that same divider when nothing below it is
// "never opened" — everything down there is pinned to the bottom by hand.
const dividerPinnedBottom = "pinned to bottom"

// LastInteractionAt returns when the user last deliberately touched this
// session: attaching to it, detaching from it, sending it a message, or
// starting/stopping/restarting it by hand. It is deliberately NOT a measure of
// how busy the agent inside the session is — output arriving, a status
// transition, or a heartbeat leave this timestamp alone.
//
// It reads Instance.LastAccessedAt, the field MarkAccessed writes. A zero value
// means "never touched" and sorts last, never first.
func LastInteractionAt(inst *Instance) time.Time {
	if inst == nil {
		return time.Time{}
	}
	return inst.LastAccessedAt
}

// moreRecentlyInteracted reports whether session a should sort before session b
// in the last-interaction order.
//
// The comparison is a strict, total order so the list never shuffles between
// two rebuilds of identical data:
//
//  1. last interaction, most recent first (a zero time — never touched — always
//     loses to any real timestamp);
//  2. creation time, newest first, for sessions touched in the same instant and
//     for the whole never-touched block;
//  3. title, then ID, both ascending, as the final deterministic fallback.
func moreRecentlyInteracted(a, b *Instance) bool {
	ta, tb := LastInteractionAt(a), LastInteractionAt(b)
	if !ta.Equal(tb) {
		// Zero (never touched) must lose to any recorded interaction.
		switch {
		case ta.IsZero():
			return false
		case tb.IsZero():
			return true
		}
		return ta.After(tb)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.ID < b.ID
}

// flattenSessionRow strips a session Item of everything that only makes sense
// inside the group tree, so it renders as a plain top-level row: no indent, no
// group-nesting connector, no root-group number badge. The list this mode
// produces has no group headers, so a row that kept Level 2 and a sub-session
// connector would render indented under nothing.
//
// Path is preserved: it is what group-scoped operations and the "move to group"
// flow read back off the selected row.
func flattenSessionRow(it Item) Item {
	it.Level = 0
	it.IsSubSession = false
	it.IsLastSubSession = false
	it.IsLastInGroup = false
	it.ParentIsLastInGroup = false
	it.RootGroupNum = 0
	return it
}

// SortByLastInteraction re-orders an already-flattened item list into a single
// group-free list of session rows, most recently interacted-with first.
//
// This is the GroupViewLastInteraction mode. Group headers are dropped: a
// recency order that still had to nest rows under their group could only sort
// within a group, which is not what the mode is for — the point is to find the
// session you were just in without knowing which group it lives in.
//
// Layout, top to bottom:
//
//	sessions still being created (placeholders; the user made them seconds ago)
//	sessions pinned to the top, by recency
//	every other session that has ever been touched, by recency
//	── divider ──
//	sessions never touched
//	sessions pinned to the bottom, by recency
//
// Pins keep the meaning they have in the other view modes: a pin-top session
// stays on top even if it has not been touched for a week, a pin-bottom session
// stays at the bottom even if it was touched a second ago.
//
// If either section would be empty the divider is omitted, matching
// PartitionByViewMode.
func SortByLastInteraction(items []Item) []Item {
	var (
		creating  []Item
		pinTop    []Item
		touched   []Item
		untouched []Item
		pinBottom []Item
		passthru  []Item
	)

	for _, it := range items {
		switch {
		case it.Type == ItemTypeGroup:
			// Dropped: this mode has no group headers.
			continue
		case it.Type == ItemTypeDivider:
			// Dropped: a divider from a previous partition must not survive.
			continue
		case it.Type == ItemTypeSession && it.Session == nil:
			// Still-creating placeholder. It has no Instance to sort on and the
			// user created it moments ago, so it belongs at the very top.
			creating = append(creating, flattenSessionRow(it))
		case it.Type == ItemTypeSession:
			row := flattenSessionRow(it)
			switch it.Session.Pin {
			case PinTop:
				pinTop = append(pinTop, row)
			case PinBottom:
				pinBottom = append(pinBottom, row)
			default:
				if LastInteractionAt(it.Session).IsZero() {
					untouched = append(untouched, row)
				} else {
					touched = append(touched, row)
				}
			}
		default:
			// Windows/remote/etc. Neither is present at this point in the
			// rebuild (both are injected later), but keep rather than drop them
			// so a future caller cannot lose rows here.
			passthru = append(passthru, it)
		}
	}

	byRecency := func(rows []Item) {
		sort.SliceStable(rows, func(i, j int) bool {
			return moreRecentlyInteracted(rows[i].Session, rows[j].Session)
		})
	}
	byRecency(pinTop)
	byRecency(touched)
	byRecency(untouched)
	byRecency(pinBottom)

	top := make([]Item, 0, len(creating)+len(pinTop)+len(touched))
	top = append(top, creating...)
	top = append(top, pinTop...)
	top = append(top, touched...)

	bottom := make([]Item, 0, len(untouched)+len(pinBottom))
	bottom = append(bottom, untouched...)
	bottom = append(bottom, pinBottom...)

	out := make([]Item, 0, len(top)+1+len(bottom)+len(passthru))
	if len(top) == 0 || len(bottom) == 0 {
		out = append(out, top...)
		out = append(out, bottom...)
	} else {
		label := dividerNeverOpened
		if len(untouched) == 0 {
			label = dividerPinnedBottom
		}
		out = append(out, top...)
		out = append(out, Item{Type: ItemTypeDivider, DividerLabel: label})
		out = append(out, bottom...)
	}
	out = append(out, passthru...)

	markLastRowOfEachSection(out)
	return out
}

// markLastRowOfEachSection sets IsLastInGroup on the final session row of each
// divider-delimited section. A session row always renders a tree connector, so
// without this every row in the flat list would draw "├─" and the list would
// never close with a "└─".
func markLastRowOfEachSection(items []Item) {
	lastSession := -1
	flush := func() {
		if lastSession >= 0 {
			items[lastSession].IsLastInGroup = true
		}
		lastSession = -1
	}
	for i := range items {
		switch items[i].Type {
		case ItemTypeDivider:
			flush()
		case ItemTypeSession:
			lastSession = i
		}
	}
	flush()
}
