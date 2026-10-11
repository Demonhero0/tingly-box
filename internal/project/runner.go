package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// TurnSpec is one turn of a thread: a prompt sent to its Claude Code session.
type TurnSpec struct {
	Dir    string
	Prompt string
	// SessionID is fixed per thread; Resume is false only on the first turn.
	SessionID          string
	Resume             bool
	Model              string
	PermissionMode     string
	AppendSystemPrompt string
	SettingsPath       string
	Env                []string
}

// TurnResult summarizes a finished turn.
type TurnResult struct {
	// Text is the thread's final message of the turn.
	Text         string
	InputTokens  int64
	OutputTokens int64
	Model        string
}

// RunnerEvent is one transcript entry produced while a turn runs.
type RunnerEvent struct {
	Kind    string // one of the db.Ev* kinds
	Content string
	Payload any
}

// Runner runs thread turns. The production runner drives the Claude Code
// CLI through agentboot; tests substitute a scripted one.
type Runner interface {
	RunTurn(ctx context.Context, spec TurnSpec, prompter agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error)
}

// ClaudeRunner runs turns with the Claude Code CLI.
type ClaudeRunner struct {
	Agent agentboot.Agent
}

// NewClaudeRunner uses the claude CLI found on PATH (or CLAUDE_CLI_PATH).
func NewClaudeRunner() *ClaudeRunner {
	return &ClaudeRunner{Agent: claude.NewAgentWithConfig(claude.DefaultConfig())}
}

// Available reports whether a Claude Code CLI was found.
func (r *ClaudeRunner) Available() bool { return r.Agent.IsAvailable() }

// RunTurn implements Runner.
func (r *ClaudeRunner) RunTurn(ctx context.Context, spec TurnSpec, prompter agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
	opts := agentboot.ExecutionOptions{
		ProjectPath:          spec.Dir,
		OutputFormat:         agentboot.OutputFormatStreamJSON,
		Timeout:              -1,
		SessionID:            spec.SessionID,
		Resume:               spec.Resume,
		Model:                spec.Model,
		PermissionMode:       spec.PermissionMode,
		PermissionPromptTool: "stdio",
		AppendSystemPrompt:   spec.AppendSystemPrompt,
		SettingsPath:         spec.SettingsPath,
		Env:                  spec.Env,
	}
	handle, err := r.Agent.Execute(ctx, spec.Prompt, opts)
	if err != nil {
		return TurnResult{}, err
	}
	conv := &converter{seen: map[string]bool{}}
	res, err := agentboot.RunWithPrompter(ctx, handle, prompter, func(raw any) {
		for _, ev := range conv.convert(raw) {
			sink(ev)
		}
	})
	out := TurnResult{Text: conv.lastText, InputTokens: conv.in, OutputTokens: conv.out, Model: conv.model}
	if out.Text == "" && res != nil {
		out.Text = strings.TrimSpace(res.TextOutput())
	}
	if err != nil && conv.succeeded && closedPipe(err) {
		// agentboot's process handle runs cmd.Wait as soon as the CLI starts,
		// and Wait closes stdout; when the CLI exits right after its result,
		// the decoder's final read races that close. The turn already
		// finished successfully, so this is not a failure.
		err = nil
	}
	return out, err
}

// closedPipe reports the "file already closed" read error from a stdout
// pipe that cmd.Wait closed.
func closedPipe(err error) bool {
	return errors.Is(err, os.ErrClosed) || strings.Contains(err.Error(), "file already closed")
}

// maxToolOutput caps a tool result kept in the transcript.
const maxToolOutput = 4 << 10

type converter struct {
	seen map[string]bool
	// succeeded is set once the CLI reported a successful result.
	succeeded bool
	lastText  string
	in, out   int64
	model     string
}

func (c *converter) convert(raw any) []RunnerEvent {
	switch m := raw.(type) {
	case *claude.SystemMessage:
		if m.SubType == claude.SystemSubtypeInit {
			if model, ok := m.Raw["model"].(string); ok {
				c.model = model
			}
		}
	case *claude.AssistantMessage:
		if m.ParentToolUseID != nil && *m.ParentToolUseID != "" {
			return nil // subagent internals stay out of the thread view
		}
		var out []RunnerEvent
		var text strings.Builder
		flush := func() {
			if t := strings.TrimSpace(text.String()); t != "" {
				out = append(out, RunnerEvent{Kind: "assistant", Content: t})
				c.lastText = t
			}
			text.Reset()
		}
		for _, b := range m.Message.Content {
			switch b.Type {
			case claude.ContentBlockTypeText:
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				text.WriteString(b.Text)
			case claude.ContentBlockTypeToolUse:
				flush()
				if c.seen[b.ID] {
					continue
				}
				c.seen[b.ID] = true
				out = append(out, RunnerEvent{Kind: "tool_use", Content: b.Name, Payload: json.RawMessage(b.Input)})
			}
		}
		flush()
		return out
	case *claude.ToolUseMessage:
		if c.seen[m.ToolUseID] {
			return nil
		}
		c.seen[m.ToolUseID] = true
		return []RunnerEvent{{Kind: "tool_use", Content: m.Name, Payload: m.Input}}
	case *claude.ToolResultMessage:
		out := m.Output
		if out == "" {
			var parts []string
			for _, b := range m.Content {
				if tb, ok := b.(*claude.TextBlock); ok && tb.Text != "" {
					parts = append(parts, tb.Text)
				}
			}
			out = strings.Join(parts, "\n")
		}
		if len(out) > maxToolOutput {
			out = out[:maxToolOutput] + "\n… (truncated)"
		}
		return []RunnerEvent{{Kind: "tool_result", Content: out, Payload: map[string]any{"is_error": m.IsError}}}
	case *claude.ResultMessage:
		c.succeeded = !m.IsError
		c.in += int64(m.Usage.InputTokens + m.Usage.CacheReadInputTokens + m.Usage.CacheCreationInputTokens)
		c.out += int64(m.Usage.OutputTokens)
		if m.IsError && m.Result != "" {
			return []RunnerEvent{{Kind: "error", Content: m.Result}}
		}
	}
	return nil
}
