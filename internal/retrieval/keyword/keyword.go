package keyword

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"recallix/internal/repository"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

type Hit struct {
	ID    string
	Score float64
}

// IndexChunk persists one chunk into the lexical inverted index used by BM25.
func IndexChunk(db *gorm.DB, chunk repository.Chunk, knowledgeBaseID string) error {
	terms := Tokenize(chunk.ContextHeader + "\n" + chunk.Content)
	freqs := termFrequencies(terms)

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&repository.ChunkLexicalIndex{
			ChunkID:         chunk.ID,
			UserID:          chunk.UserID,
			KnowledgeBaseID: knowledgeBaseID,
			DocLen:          len(terms),
			CreatedAt:       time.Now(),
		}).Error; err != nil {
			return err
		}

		postings := make([]repository.ChunkTermPosting, 0, len(freqs))
		for term, freq := range freqs {
			postings = append(postings, repository.ChunkTermPosting{
				Term:            term,
				ChunkID:         chunk.ID,
				UserID:          chunk.UserID,
				KnowledgeBaseID: knowledgeBaseID,
				TermFreq:        freq,
				CreatedAt:       time.Now(),
			})
		}
		if len(postings) == 0 {
			return nil
		}
		return tx.Create(&postings).Error
	})
}

func Search(db *gorm.DB, userID, knowledgeBaseID, query string, topK int) ([]Hit, error) {
	queryTerms := uniqueTerms(Tokenize(query))
	if len(queryTerms) == 0 {
		return nil, nil
	}

	var corpus struct {
		Count  int64
		AvgLen float64
	}
	q := db.Model(&repository.ChunkLexicalIndex{}).Where("user_id = ?", userID)
	if knowledgeBaseID != "" {
		q = q.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	if err := q.Select("COUNT(*) AS count, COALESCE(AVG(doc_len), 0) AS avg_len").Scan(&corpus).Error; err != nil {
		return nil, err
	}
	if corpus.Count == 0 || corpus.AvgLen == 0 {
		return nil, nil
	}

	var dfs []struct {
		Term string
		DF   int64
	}
	dfQuery := db.Model(&repository.ChunkTermPosting{}).
		Select("term, COUNT(*) AS df").
		Where("user_id = ? AND term IN ?", userID, queryTerms)
	if knowledgeBaseID != "" {
		dfQuery = dfQuery.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	if err := dfQuery.Group("term").Scan(&dfs).Error; err != nil {
		return nil, err
	}
	dfMap := make(map[string]int64, len(dfs))
	for _, row := range dfs {
		dfMap[row.Term] = row.DF
	}

	var postings []struct {
		Term     string
		ChunkID  string
		TermFreq int
		DocLen   int
	}
	postingQuery := db.Table("chunk_term_postings").
		Select("chunk_term_postings.term, chunk_term_postings.chunk_id, chunk_term_postings.term_freq, chunk_lexical_indices.doc_len").
		Joins("JOIN chunk_lexical_indices ON chunk_lexical_indices.chunk_id = chunk_term_postings.chunk_id").
		Where("chunk_term_postings.user_id = ? AND chunk_term_postings.term IN ?", userID, queryTerms)
	if knowledgeBaseID != "" {
		postingQuery = postingQuery.Where("chunk_term_postings.knowledge_base_id = ?", knowledgeBaseID)
	}
	if err := postingQuery.Find(&postings).Error; err != nil {
		return nil, err
	}

	scores := make(map[string]float64)
	for _, posting := range postings {
		df := dfMap[posting.Term]
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(corpus.Count)-float64(df)+0.5)/(float64(df)+0.5))
		tf := float64(posting.TermFreq)
		docLen := float64(posting.DocLen)
		score := idf * (tf * (bm25K1 + 1)) / (tf + bm25K1*(1-bm25B+bm25B*docLen/corpus.AvgLen))
		scores[posting.ChunkID] += score
	}

	hits := make([]Hit, 0, len(scores))
	for id, score := range scores {
		hits = append(hits, Hit{ID: id, Score: score})
	}
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

// Tokenize uses Latin word tokens plus CJK bigrams. The bigram strategy keeps
// the MVP dependency-free while making Chinese text searchable without spaces.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var latin []rune
	var cjk []rune

	flushLatin := func() {
		if len(latin) > 1 {
			tokens = append(tokens, string(latin))
		}
		latin = nil
	}
	flushCJK := func() {
		switch len(cjk) {
		case 0:
		case 1:
			tokens = append(tokens, string(cjk))
		default:
			for i := 0; i < len(cjk)-1; i++ {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = nil
	}

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin = append(latin, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return tokens
}

func termFrequencies(terms []string) map[string]int {
	freqs := make(map[string]int)
	for _, term := range terms {
		freqs[term]++
	}
	return freqs
}

func uniqueTerms(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}
