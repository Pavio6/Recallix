package keyword

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/go-ego/gse"
	"gorm.io/gorm"

	"recallix/internal/repository"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

var seg gse.Segmenter

func init() {
	var err error
	seg, err = gse.NewEmbed()
	if err != nil {
		// Fallback to empty segmenter if loading fails
		seg = gse.Segmenter{}
	}
}

type Hit struct {
	ID    string
	Score float64
}

// DeleteByKnowledgeID removes all lexical index entries for a specific knowledge document.
func DeleteByKnowledgeID(db *gorm.DB, knowledgeID string) error {
	// 先获取该文档下所有 chunk IDs
	var chunkIDs []string
	if err := db.Model(&repository.Chunk{}).Where("knowledge_id = ?", knowledgeID).Pluck("id", &chunkIDs).Error; err != nil {
		return err
	}
	if len(chunkIDs) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 删除 chunk_term_postings
		if err := tx.Where("chunk_id IN ?", chunkIDs).Delete(&repository.ChunkTermPosting{}).Error; err != nil {
			return err
		}
		// 删除 chunk_lexical_indices
		if err := tx.Where("chunk_id IN ?", chunkIDs).Delete(&repository.ChunkLexicalIndex{}).Error; err != nil {
			return err
		}
		return nil
	})
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

// Tokenize uses gse (Go Segmenter Engine) for Chinese text segmentation.
// This provides much better accuracy than the previous bigram approach,
// especially for multi-character Chinese words and phrases.
func Tokenize(text string) []string {
	text = strings.ToLower(text)

	// Use gse CutSearch mode which is optimized for search scenarios
	// It splits compound words and keeps individual characters for better recall
	words := seg.CutSearch(text, true)

	// Filter out single characters and punctuation, keep meaningful tokens
	var tokens []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		// Skip single Chinese characters (keep words with 2+ chars for better precision)
		// But keep single Latin letters/digits as they might be meaningful
		if len([]rune(w)) == 1 && unicode.Is(unicode.Han, []rune(w)[0]) {
			continue
		}
		// Skip punctuation
		if len([]rune(w)) == 1 && unicode.IsPunct([]rune(w)[0]) {
			continue
		}
		tokens = append(tokens, w)
	}

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
