package chunker

import (
	"regexp"
	"strings"
	"unicode"
)

type Config struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
	Strategy     string // auto, heading, heuristic, recursive
}

type ChunkResult struct {
	Content       string
	Seq           int
	StartPos      int
	EndPos        int
	ContextHeader string
}

func DefaultConfig() Config {
	return Config{
		ChunkSize:    512,
		ChunkOverlap: 80,
		Separators:   []string{"\n\n", "\n", "。", "！", "？", ";", "；"},
		Strategy:     "auto",
	}
}

const defaultChunkSize = 512

func Chunk(text string, cfg Config) []ChunkResult {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	switch cfg.Strategy {
	case "heading":
		return chunkByHeading(text, cfg)
	case "heuristic":
		return chunkByHeuristic(text, cfg)
	case "recursive":
		return chunkRecursive(text, cfg)
	case "auto":
		fallthrough
	default:
		return chunkAuto(text, cfg)
	}
}

func chunkAuto(text string, cfg Config) []ChunkResult {
	hasHeadings := regexp.MustCompile(`(?m)^#{1,6}\s`).MatchString(text)
	hasChapterMarkers := regexp.MustCompile(`(?m)^(第[一二三四五六七八九十\d]+[章节]|\d+[\.\)、])`).MatchString(text)

	switch {
	case hasHeadings:
		result := chunkByHeading(text, cfg)
		if isValidChunkResult(result) {
			return result
		}
	case hasChapterMarkers:
		result := chunkByHeuristic(text, cfg)
		if isValidChunkResult(result) {
			return result
		}
	}
	return chunkRecursive(text, cfg)
}

func isValidChunkResult(result []ChunkResult) bool {
	if len(result) == 0 {
		return false
	}
	tooManyTiny := 0
	for _, r := range result {
		if len([]rune(r.Content)) < 10 {
			tooManyTiny++
		}
	}
	if len(result) > 10 && tooManyTiny > len(result)/2 {
		return false
	}
	return true
}

func chunkByHeading(text string, cfg Config) []ChunkResult {
	headingRe := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	matches := headingRe.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		return chunkRecursive(text, cfg)
	}

	type section struct {
		level     int
		title     string
		startByte int
		endByte   int
		startRune int
	}

	sections := []section{}
	for i, m := range matches {
		level := len(text[m[2]:m[3]])
		title := text[m[4]:m[5]]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		sections = append(sections, section{
			level:     level,
			title:     title,
			startByte: m[0],
			endByte:   end,
			startRune: runeOffsetAtByte(text, m[0]),
		})
	}

	breadcrumbs := []section{}
	seq := 0
	results := make([]ChunkResult, 0)

	for _, sec := range sections {
		for len(breadcrumbs) > 0 && breadcrumbs[len(breadcrumbs)-1].level >= sec.level {
			breadcrumbs = breadcrumbs[:len(breadcrumbs)-1]
		}
		breadcrumbs = append(breadcrumbs, sec)

		header := ""
		for _, bc := range breadcrumbs {
			lvl := strings.Repeat("#", bc.level)
			if header != "" {
				header += " > "
			}
			header += lvl + " " + bc.title
		}

		body := text[sec.startByte:sec.endByte]
		subResults := chunkRecursiveAt(body, cfg, sec.startRune)
		for i := range subResults {
			subResults[i].Seq = seq
			subResults[i].ContextHeader = header
			seq++
		}
		results = append(results, subResults...)
	}

	return compressSeq(results)
}

func chunkByHeuristic(text string, cfg Config) []ChunkResult {
	patterns := []string{
		`\f`,
		`\n---+\n`,
		`\n={3,}\n`,
		`(?m)^第[一二三四五六七八九十\d]+[章节]`,
		`(?m)^\d+[\.\)、]\s`,
		`(?m)^[A-Z][A-Z\s]{3,}$`,
	}
	combined := strings.Join(patterns, "|")
	re := regexp.MustCompile(combined)
	locations := re.FindAllStringIndex(text, -1)

	if len(locations) == 0 {
		return chunkRecursive(text, cfg)
	}

	results := make([]ChunkResult, 0)
	prevEndByte := 0
	seq := 0

	for _, loc := range locations {
		if loc[1] > prevEndByte {
			startRune := runeOffsetAtByte(text, prevEndByte)
			endRune := runeOffsetAtByte(text, loc[1])
			content, startRune, endRune := trimRuneSpan([]rune(text), startRune, endRune)
			if content != "" {
				results = append(results, ChunkResult{
					Content:  content,
					Seq:      seq,
					StartPos: startRune,
					EndPos:   endRune,
				})
				seq++
			}
		}
		prevEndByte = loc[1]
	}

	if prevEndByte < len(text) {
		startRune := runeOffsetAtByte(text, prevEndByte)
		endRune := len([]rune(text))
		content, startRune, endRune := trimRuneSpan([]rune(text), startRune, endRune)
		if content != "" {
			results = append(results, ChunkResult{
				Content:  content,
				Seq:      seq,
				StartPos: startRune,
				EndPos:   endRune,
			})
			seq++
		}
	}

	return compressSeq(results)
}

