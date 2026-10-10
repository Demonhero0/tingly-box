package llm

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type openaiClient struct {
	client openai.Client
}

func newOpenAI(conn Connection) *openaiClient {
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(conn.BaseURL, "/") + "/"),
		option.WithMaxRetries(1),
		// Never fall back to OPENAI_API_KEY from the environment.
		option.WithAPIKey(conn.APIKey),
	}
	return &openaiClient{client: openai.NewClient(opts...)}
}

func (o *openaiClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Result, error) {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			msgs = append(msgs, openai.AssistantMessage(m.Content))
		} else {
			msgs = append(msgs, openai.UserMessage(m.Content))
		}
	}
	params := openai.ChatCompletionNewParams{
		Model:         openai.ChatModel(req.Model),
		Messages:      msgs,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}

	var res Result
	var text strings.Builder
	stream := o.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		chunk := stream.Current()
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				onDelta(ch.Delta.Content)
			}
			if ch.FinishReason != "" {
				res.StopReason = ch.FinishReason
			}
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			res.Usage.InputTokens = int(chunk.Usage.PromptTokens)
			res.Usage.OutputTokens = int(chunk.Usage.CompletionTokens)
		}
	}
	res.Text = text.String()
	return res, stream.Err()
}

func (o *openaiClient) ListModels(ctx context.Context) ([]Model, error) {
	var out []Model
	pager := o.client.Models.ListAutoPaging(ctx)
	for pager.Next() {
		m := pager.Current()
		out = append(out, Model{ID: m.ID, DisplayName: m.ID})
	}
	return out, pager.Err()
}
