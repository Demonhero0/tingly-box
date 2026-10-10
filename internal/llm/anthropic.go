package llm

import (
	"context"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicClient struct {
	client anthropic.Client
}

func newAnthropic(conn Connection) *anthropicClient {
	opts := []option.RequestOption{
		// Only the connection configured in settings is used: no
		// ANTHROPIC_* env vars or profiles, and a keyless endpoint (a local
		// gateway without auth) is allowed instead of failing for lack of
		// credentials.
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(strings.TrimRight(conn.BaseURL, "/") + "/"),
		option.WithMaxRetries(1),
	}
	if conn.APIKey != "" {
		opts = append(opts, option.WithAPIKey(conn.APIKey))
	}
	return &anthropicClient{client: anthropic.NewClient(opts...)}
}

func (a *anthropicClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Result, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(maxTokens),
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		block := anthropic.NewTextBlock(m.Content)
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		}
	}

	var res Result
	var text strings.Builder
	stream := a.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.MessageStartEvent:
			res.Usage.InputTokens = int(ev.Message.Usage.InputTokens)
		case anthropic.ContentBlockDeltaEvent:
			if d, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok && d.Text != "" {
				text.WriteString(d.Text)
				onDelta(d.Text)
			}
		case anthropic.MessageDeltaEvent:
			if ev.Delta.StopReason != "" {
				res.StopReason = string(ev.Delta.StopReason)
			}
			if ev.Usage.OutputTokens > 0 {
				res.Usage.OutputTokens = int(ev.Usage.OutputTokens)
			}
			if ev.Usage.InputTokens > 0 {
				res.Usage.InputTokens = int(ev.Usage.InputTokens)
			}
		}
	}
	res.Text = text.String()
	return res, stream.Err()
}

func (a *anthropicClient) ListModels(ctx context.Context) ([]Model, error) {
	var out []Model
	pager := a.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for pager.Next() {
		m := pager.Current()
		out = append(out, Model{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out, pager.Err()
}
