package main

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// newTestInstanceForInteraction returns a bare instance with a zero
// LastAccessedAt, i.e. a session the user has never touched.
func newTestInstanceForInteraction() *session.Instance {
	return &session.Instance{ID: "sess-1", Title: "sess-1"}
}

func TestInvocationIsAgentOriginated(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "user's own shell",
			env:  map[string]string{"AGENTDECK_INSTANCE_ID": "", "AGENT_DECK_SESSION_ID": ""},
			want: false,
		},
		{
			name: "inside a managed session (conductor sending to a peer)",
			env:  map[string]string{"AGENTDECK_INSTANCE_ID": "abc123"},
			want: true,
		},
		{
			name: "inside a managed session via the alternate variable",
			env:  map[string]string{"AGENT_DECK_SESSION_ID": "abc123"},
			want: true,
		},
		{
			name: "whitespace-only value is not an identity",
			env:  map[string]string{"AGENTDECK_INSTANCE_ID": "   "},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTDECK_INSTANCE_ID", "")
			t.Setenv("AGENT_DECK_SESSION_ID", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := invocationIsAgentOriginated(); got != tc.want {
				t.Errorf("invocationIsAgentOriginated() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMarkUserInteractionSkipsAgentOriginatedSends(t *testing.T) {
	t.Setenv("AGENTDECK_INSTANCE_ID", "conductor-1")

	inst := newTestInstanceForInteraction()
	markUserInteraction(nil, inst)
	if !inst.LastAccessedAt.IsZero() {
		t.Fatal("an agent-originated send must not stamp the user-interaction clock; " +
			"otherwise conductor traffic pins its targets to the top of the last-interaction sort")
	}
}

func TestMarkUserInteractionRecordsUserSends(t *testing.T) {
	t.Setenv("AGENTDECK_INSTANCE_ID", "")
	t.Setenv("AGENT_DECK_SESSION_ID", "")

	inst := newTestInstanceForInteraction()
	markUserInteraction(nil, inst)
	if inst.LastAccessedAt.IsZero() {
		t.Fatal("a send from the user's own shell must stamp the user-interaction clock")
	}
}

func TestMarkUserInteractionToleratesNilInstance(t *testing.T) {
	t.Setenv("AGENTDECK_INSTANCE_ID", "")
	markUserInteraction(nil, nil) // must not panic
}
