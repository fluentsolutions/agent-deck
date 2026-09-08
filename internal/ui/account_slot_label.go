package ui

import "strconv"

// Empty metadata means no explicit slot, not a claim about the running login.
func storedAccountLabel(account string) string {
	if account == "" {
		return "inherited"
	}
	return strconv.Quote(account)
}

const storedAccountPrefix = " [account:"

// Immutable presentation travels with the raw slot in the render snapshot.
// Width-independent quoting is shared across rows with the same stored slot.
type accountPresentation struct {
	label  string
	badge  string
	width  int
	quoted bool
}

func newAccountPresentation(account string) accountPresentation {
	label := storedAccountLabel(account)
	// No stored slot: render no row badge at all. "inherited" is not a fact
	// about the session, it is the absence of one — and it is the WIDEST
	// variant the badge has (19 columns, wider than any real slot name), so on
	// a narrow pane it costs every untagged row that much title and pushes the
	// name into an ellipsis. The rows that carry real information (an explicit
	// slot) keep their badge; the ones with nothing to say give the width back
	// to the title.
	//
	// The label is still returned, because the session info card renders it
	// from here and has the room to say "inherited" usefully.
	if account == "" {
		return accountPresentation{label: label}
	}
	return accountPresentation{
		label:  label,
		badge:  storedAccountPrefix + label + "]",
		width:  len(storedAccountPrefix) + cellWidth(label) + 1,
		quoted: account != "",
	}
}

// Quote before styling/truncation so terminal controls cannot become commands.
// Keep the delimiters visible even when a long account needs an ellipsis.
func (p accountPresentation) fit(budget int) (string, int) {
	if p.width <= budget {
		return p.badge, p.width
	}
	available := budget - len(storedAccountPrefix) - 1
	if available < 1 {
		return "", 0
	}
	label := p.label
	if p.quoted {
		if available < 3 {
			return "", 0
		}
		label = "\"" + cellTruncate(label[1:len(label)-1], available-2, "…") + "\""
	} else {
		label = cellTruncate(label, available, "…")
	}
	return storedAccountPrefix + label + "]", len(storedAccountPrefix) + cellWidth(label) + 1
}
