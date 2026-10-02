//go:build eval_smoke

package session_test

import (
	"os"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/testutil"
	"github.com/asheshgoplani/agent-deck/tests/eval/harness"
)

// TestMain releases the shared agent-deck binary build dir after the package's
// tests finish. buildAgentDeck builds it once (and it must outlive every
// individual test), so t.Cleanup can't own it — the temp dir would otherwise
// leak on every `go test` run.
func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

// runTestMain holds the body so the deferred cleanup runs: os.Exit skips
// deferred functions.
func runTestMain(m *testing.M) int {
	// Several tests here run tmux without the harness shim (raw
	// exec.Command("tmux", ...), or the binary under a [tmux].socket_name),
	// which resolves under TMUX_TMPDIR. Isolate it so none can reach the
	// user's live default server. harness.Sandbox.Env passes it to the binary.
	cleanupTmux := testutil.IsolateTmuxSocket()
	defer cleanupTmux()
	code := m.Run()
	harness.RemoveBuildArtifacts()
	return code
}
