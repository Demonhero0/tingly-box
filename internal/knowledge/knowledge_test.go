package knowledge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-project/internal/db"
)

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, EstimateTokens(""))
	assert.Equal(t, 1, EstimateTokens("abcd"))
	assert.Equal(t, 2, EstimateTokens("abcde"))
	assert.Equal(t, 4, EstimateTokens("你好世界"))
}

func TestBuildInline(t *testing.T) {
	assert.Empty(t, Build(PromptInput{Project: &db.Project{}}).System)

	p := &db.Project{Name: "secret name", Description: "secret desc", Instructions: "  Answer in French.  ", Memory: "User likes tables."}
	files := []db.KnowledgeFile{
		{Name: "a&b.md", Content: "alpha", Tokens: 2},
		{Name: "c.txt", Content: "gamma", Tokens: 2},
	}
	got := Build(PromptInput{Project: p, Files: files, Memory: true})
	assert.Equal(t, ModeInline, got.Mode)
	assert.Equal(t, 4, got.KnowledgeTokens)
	s := got.System
	assert.Contains(t, s, `<document index="1">`)
	assert.Contains(t, s, "<source>a&amp;b.md</source>")
	assert.Contains(t, s, `<document index="2">`)
	assert.Contains(t, s, "<project_memory>\nUser likes tables.\n</project_memory>")
	assert.Contains(t, s, "<project_instructions>\nAnswer in French.\n</project_instructions>")
	assert.Less(t, strings.Index(s, "</documents>"), strings.Index(s, "<project_instructions>"))
	assert.NotContains(t, s, "secret", "name and description are never sent")

	noMem := Build(PromptInput{Project: p, Files: files, Memory: false})
	assert.NotContains(t, noMem.System, "project_memory")
}

func TestBuildRetrieval(t *testing.T) {
	filler := strings.Repeat("lorem ipsum dolor sit amet consectetur. ", 400)
	var files []db.KnowledgeFile
	for i := 0; i < 40; i++ {
		content := filler
		if i == 17 {
			content = filler + "\n\nThe launch codename is BLUEHERON and ships in March.\n\n" + filler
		}
		files = append(files, db.KnowledgeFile{Name: "doc.txt", Content: content, Tokens: EstimateTokens(content)})
	}
	got := Build(PromptInput{Project: &db.Project{}, Files: files, Query: "what is the launch codename?"})
	assert.Equal(t, ModeRetrieval, got.Mode)
	assert.Greater(t, got.Excerpts, 0)
	assert.Contains(t, got.System, "BLUEHERON")
	assert.LessOrEqual(t, EstimateTokens(got.System), RetrievalBudget+1000)
}

func TestTokenizeCJK(t *testing.T) {
	assert.Equal(t, []string{"项", "目", "项目", "hello"}, tokenize("项目 Hello"))
}

func TestDecode(t *testing.T) {
	content, mt, err := Decode("notes.md", []byte("\ufeff# Hi"))
	require.NoError(t, err)
	assert.Equal(t, "# Hi", content)
	assert.NotEmpty(t, mt)

	_, mt, err = Decode("main.go", []byte("package main"))
	require.NoError(t, err)
	assert.NotEmpty(t, mt)

	_, _, err = Decode("doc.pdf", []byte("%PDF-1.4\x00\xff"))
	assert.ErrorContains(t, err, "PDF")

	_, _, err = Decode("blob.bin", []byte{0xff, 0xfe, 0x00})
	assert.Error(t, err)
}
