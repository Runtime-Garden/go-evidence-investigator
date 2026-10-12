// Package investigator provides read-only, reproducible passage retrieval.
// It does not make scientific truth claims or use gold labels during inference.
package investigator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const MaxJSONLLine = 4 * 1024 * 1024

type Document struct {
	DocID    int64    `json:"doc_id"`
	Title    string   `json:"title"`
	Abstract []string `json:"abstract"`
}

type Claim struct {
	ID       int64  `json:"id"`
	Claim    string `json:"claim"`
	Evidence map[string][]struct {
		Label     string `json:"label"`
		Sentences []int  `json:"sentences"`
	} `json:"evidence,omitempty"`
}

type Passage struct {
	SourceID      string  `json:"source_id"`
	DocumentID    int64   `json:"document_id"`
	Title         string  `json:"title"`
	SentenceIndex int     `json:"sentence_index"`
	Quote         string  `json:"quote"`
	SHA256        string  `json:"sha256"`
	Score         float64 `json:"score"`
	Dataset       string  `json:"dataset"`
}

type posting struct{ Index, Freq int }
type indexedPassage struct {
	Passage
	Length int
}

type Engine struct {
	Dataset       string
	Documents     int
	Claims        []Claim
	passages      []indexedPassage
	inverted      map[string][]posting
	meanLength    float64
	sourceIndex   map[string]int
	documentIndex map[int64][]int
}

