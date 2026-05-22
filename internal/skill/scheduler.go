package skill

import (
	"sort"
	"strings"
	"unicode"

	"recallix/internal/repository"
)

const (
	defaultTopK     = 3
	defaultMinScore = 0.10
)

type Scheduler struct {
	topK     int
	minScore float64
}

type Option func(*Scheduler)

type Candidate struct {
	Skill repository.Skill
	Score float64
}

func NewScheduler(opts ...Option) *Scheduler {
	s := &Scheduler{
		topK:     defaultTopK,
		minScore: defaultMinScore,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithTopK(topK int) Option {
	return func(s *Scheduler) {
		if topK > 0 {
			s.topK = topK
		}
	}
}

func WithMinScore(score float64) Option {
	return func(s *Scheduler) {
		if score >= 0 {
			s.minScore = score
		}
	}
}

func (s *Scheduler) Schedule(question string, available []repository.Skill) []Candidate {
	queryTokens := tokenize(question)
	if len(queryTokens) == 0 || len(available) == 0 {
		return nil
	}

	candidates := make([]Candidate, 0, len(available))
	for _, item := range available {
		score := scoreSkill(question, queryTokens, item)
		if score < s.minScore {
			continue
		}
		candidates = append(candidates, Candidate{Skill: item, Score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Skill.Name < candidates[j].Skill.Name
		}
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > s.topK {
		candidates = candidates[:s.topK]
	}
	return candidates
}

func scoreSkill(question string, queryTokens map[string]struct{}, item repository.Skill) float64 {
	text := strings.TrimSpace(item.Name + " " + item.Description)
	docTokens := tokenize(text)
	if len(docTokens) == 0 {
		return 0
	}

	overlap := 0
	for token := range queryTokens {
		if _, ok := docTokens[token]; ok {
			overlap++
		}
	}
	if overlap == 0 {
		return 0
	}

	coverage := float64(overlap) / float64(len(queryTokens))
	precision := float64(overlap) / float64(len(docTokens))
	score := 0.75*coverage + 0.25*precision

	lowerQuestion := strings.ToLower(question)
	lowerName := strings.ToLower(strings.TrimSpace(item.Name))
	if lowerName != "" && strings.Contains(lowerQuestion, lowerName) {
		score += 0.35
	}
	if score > 1 {
		return 1
	}
	return score
}

func tokenize(text string) map[string]struct{} {
	result := make(map[string]struct{})
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return result
	}

	var word strings.Builder
	var cjkRunes []rune
	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		token := strings.TrimSpace(word.String())
		if token != "" {
			result[token] = struct{}{}
		}
		word.Reset()
	}
	flushCJK := func() {
		if len(cjkRunes) == 0 {
			return
		}
		if len(cjkRunes) == 1 {
			result[string(cjkRunes)] = struct{}{}
		} else {
			for i := 0; i < len(cjkRunes)-1; i++ {
				result[string(cjkRunes[i:i+2])] = struct{}{}
			}
		}
		cjkRunes = nil
	}

	for _, r := range lower {
		switch {
		case isCJK(r):
			flushWord()
			cjkRunes = append(cjkRunes, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word.WriteRune(r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return result
}

func isCJK(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Hangul,
	)
}
