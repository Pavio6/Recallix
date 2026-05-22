package chunker

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	markdownHeadingPattern = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*#*\s*$`)
	numberedSectionPattern = regexp.MustCompile(`(?m)^[ \t]*(?:\d+(?:\.\d+){1,3}\.?[ \t]*|(?:\d+|[IVX]{1,5})[\.\)、][ \t]*)\S.{0,200}$`)
	allCapsHeadingPattern  = regexp.MustCompile(`(?m)^[ \t]*([A-ZÄÖÜ][A-ZÄÖÜ \-]{3,80}):?\s*$`)
	visualSeparatorPattern = regexp.MustCompile(`(?m)^[ \t]*(?:-{3,}|={3,}|\*{3,}|_{3,})[ \t]*$`)
	excessiveBlanksPattern = regexp.MustCompile(`\n{3,}`)
	pageFooterPattern      = regexp.MustCompile(`(?mi)^[ \t]*(?:Seite|Page|页码?)\s+\d+(?:\s*(?:von|of|/)\s*\d+)?[ \t]*$`)
	germanChapterPattern   = regexp.MustCompile(`(?m)^[ \t]*(?:Kapitel|Abschnitt|Teil)\s+(?:[0-9]+|[IVX]{1,5})[\.: ].{0,200}$`)
	englishChapterPattern  = regexp.MustCompile(`(?m)^[ \t]*(?:Chapter|Section|Part)\s+(?:[0-9]+|[IVX]{1,5})[\.: ].{0,200}$`)
	chineseChapterPattern  = regexp.MustCompile(`(?m)^[ \t]*第[ \t]*[一二三四五六七八九十百千零〇0-9]+[ \t]*(?:章|节|節|部分|篇)[ \t]?.{0,200}$`)
)

type docProfile struct {
	mdHeadingTotal       int
	numberedSectionCount int
	allCapsLineCount     int
	formFeedCount        int
	visualSepCount       int
	germanChapterCount   int
	englishChapterCount  int
	chineseChapterCount  int
	blankBlockCount      int
}

func (p docProfile) heuristicMarkerTotal() int {
	return p.numberedSectionCount +
		p.allCapsLineCount +
		p.formFeedCount +
		p.visualSepCount +
		p.germanChapterCount +
		p.englishChapterCount +
		p.chineseChapterCount +
		p.blankBlockCount
}

func (p docProfile) chapterMarkerTotal() int {
	return p.germanChapterCount + p.englishChapterCount + p.chineseChapterCount
}

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
	profile := profileDocument(text)

	switch {
	case profile.mdHeadingTotal > 0:
		result := chunkByHeading(text, cfg)
		if isValidChunkResult(result) {
			return result
		}
	case shouldUseHeuristic(profile):
		result := chunkByHeuristic(text, cfg)
		if isValidChunkResult(result) {
			return result
		}
	}
	return chunkRecursive(text, cfg)
}

func profileDocument(text string) docProfile {
	profile := docProfile{
		formFeedCount:   strings.Count(text, "\f"),
		blankBlockCount: len(excessiveBlanksPattern.FindAllStringIndex(text, -1)),
	}

	inFence := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if markdownHeadingPattern.MatchString(line) {
			profile.mdHeadingTotal++
			continue
		}
		if numberedSectionPattern.MatchString(line) {
			profile.numberedSectionCount++
		}
		if allCapsHeadingPattern.MatchString(line) {
			profile.allCapsLineCount++
		}
		if visualSeparatorPattern.MatchString(line) {
			profile.visualSepCount++
		}
		if germanChapterPattern.MatchString(line) {
			profile.germanChapterCount++
		}
		if englishChapterPattern.MatchString(line) {
			profile.englishChapterCount++
		}
		if chineseChapterPattern.MatchString(line) {
			profile.chineseChapterCount++
		}
	}

	return profile
}

