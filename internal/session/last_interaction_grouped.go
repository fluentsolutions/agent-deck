package session

import (
	"sort"
	"strings"
	"time"
)

// sessionBlock is one top-level session row plus the sub-session rows that
// render underneath it. Sub-sessions must travel with their parent — sorting
// individual rows would strand a child under whichever session happened to land
// above it — so recency ordering moves blocks, never rows.
type sessionBlock struct {
	rows []Item
	idx  int // original position, for a stable tie-break
}

// head returns the block's top-level session (the row the sub-sessions hang
// off). Never nil for a well-formed block.
func (b sessionBlock) head() *Instance {
	if len(b.rows) == 0 {
		return nil
	}
	return b.rows[0].Session
}

// recency returns the most recent user interaction anywhere in the block. A
// sub-session the user touched counts for its parent: the parent row is how you
// find it, so it has to rise with it.
func (b sessionBlock) recency() time.Time {
	var newest time.Time
	for _, r := range b.rows {
		if t := LastInteractionAt(r.Session); t.After(newest) {
			newest = t
		}
	}
	return newest
}

// groupNode is one group header with the session blocks directly inside it and
// its child groups, rebuilt from the flattened list so siblings can be reordered
// without touching GroupTree itself (which owns persisted group Order).
type groupNode struct {
	header   Item
	path     string
	blocks   []sessionBlock
	children []*groupNode
	idx      int // original position, for a stable tie-break
}

// recency returns the most recent user interaction anywhere in this group's
// subtree — its own sessions and every descendant group's. This is what lifts a
// whole folder to the top when you touch one session inside it.
func (n *groupNode) recency() time.Time {
	var newest time.Time
	for _, b := range n.blocks {
		if t := b.recency(); t.After(newest) {
			newest = t
		}
	}
	for _, c := range n.children {
		if t := c.recency(); t.After(newest) {
			newest = t
		}
	}
	return newest
}

// moreRecentUnit orders two siblings (groups or blocks) by recency, newest
// first, with a never-touched unit always losing to a touched one and the
// original position as the final, deterministic tie-break.
func moreRecentUnit(a, b time.Time, aIdx, bIdx int) bool {
	if !a.Equal(b) {
		switch {
		case a.IsZero():
			return false
		case b.IsZero():
			return true
		}
		return a.After(b)
	}
	return aIdx < bIdx
}

// parentPath returns the group path one level up ("a/b/c" -> "a/b"), or "" for
// a root-level group.
func parentPath(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return ""
}

