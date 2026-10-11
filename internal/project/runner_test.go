package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
)

type allowAll struct{ asked []string }

func (a *allowAll) OnApproval(_ context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	a.asked = append(a.asked, req.ToolName)
	return agentboot.ApprovalResponse{Approved: true}, nil
}

func (a *allowAll) OnAsk(context.Context, agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return agentboot.AskResponse{Approved: true}, nil
}

func TestClaudeRunnerWithScriptedCLI(t *testing.T) {
	script := fixture.Script{
		fixture.System("sess-1", t.TempDir()),
		fixture.AssistantText("Running the tests."),
		fixture.PermissionRequest("req-1", "Bash", map[string]any{"command": "go test ./..."}),
		fixture.AssistantText("All tests pass. Committed on the branch."),
		fixture.Result(true),
	}
	r := &ClaudeRunner{Agent: claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(script))}
	p := &allowAll{}
	var events []RunnerEvent
	res, err := r.RunTurn(context.Background(), TurnSpec{Dir: t.TempDir(), Prompt: "run tests", SessionID: "sess-1"}, p, func(ev RunnerEvent) {
		events = append(events, ev)
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Bash"}, p.asked)
	assert.Equal(t, "All tests pass. Committed on the branch.", res.Text)
	var texts []string
	for _, e := range events {
		if e.Kind == "assistant" {
			texts = append(texts, e.Content)
		}
	}
	assert.Equal(t, []string{"Running the tests.", "All tests pass. Committed on the branch."}, texts)
}

func TestClosedPipeAfterResultIsNotAFailure(t *testing.T) {
	assert.True(t, closedPipe(errors.New("agentboot: claude protocol error: decode: read |0: file already closed")))
	assert.True(t, closedPipe(fmt.Errorf("wrapped: %w", os.ErrClosed)))
	assert.False(t, closedPipe(errors.New("rate limited")))
}
