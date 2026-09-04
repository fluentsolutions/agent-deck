package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// invocationIsAgentOriginated reports whether this agent-deck process was
// launched from inside a session agent-deck itself manages, rather than from a
// terminal the user is sitting at.
//
// Every session agent-deck spawns exports AGENTDECK_INSTANCE_ID (and, for some
// launch paths, AGENT_DECK_SESSION_ID) into the pane's environment, so any
// `agent-deck session ...` command a conductor or sub-agent runs inherits it. A
// command the user types in their own shell, and one the Telegram/Slack bridge
// daemon runs on their behalf, does not.
//
// This is what keeps the last-interaction sort honest. Conductors drive their
// fleet with `agent-deck session send`; if those sends counted as the user
// touching a session, the busiest robot conversation would sit permanently at
// the top of the list — the exact ordering the feature exists to avoid.
func invocationIsAgentOriginated() bool {
	for _, key := range []string{"AGENTDECK_INSTANCE_ID", "AGENT_DECK_SESSION_ID"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// markUserInteraction records that the user personally touched this session —
// sent it a message, attached to it, or started/stopped/restarted it by hand —
// so it sorts to the top of the TUI's last-interaction view (hotkey "t").
//
// It is a no-op for agent-originated invocations (see invocationIsAgentOriginated).
//
// storage may be nil. When it is not, the timestamp is written straight to the
// database with a single targeted UPDATE, because most CLI commands never
// register statedb.SetGlobal and several of them (send, attach) never run a
// full save afterwards — without the direct write the timestamp would live only
// in a process that is about to exit.
func markUserInteraction(storage *session.Storage, inst *session.Instance) {
	if inst == nil || invocationIsAgentOriginated() {
		return
	}
	inst.MarkAccessed()
	if storage == nil {
		return
	}
	db := storage.GetDB()
	if db == nil {
		return
	}
	if err := db.WriteLastAccessed(inst.ID, inst.LastAccessedAt); err != nil {
		// Best-effort: the sort degrades to the previous timestamp, which is
		// never worth failing the user's actual command over.
		fmt.Fprintf(os.Stderr, "Warning: could not record session interaction time: %v\n", err)
	}
}
