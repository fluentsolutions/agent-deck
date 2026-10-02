package main

import (
	"os"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

// runTestMain holds the body so the deferred cleanups run: os.Exit skips
// deferred functions.
func runTestMain(m *testing.M) int {
	cleanupHome := testutil.IsolateHome()
	defer cleanupHome()
	// The suite gives every tmux it spawns an explicit TMUX_TMPDIR under its
	// own sandbox, but every TestMain isolates the socket
	// (internal/testutil/testmain_audit_test.go) so a future test cannot
	// reach the live server by accident.
	cleanupTmux := testutil.IsolateTmuxSocket()
	defer cleanupTmux()
	return m.Run()
}
