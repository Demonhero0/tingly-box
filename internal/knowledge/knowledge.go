// Package knowledge turns a project's instructions and knowledge files into
// the system prompt sent with every conversation in that project, and decides
// which uploaded files are usable as knowledge.
package knowledge

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/tingly-dev/tingly-project/internal/db"
)

// ContextLimitTokens is the model context window the budgets below are
// carved from.
const ContextLimitTokens = 200_000

// InlineBudget is how much knowledge is sent in full with every request.
// Half the window leaves room for the chat itself.
const InlineBudget = 100_000

// RetrievalBudget is how much knowledge is sent per request once a project
// is in retrieval mode.
const RetrievalBudget = 30_000

// MaxKnowledgeTokens caps a project's knowledge: retrieval mode stretches
// capacity to 10x the inline budget, as on claude.ai.
const MaxKnowledgeTokens = 10 * InlineBudget

// MaxFileBytes caps a single uploaded knowledge file.
const MaxFileBytes = 10 << 20

// Knowledge modes.
const (
	ModeInline    = "inline"
	ModeRetrieval = "retrieval"
)

// ModeFor reports which mode a project with this much knowledge runs in.
func ModeFor(knowledgeTokens int) string {
	if knowledgeTokens > InlineBudget {
		return ModeRetrieval
	}
	return ModeInline
}

// EstimateTokens approximates the token count of s without a tokenizer:
// roughly four ASCII characters per token, and one token per non-ASCII rune
// (CJK text tokenizes close to one token per character).
func EstimateTokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}

// PromptInput is everything that goes into a project chat's system prompt.
type PromptInput struct {
	Project *db.Project
	Files   []db.KnowledgeFile
	// Query is the recent user text retrieval ranks chunks against.
	Query string
	// Memory includes the project memory; false for chats with memory off.
	Memory bool
}

// Prompt is the assembled system prompt and how knowledge was included.
type Prompt struct {
	System          string `json:"system_prompt"`
	Mode            string `json:"mode"`
	KnowledgeTokens int    `json:"knowledge_tokens"`
	// Excerpts is the number of chunks sent in retrieval mode.
	Excerpts int `json:"excerpts"`
}

// Build assembles a project's system prompt: knowledge first (long context
// reads best before the instructions that refer to it), then memory, then the
// custom instructions. Name and description are never included, as on
// claude.ai.
func Build(in PromptInput) Prompt {
	total := 0
	for _, f := range in.Files {
		total += f.Tokens
	}
	out := Prompt{Mode: ModeFor(total), KnowledgeTokens: total}

	var b strings.Builder
	if len(in.Files) > 0 {
		if out.Mode == ModeInline {
			b.WriteString("The user has attached the following documents to this project as reference knowledge. ")
			b.WriteString("Use them when they are relevant to the user's request.\n\n<documents>\n")
			for i, f := range in.Files {
				writeDocument(&b, i+1, f.Name, f.Content)
			}
			b.WriteString("</documents>\n")
		} else {
			picked := Retrieve(in.Files, in.Query, RetrievalBudget)
			out.Excerpts = 0
			b.WriteString("This project's knowledge is too large to include in full. ")
			b.WriteString("The excerpts below were retrieved as the most relevant to the conversation; ")
			b.WriteString("say so if they do not contain what is needed.\n\n<documents>\n")
			for i, d := range picked {
				out.Excerpts += len(d.Chunks)
				writeDocument(&b, i+1, d.Name, strings.Join(d.Chunks, "\n[…]\n"))
			}
			b.WriteString("</documents>\n")
		}
	}
	section := func(tag, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "<%s>\n%s\n</%s>\n", tag, body, tag)
	}
	if in.Project != nil {
		if in.Memory {
			section("project_memory", in.Project.Memory)
		}
		section("project_instructions", in.Project.Instructions)
	}
	out.System = b.String()
	return out
}

func writeDocument(b *strings.Builder, index int, name, content string) {
	fmt.Fprintf(b, "<document index=\"%d\">\n<source>%s</source>\n<document_content>\n%s\n</document_content>\n</document>\n",
		index, html.EscapeString(name), content)
}

// Decode validates an uploaded file as text knowledge and returns its content
// and MIME type, or a user-facing reason it was rejected.
func Decode(name string, data []byte) (content, mimeType string, err error) {
	if len(data) > MaxFileBytes {
		return "", "", fmt.Errorf("file is larger than %d MB", MaxFileBytes>>20)
	}
	ext := strings.ToLower(filepath.Ext(name))
	sniffed := http.DetectContentType(data)
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		switch {
		case ext == ".pdf" || strings.HasPrefix(sniffed, "application/pdf"):
			return "", "", fmt.Errorf("PDF is not supported yet — paste its text with \"Add text content\"")
		case strings.HasPrefix(sniffed, "image/"):
			return "", "", fmt.Errorf("images are not supported as knowledge")
		default:
			return "", "", fmt.Errorf("not a UTF-8 text file")
		}
	}
	mimeType = mime.TypeByExtension(ext)
	if i := strings.Index(mimeType, ";"); i >= 0 {
		mimeType = mimeType[:i]
	}
	if mimeType == "" || strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		mimeType = "text/plain"
	}
	return strings.TrimPrefix(string(data), "\ufeff"), mimeType, nil
}
