package knowledge

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/tingly-dev/tingly-project/internal/db"
)

// chunkTokens is the target size of a retrieval chunk.
const chunkTokens = 800

// RetrievedDoc is one source file with the chunks picked from it, in file
// order.
type RetrievedDoc struct {
	Name   string
	Chunks []string
}

type chunk struct {
	file  int
	index int
	text  string
	terms map[string]int
	len   int
	score float64
}

// Retrieve splits files into chunks, ranks them with BM25 against query and
// returns the best chunks that fit in budget tokens, grouped per file. With
// an empty query it returns the start of each file.
func Retrieve(files []db.KnowledgeFile, query string, budget int) []RetrievedDoc {
	var chunks []*chunk
	for fi, f := range files {
		for ci, text := range split(f.Content, chunkTokens) {
			terms := map[string]int{}
			n := 0
			for _, t := range tokenize(text) {
				terms[t]++
				n++
			}
			chunks = append(chunks, &chunk{file: fi, index: ci, text: text, terms: terms, len: n})
		}
	}
	if len(chunks) == 0 {
		return nil
	}

	qterms := uniq(tokenize(query))
	if len(qterms) > 0 {
		df := map[string]int{}
		totalLen := 0
		for _, c := range chunks {
			totalLen += c.len
			for _, t := range qterms {
				if c.terms[t] > 0 {
					df[t]++
				}
			}
		}
		avg := float64(totalLen) / float64(len(chunks))
		const k1, bParam = 1.2, 0.75
		N := float64(len(chunks))
		for _, c := range chunks {
			for _, t := range qterms {
				tf := float64(c.terms[t])
				if tf == 0 {
					continue
				}
				idf := math.Log(1 + (N-float64(df[t])+0.5)/(float64(df[t])+0.5))
				c.score += idf * tf * (k1 + 1) / (tf + k1*(1-bParam+bParam*float64(c.len)/avg))
			}
		}
	}

	ranked := append([]*chunk(nil), chunks...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		// Ties (including no query): earlier chunks first, round-robin
		// across files so every file gets its opening.
		if ranked[i].index != ranked[j].index {
			return ranked[i].index < ranked[j].index
		}
		return ranked[i].file < ranked[j].file
	})

	var picked []*chunk
	used := 0
	for _, c := range ranked {
		t := EstimateTokens(c.text)
		if used+t > budget {
			continue
		}
		picked = append(picked, c)
		used += t
	}
	sort.Slice(picked, func(i, j int) bool {
		if picked[i].file != picked[j].file {
			return picked[i].file < picked[j].file
		}
		return picked[i].index < picked[j].index
	})

	var out []RetrievedDoc
	lastFile := -1
	for _, c := range picked {
		if c.file != lastFile {
			out = append(out, RetrievedDoc{Name: files[c.file].Name})
			lastFile = c.file
		}
		out[len(out)-1].Chunks = append(out[len(out)-1].Chunks, c.text)
	}
	return out
}

// split breaks text into chunks of about size tokens on paragraph, then line,
// boundaries.
func split(text string, size int) []string {
	var out []string
	var cur strings.Builder
	curTokens := 0
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
		curTokens = 0
	}
	for _, para := range strings.SplitAfter(text, "\n\n") {
		pieces := []string{para}
		if EstimateTokens(para) > size {
			pieces = strings.SplitAfter(para, "\n")
		}
		for _, p := range pieces {
			for EstimateTokens(p) > size {
				// A single huge line: cut by runes.
				r := []rune(p)
				cut := size * 3
				if cut > len(r) {
					cut = len(r)
				}
				flush()
				out = append(out, string(r[:cut]))
				p = string(r[cut:])
			}
			t := EstimateTokens(p)
			if curTokens+t > size {
				flush()
			}
			cur.WriteString(p)
			curTokens += t
		}
	}
	flush()
	return out
}

// tokenize lowercases latin words and emits CJK text as single characters
// plus bigrams, which works without a dictionary.
func tokenize(s string) []string {
	var out []string
	var word []rune
	var prevCJK rune
	flushWord := func() {
		if len(word) > 1 {
			out = append(out, string(word))
		}
		word = word[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case isCJK(r):
			flushWord()
			out = append(out, string(r))
			if prevCJK != 0 {
				out = append(out, string([]rune{prevCJK, r}))
			}
			prevCJK = r
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			word = append(word, r)
		default:
			flushWord()
		}
		prevCJK = 0
	}
	flushWord()
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