func chunkRecursive(text string, cfg Config) []ChunkResult {
	return chunkRecursiveAt(text, cfg, 0)
}

func chunkRecursiveAt(text string, cfg Config, baseRuneOffset int) []ChunkResult {
	units := splitToUnits([]rune(text), 0, len([]rune(text)), cfg.Separators, baseRuneOffset)
	results := mergeUnits(units, cfg)
	return results
}

type textUnit struct {
	content string
	start   int
	end     int
}

func splitToUnits(text []rune, start, end int, separators []string, baseRuneOffset int) []textUnit {
	if len(separators) == 0 {
		content, trimmedStart, trimmedEnd := trimRuneSpan(text, start, end)
		if content == "" {
			return nil
		}
		return []textUnit{{content: content, start: baseRuneOffset + trimmedStart, end: baseRuneOffset + trimmedEnd}}
	}
	sep := []rune(separators[0])
	ranges := splitRuneRanges(text, start, end, sep)
	units := make([]textUnit, 0)
	for _, rg := range ranges {
		content, trimmedStart, trimmedEnd := trimRuneSpan(text, rg[0], rg[1])
		if content == "" {
			continue
		}
		if trimmedEnd-trimmedStart > defaultChunkSize && len(separators) > 1 {
			subUnits := splitToUnits(text, trimmedStart, trimmedEnd, separators[1:], baseRuneOffset)
			units = append(units, subUnits...)
		} else {
			units = append(units, textUnit{
				content: content,
				start:   baseRuneOffset + trimmedStart,
				end:     baseRuneOffset + trimmedEnd,
			})
		}
	}
	return units
}

func mergeUnits(units []textUnit, cfg Config) []ChunkResult {
	results := make([]ChunkResult, 0)
	seq := 0

	i := 0
	for i < len(units) {
		current := units[i]
		chunkStart := current.start
		chunkEnd := current.end
		overlapText := ""
		if i > 0 {
			overlapRunes := []rune{}
			for j := i - 1; j >= 0 && len(overlapRunes) < cfg.ChunkOverlap; j-- {
				prevRunes := []rune(units[j].content)
				needed := cfg.ChunkOverlap - len(overlapRunes)
				if needed > len(prevRunes) {
					needed = len(prevRunes)
				}
				from := len(prevRunes) - needed
				if from < 0 {
					from = 0
				}
				overlapRunes = append(prevRunes[from:], overlapRunes...)
			}
			overlapText = string(overlapRunes)
			if cfg.ChunkOverlap > 0 {
				chunkStart = max(0, current.start-len([]rune(overlapText)))
			}
		}

		fullContent := overlapText
		if fullContent != "" {
			fullContent += "\n"
		}
		fullContent += current.content

		for i+1 < len(units) && len([]rune(fullContent+units[i+1].content)) <= cfg.ChunkSize {
			i++
			fullContent += "\n" + units[i].content
			chunkEnd = units[i].end
		}

		results = append(results, ChunkResult{
			Content:  strings.TrimSpace(fullContent),
			Seq:      seq,
			StartPos: chunkStart,
			EndPos:   chunkEnd,
		})
		seq++
		i++
	}

	return results
}

func splitRuneRanges(text []rune, start, end int, sep []rune) [][2]int {
	if len(sep) == 0 {
		return [][2]int{{start, end}}
	}
	ranges := make([][2]int, 0)
	partStart := start
	for i := start; i <= end-len(sep); {
		if runeSliceHasPrefix(text[i:end], sep) {
			ranges = append(ranges, [2]int{partStart, i})
			i += len(sep)
			partStart = i
			continue
		}
		i++
	}
	ranges = append(ranges, [2]int{partStart, end})
	return ranges
}

func runeSliceHasPrefix(text, prefix []rune) bool {
	if len(prefix) > len(text) {
		return false
	}
	for i := range prefix {
		if text[i] != prefix[i] {
			return false
		}
	}
	return true
}

func trimRuneSpan(text []rune, start, end int) (string, int, int) {
	for start < end && unicode.IsSpace(text[start]) {
		start++
	}
	for end > start && unicode.IsSpace(text[end-1]) {
		end--
	}
	return string(text[start:end]), start, end
}

func runeOffsetAtByte(text string, byteIndex int) int {
	if byteIndex <= 0 {
		return 0
	}
	if byteIndex >= len(text) {
		return len([]rune(text))
	}
	return len([]rune(text[:byteIndex]))
}

func compressSeq(results []ChunkResult) []ChunkResult {
	for i := range results {
		results[i].Seq = i
	}
	return results
}