// SortGroupsByLastInteraction reorders an already-flattened item list so both
// the groups and the sessions inside them are ordered by the most recent user
// interaction — while the group tree itself is preserved.
//
// This is the GroupViewLastInteractionGrouped mode, the counterpart to
// SortByLastInteraction. That one answers "where was I?" by throwing the folders
// away; this one answers the same question while keeping the folder you were in
// visible around the answer.
//
// Rules, all of them pure recency — session status is never consulted, so a
// session the agent has been hammering for an hour does not climb:
//
//   - Groups are ordered among their siblings by the most recent interaction
//     anywhere in their subtree, so the folder you last worked in floats to the
//     top and its parents rise with it.
//   - Sessions inside a group are ordered by their own most recent interaction,
//     sub-sessions travelling with their parent row.
//   - Pins still win, exactly as they do elsewhere: the Maestro row, then
//     pin-top, then normal, then pin-bottom (pinZone).
//   - Never-touched groups and sessions sort last among their siblings, keeping
//     their existing relative order.
//
// Hierarchy is never violated: a child group always renders under its parent,
// and a group's own sessions always precede its child groups — the same shape
// GroupTree.Flatten emits. Nothing here mutates GroupTree, so the persisted
// group Order and the K/J manual session order survive a trip through this mode.
func SortGroupsByLastInteraction(items []Item) []Item {
	nodesByPath := make(map[string]*groupNode)
	var roots []*groupNode
	var loose []sessionBlock // sessions whose group header is not in the list
	var passthru []Item

	// Pass 1: rebuild the forest from the pre-order flat list.
	for i, it := range items {
		switch it.Type {
		case ItemTypeGroup:
			n := &groupNode{header: it, path: it.Path, idx: i}
			nodesByPath[it.Path] = n
			if parent, ok := nodesByPath[parentPath(it.Path)]; ok && it.Path != "" {
				parent.children = append(parent.children, n)
			} else {
				roots = append(roots, n)
			}
		case ItemTypeSession:
			target := &loose
			if n, ok := nodesByPath[it.Path]; ok {
				target = &n.blocks
			}
			// A sub-session row joins the block above it; anything else — a
			// top-level session, or a sub-session orphaned from its parent —
			// starts one.
			if it.IsSubSession && len(*target) > 0 {
				last := &(*target)[len(*target)-1]
				if len(last.rows) > 0 && it.Level > last.rows[0].Level {
					last.rows = append(last.rows, it)
					continue
				}
			}
			*target = append(*target, sessionBlock{rows: []Item{it}, idx: i})
		case ItemTypeDivider:
			// A divider from a previous partition must not survive.
		default:
			passthru = append(passthru, it)
		}
	}

	// Pass 2: order every sibling list by recency.
	sortBlocks := func(blocks []sessionBlock) {
		sort.SliceStable(blocks, func(i, j int) bool {
			zi, zj := pinZone(blocks[i].head()), pinZone(blocks[j].head())
			if zi != zj {
				return zi < zj
			}
			return moreRecentUnit(blocks[i].recency(), blocks[j].recency(), blocks[i].idx, blocks[j].idx)
		})
	}
	var sortNode func(n *groupNode)
	sortNode = func(n *groupNode) {
		sortBlocks(n.blocks)
		sort.SliceStable(n.children, func(i, j int) bool {
			return moreRecentUnit(n.children[i].recency(), n.children[j].recency(),
				n.children[i].idx, n.children[j].idx)
		})
		for _, c := range n.children {
			sortNode(c)
		}
	}
	for _, r := range roots {
		sortNode(r)
	}
	sort.SliceStable(roots, func(i, j int) bool {
		return moreRecentUnit(roots[i].recency(), roots[j].recency(), roots[i].idx, roots[j].idx)
	})
	sortBlocks(loose)

	// Pass 3: re-emit in pre-order — header, own sessions, then child groups.
	out := make([]Item, 0, len(items))
	var emit func(n *groupNode)
	emit = func(n *groupNode) {
		out = append(out, n.header)
		out = append(out, emitBlocks(n.blocks)...)
		for _, c := range n.children {
			emit(c)
		}
	}
	for _, r := range roots {
		emit(r)
	}
	out = append(out, emitBlocks(loose)...)
	out = append(out, passthru...)
	return out
}

// emitBlocks flattens a group's ordered blocks back to rows, rewriting the
// tree-connector flags for their new positions. Reordering changes which row is
// last, and a stale IsLastInGroup/ParentIsLastInGroup draws the wrong connector
// — a "├─" where the list should close, or a dangling "│" beside the final
// sub-session. The rules mirror GroupTree.Flatten exactly.
func emitBlocks(blocks []sessionBlock) []Item {
	out := make([]Item, 0, len(blocks))
	for bi, b := range blocks {
		isLastTopLevel := bi == len(blocks)-1
		subs := b.rows[1:]
		for ri := range b.rows {
			row := b.rows[ri]
			switch ri {
			case 0:
				row.IsLastInGroup = isLastTopLevel && len(subs) == 0
			default:
				row.IsLastInGroup = isLastTopLevel && ri == len(b.rows)-1
				row.ParentIsLastInGroup = isLastTopLevel
			}
			out = append(out, row)
		}
	}
	return out
}