func shouldUseHeuristic(profile docProfile) bool {
	return profile.heuristicMarkerTotal() >= 5 ||
		profile.formFeedCount > 0 ||
		profile.chapterMarkerTotal() > 0
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
	matches := markdownHeadingPattern.FindAllStringSubmatchIndex(text, -1)

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
	runes := []rune(text)
	bounds := findHeuristicBoundaries(text)
	if len(bounds) == 0 {
		return chunkRecursive(text, cfg)
	}
	if bounds[0] != 0 {
		bounds = append([]int{0}, bounds...)
	}
	if bounds[len(bounds)-1] != len(runes) {
		bounds = append(bounds, len(runes))
	}

	results := make([]ChunkResult, 0)
	chunkStart := bounds[0]
	curEnd := chunkStart
	minChunkSize := cfg.ChunkSize / 4
	if minChunkSize < 50 {
		minChunkSize = 50
	}

	for i := 1; i < len(bounds); i++ {
		nextEnd := bounds[i]
		blockLen := nextEnd - curEnd
		if blockLen > cfg.ChunkSize {
			if curEnd > chunkStart {
				results = appendHeuristicChunk(results, runes, chunkStart, curEnd)
			}
			results = appendOversizeHeuristicBlock(results, runes, curEnd, nextEnd, cfg)
			chunkStart = nextEnd
			curEnd = nextEnd
			continue
		}

		if nextEnd-chunkStart > cfg.ChunkSize && curEnd-chunkStart >= minChunkSize {
			results = appendHeuristicChunk(results, runes, chunkStart, curEnd)
			chunkStart = applyHeuristicOverlap(runes, curEnd, cfg.ChunkOverlap, bounds)
		}
		curEnd = nextEnd
	}

	if curEnd > chunkStart {
		results = appendHeuristicChunk(results, runes, chunkStart, curEnd)
	}
	if len(results) == 0 {
		return chunkRecursive(text, cfg)
	}
	return compressSeq(results)
}

func findHeuristicBoundaries(text string) []int {
	boundMap := map[int]struct{}{}
	for i, r := range []rune(text) {
		if r == '\f' {
			boundMap[i] = struct{}{}
		}
	}

	lines := strings.Split(text, "\n")
	pos := 0
	inFence := false
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
		} else if !inFence && isHeuristicBoundaryLine(line) {
			boundMap[pos] = struct{}{}
		}
		pos += len([]rune(line))
		if idx < len(lines)-1 {
			pos++
		}
	}

	for _, loc := range excessiveBlanksPattern.FindAllStringIndex(text, -1) {
		boundMap[runeOffsetAtByte(text, loc[1])] = struct{}{}
	}

	bounds := make([]int, 0, len(boundMap))
	for b := range boundMap {
		bounds = append(bounds, b)
	}
	sortInts(bounds)
	return dedupeBounds(bounds)
}

func isHeuristicBoundaryLine(line string) bool {
	return numberedSectionPattern.MatchString(line) ||
		germanChapterPattern.MatchString(line) ||
		englishChapterPattern.MatchString(line) ||
		chineseChapterPattern.MatchString(line) ||
		allCapsHeadingPattern.MatchString(line) ||
		visualSeparatorPattern.MatchString(line) ||
		pageFooterPattern.MatchString(line)
}

func appendHeuristicChunk(results []ChunkResult, runes []rune, start, end int) []ChunkResult {
	content, trimmedStart, trimmedEnd := trimRuneSpan(runes, start, end)
	if content == "" {
		return results
	}
	return append(results, ChunkResult{
		Content:  content,
		Seq:      len(results),
		StartPos: trimmedStart,
		EndPos:   trimmedEnd,
	})
}

func appendOversizeHeuristicBlock(results []ChunkResult, runes []rune, start, end int, cfg Config) []ChunkResult {
	if end <= start {
		return results
	}
	subResults := chunkRecursiveAt(string(runes[start:end]), cfg, start)
	for _, result := range subResults {
		result.Seq = len(results)
		results = append(results, result)
	}
	return results
}

func applyHeuristicOverlap(runes []rune, curEnd, overlap int, bounds []int) int {
	if overlap <= 0 {
		return curEnd
	}
	target := curEnd - overlap
	if target < 0 {
		target = 0
	}
	windowStart := curEnd - 2*overlap
	if windowStart < 0 {
		windowStart = 0
	}

	bestBound := -1
	for _, b := range bounds {
		if b >= windowStart && b < curEnd && b > bestBound {
			bestBound = b
		}
	}
	if bestBound >= 0 {
		return bestBound
	}

	for i := target; i > windowStart && i < len(runes); i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	return target
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

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		v := values[i]
		j := i - 1
		for j >= 0 && values[j] > v {
			values[j+1] = values[j]
			j--
		}
		values[j+1] = v
	}
}

func dedupeBounds(values []int) []int {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