func Load(corpusPath, claimsPath, dataset string) (*Engine, error) {
	if strings.TrimSpace(dataset) == "" {
		return nil, errors.New("dataset name is required")
	}
	e := &Engine{Dataset: dataset, inverted: make(map[string][]posting), sourceIndex: make(map[string]int), documentIndex: make(map[int64][]int)}
	err := scanJSONL(corpusPath, func(raw json.RawMessage) error {
		var d Document
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		if d.DocID == 0 || strings.TrimSpace(d.Title) == "" || len(d.Abstract) == 0 {
			return errors.New("invalid document schema")
		}
		e.Documents++
		for sentenceID, sentence := range d.Abstract {
			sentence = strings.TrimSpace(sentence)
			if sentence == "" {
				continue
			}
			digest := sha256.Sum256([]byte(sentence))
			p := indexedPassage{Passage: Passage{
				SourceID:   fmt.Sprintf("%s:%d:%d", dataset, d.DocID, sentenceID),
				DocumentID: d.DocID, Title: d.Title, SentenceIndex: sentenceID,
				Quote: sentence, SHA256: hex.EncodeToString(digest[:]), Dataset: dataset,
			}}
			terms := tokenize(d.Title + " " + sentence)
			p.Length = len(terms)
			index := len(e.passages)
			e.passages = append(e.passages, p)
			e.sourceIndex[p.SourceID] = index
			e.documentIndex[d.DocID] = append(e.documentIndex[d.DocID], index)
			counts := make(map[string]int)
			for _, t := range terms {
				counts[t]++
			}
			for t, n := range counts {
				e.inverted[t] = append(e.inverted[t], posting{index, n})
			}
			e.meanLength += float64(len(terms))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("corpus: %w", err)
	}
	if len(e.passages) == 0 {
		return nil, errors.New("no passages in corpus")
	}
	e.meanLength /= float64(len(e.passages))
	if claimsPath != "" {
		err = scanJSONL(claimsPath, func(raw json.RawMessage) error {
			var c Claim
			if err := json.Unmarshal(raw, &c); err != nil {
				return err
			}
			if c.ID == 0 || strings.TrimSpace(c.Claim) == "" {
				return errors.New("invalid claim schema")
			}
			e.Claims = append(e.Claims, c)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("claims: %w", err)
		}
	}
	return e, nil
}

func scanJSONL(path string, handle func(json.RawMessage) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), MaxJSONLLine)
	line := 0
	for s.Scan() {
		line++
		b := strings.TrimSpace(s.Text())
		if b == "" {
			continue
		}
		if err := handle(json.RawMessage(b)); err != nil {
			return fmt.Errorf("%s line %d: %w", path, line, err)
		}
	}
	return s.Err()
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "for": true, "with": true,
	"at": true, "and": true, "or": true, "of": true, "in": true, "on": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "by": true,
	"that": true, "this": true, "from": true, "as": true, "it": true,
}

func tokenize(s string) []string {
	var result []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			t := b.String()
			if !stopwords[t] {
				result = append(result, t)
			}
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return result
}

// Search uses BM25 over individual abstract sentences (with title context).
// Scores indicate lexical relevance, never entailment confidence.
func (e *Engine) Search(claim string, k int) []Passage {
	if k < 1 {
		k = 1
	}
	if k > 10 {
		k = 10
	}
	unique := make(map[string]bool)
	for _, t := range tokenize(claim) {
		unique[t] = true
	}
	scores := make(map[int]float64)
	N := float64(len(e.passages))
	const k1, b = 1.2, 0.75
	for term := range unique {
		ps := e.inverted[term]
		if len(ps) == 0 {
			continue
		}
		df := float64(len(ps))
		idf := math.Log(1 + (N-df+0.5)/(df+0.5))
		for _, p := range ps {
			length := float64(e.passages[p.Index].Length)
			tf := float64(p.Freq)
			scores[p.Index] += idf * (tf * (k1 + 1) / (tf + k1*(1-b+b*length/e.meanLength)))
		}
	}
	type rank struct {
		index int
		score float64
	}
	ranked := make([]rank, 0, len(scores))
	for id, s := range scores {
		ranked = append(ranked, rank{id, s})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].index < ranked[j].index
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	out := make([]Passage, 0, len(ranked))
	for _, p := range ranked {
		v := e.passages[p.index].Passage
		v.Score = math.Round(p.score*10000) / 10000
		out = append(out, v)
	}
	return out
}

type Metrics struct {
	Dataset           string  `json:"dataset"`
	TopK              int     `json:"top_k"`
	Evaluated         int     `json:"evaluated"`
	Hits              int     `json:"hits"`
	DocumentRecallAtK float64 `json:"document_recall_at_k"`
	SkippedNoGold     int     `json:"skipped_no_gold"`
}

// Evaluate calculates gold document hit recall, not verdict accuracy or strict
// rationale recall. Uses claims only as queries; gold docs are never searched.
func (e *Engine) Evaluate(k, limit int) Metrics {
	m := Metrics{Dataset: e.Dataset, TopK: k}
	if limit <= 0 || limit > len(e.Claims) {
		limit = len(e.Claims)
	}
	for _, c := range e.Claims[:limit] {
		if len(c.Evidence) == 0 {
			m.SkippedNoGold++
			continue
		}
		m.Evaluated++
		ranked := e.Search(c.Claim, k)
		matched := false
		for _, p := range ranked {
			if _, ok := c.Evidence[strconv.FormatInt(p.DocumentID, 10)]; ok {
				matched = true
				break
			}
		}
		if matched {
			m.Hits++
		}
	}
	if m.Evaluated > 0 {
		m.DocumentRecallAtK = float64(m.Hits) / float64(m.Evaluated)
	}
	return m
}

// AuditQuote proves the returned quote equals the exact corpus sentence loaded.
func AuditQuote(p Passage) bool {
	sum := sha256.Sum256([]byte(p.Quote))
	return hex.EncodeToString(sum[:]) == p.SHA256
}

func ClaimsPreview(claims []Claim, limit int) []struct {
	ID    int64  `json:"id"`
	Claim string `json:"claim"`
} {
	if limit < 1 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	if limit > len(claims) {
		limit = len(claims)
	}
	out := make([]struct {
		ID    int64  `json:"id"`
		Claim string `json:"claim"`
	}, 0, limit)
	for _, c := range claims[:limit] {
		out = append(out, struct {
			ID    int64  `json:"id"`
			Claim string `json:"claim"`
		}{c.ID, c.Claim})
	}
	return out
}

// LookupSource retrieves an immutable copy of a corpus sentence, not its gold label.
func (e *Engine) LookupSource(sourceID string) (Passage, bool) {
	index, ok := e.sourceIndex[sourceID]
	if !ok {
		return Passage{}, false
	}
	return e.passages[index].Passage, true
}

// ExpandDocument returns at most limit sentences from a known corpus document.
// It does not expand into an untrusted network source or expose gold labels.
func (e *Engine) ExpandDocument(sourceID string, limit int) []Passage {
	index, ok := e.sourceIndex[sourceID]
	if !ok {
		return nil
	}
	if limit < 1 || limit > 3 {
		limit = 3
	}
	docID := e.passages[index].DocumentID
	ids := e.documentIndex[docID]
	out := make([]Passage, 0, limit)
	// Start with the requested sentence and then its neighbors.
	pos := 0
	for i, id := range ids {
		if id == index {
			pos = i
			break
		}
	}
	for radius := 0; len(out) < limit && radius < len(ids); radius++ {
		if pos-radius >= 0 {
			out = append(out, e.passages[ids[pos-radius]].Passage)
		}
		if radius > 0 && len(out) < limit && pos+radius < len(ids) {
			out = append(out, e.passages[ids[pos+radius]].Passage)
		}
	}
	return out
}
