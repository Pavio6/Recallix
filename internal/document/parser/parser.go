package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type MarkdownContent struct {
	Text     string
	Metadata map[string]string
}

type Parser interface {
	Parse(path string) (*MarkdownContent, error)
}

func GetParser(ext string) (Parser, error) {
	switch strings.ToLower(ext) {
	case ".md", ".txt":
		return &PlainParser{}, nil
	case ".docx":
		return &DocxParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

type PlainParser struct{}

func (p *PlainParser) Parse(path string) (*MarkdownContent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &MarkdownContent{Text: string(data)}, nil
}

func ParseBytes(ext string, data []byte) (*MarkdownContent, error) {
	switch strings.ToLower(ext) {
	case ".md", ".txt":
		return &MarkdownContent{Text: string(data)}, nil
	case ".docx":
		return parseDocxBytes(data)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

type DocxParser struct{}

func (p *DocxParser) Parse(path string) (*MarkdownContent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseDocxBytes(data)
}

func parseDocxBytes(data []byte) (*MarkdownContent, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid docx file: %w", err)
	}
	var docXML, stylesXML, numberingXML []byte
	for _, f := range reader.File {
		switch f.Name {
		case "word/document.xml", "word/styles.xml", "word/numbering.xml":
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			switch f.Name {
			case "word/document.xml":
				docXML = content
			case "word/styles.xml":
				stylesXML = content
			case "word/numbering.xml":
				numberingXML = content
			}
		}
	}
	if docXML == nil {
		return nil, fmt.Errorf("document.xml not found in docx")
	}
	text, err := extractTextFromDocxXML(docXML, stylesXML, numberingXML)
	if err != nil {
		return nil, fmt.Errorf("parse docx xml: %w", err)
	}
	return &MarkdownContent{Text: text}, nil
}

type docxDocument struct {
	Paragraphs []docxParagraph `xml:"body>p"`
}

type docxParagraph struct {
	InnerXML string `xml:",innerxml"`
}

type docxStyles struct {
	Styles []docxStyle `xml:"style"`
}

type docxStyle struct {
	ID      string       `xml:"styleId,attr"`
	Name    docxVal      `xml:"name"`
	BasedOn docxVal      `xml:"basedOn"`
	PPr     docxStylePPr `xml:"pPr"`
}

type docxStylePPr struct {
	OutlineLvl docxVal `xml:"outlineLvl"`
}

type docxNumbering struct {
	AbstractNums []docxAbstractNum `xml:"abstractNum"`
	Nums         []docxNum         `xml:"num"`
}

type docxAbstractNum struct {
	ID     string         `xml:"abstractNumId,attr"`
	Levels []docxNumLevel `xml:"lvl"`
}

type docxNumLevel struct {
	Level  string  `xml:"ilvl,attr"`
	NumFmt docxVal `xml:"numFmt"`
}

type docxNum struct {
	ID            string  `xml:"numId,attr"`
	AbstractNumID docxVal `xml:"abstractNumId"`
}

type docxVal struct {
	Val string `xml:"val,attr"`
}

type parsedParagraph struct {
	Text            string
	StyleID         string
	NumID           string
	ListLevel       int
	HasList         bool
	HasPageBreak    bool
	HasSectionBreak bool
}

type docxContext struct {
	headingLevels map[string]int
	numberFormats map[string]map[int]string
	listCounters  map[string][]int
}

func extractTextFromDocxXML(docXML, stylesXML, numberingXML []byte) (string, error) {
	var doc docxDocument
	if err := xml.Unmarshal(docXML, &doc); err != nil {
		return "", err
	}

	ctx := docxContext{
		headingLevels: parseHeadingLevels(stylesXML),
		numberFormats: parseNumberFormats(numberingXML),
		listCounters:  make(map[string][]int),
	}

	var blocks []string
	for _, raw := range doc.Paragraphs {
		paragraph, err := parseParagraph(raw.InnerXML)
		if err != nil {
			return "", err
		}

		text := strings.TrimSpace(paragraph.Text)
		if text != "" {
			switch {
			case ctx.headingLevel(paragraph.StyleID) > 0:
				level := ctx.headingLevel(paragraph.StyleID)
				blocks = append(blocks, strings.Repeat("#", level)+" "+text)
			case paragraph.HasList:
				blocks = append(blocks, ctx.formatListItem(paragraph, text))
			default:
				blocks = append(blocks, text)
			}
		}

		// Form-feed is intentionally preserved because the chunker already
		// treats it as a strong heuristic boundary.
		if paragraph.HasPageBreak || paragraph.HasSectionBreak {
			if len(blocks) == 0 || blocks[len(blocks)-1] != "\f" {
				blocks = append(blocks, "\f")
			}
		}
	}

	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

func parseHeadingLevels(stylesXML []byte) map[string]int {
	levels := make(map[string]int)
	if len(stylesXML) == 0 {
		return levels
	}

	var styles docxStyles
	if err := xml.Unmarshal(stylesXML, &styles); err != nil {
		return levels
	}

	for _, style := range styles.Styles {
		if level, ok := headingLevelFromStyle(style); ok {
			levels[style.ID] = level
		}
	}

	// If a style inherits from a heading style, let it inherit the level too.
	for changed := true; changed; {
		changed = false
		for _, style := range styles.Styles {
			if _, exists := levels[style.ID]; exists || style.BasedOn.Val == "" {
				continue
			}
			if level, ok := levels[style.BasedOn.Val]; ok {
				levels[style.ID] = level
				changed = true
			}
		}
	}
	return levels
}

func headingLevelFromStyle(style docxStyle) (int, bool) {
	if style.PPr.OutlineLvl.Val != "" {
		if n, err := strconv.Atoi(style.PPr.OutlineLvl.Val); err == nil && n >= 0 && n < 6 {
			return n + 1, true
		}
	}

	candidates := []string{style.ID, style.Name.Val}
	for _, candidate := range candidates {
		lower := strings.ToLower(strings.ReplaceAll(candidate, " ", ""))
		if strings.HasPrefix(lower, "heading") {
			if n, err := strconv.Atoi(strings.TrimPrefix(lower, "heading")); err == nil && n >= 1 && n <= 6 {
				return n, true
			}
		}
		if strings.HasPrefix(candidate, "标题") {
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(candidate, "标题"))); err == nil && n >= 1 && n <= 6 {
				return n, true
			}
		}
	}
	return 0, false
}

func parseNumberFormats(numberingXML []byte) map[string]map[int]string {
	formats := make(map[string]map[int]string)
	if len(numberingXML) == 0 {
		return formats
	}

	var numbering docxNumbering
	if err := xml.Unmarshal(numberingXML, &numbering); err != nil {
		return formats
	}

	abstractFormats := make(map[string]map[int]string)
	for _, abstractNum := range numbering.AbstractNums {
		levelFormats := make(map[int]string)
		for _, level := range abstractNum.Levels {
			if ilvl, err := strconv.Atoi(level.Level); err == nil {
				levelFormats[ilvl] = level.NumFmt.Val
			}
		}
		abstractFormats[abstractNum.ID] = levelFormats
	}
	for _, num := range numbering.Nums {
		if levelFormats, ok := abstractFormats[num.AbstractNumID.Val]; ok {
			formats[num.ID] = levelFormats
		}
	}
	return formats
}

func parseParagraph(innerXML string) (parsedParagraph, error) {
	var paragraph parsedParagraph
	decoder := xml.NewDecoder(strings.NewReader("<p>" + innerXML + "</p>"))

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return paragraph, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "pStyle":
			paragraph.StyleID = attrValue(start.Attr, "val")
		case "numId":
			paragraph.NumID = attrValue(start.Attr, "val")
			paragraph.HasList = paragraph.NumID != ""
		case "ilvl":
			if n, err := strconv.Atoi(attrValue(start.Attr, "val")); err == nil {
				paragraph.ListLevel = n
			}
		case "sectPr":
			paragraph.HasSectionBreak = true
		case "br":
			if attrValue(start.Attr, "type") == "page" {
				paragraph.HasPageBreak = true
			}
		case "tab":
			paragraph.Text += "\t"
		case "t":
			var text string
			if err := decoder.DecodeElement(&text, &start); err != nil {
				return paragraph, err
			}
			paragraph.Text += decodeXMLText(text)
		}
	}
	return paragraph, nil
}

func attrValue(attrs []xml.Attr, localName string) string {
	for _, attr := range attrs {
		if attr.Name.Local == localName {
			return attr.Value
		}
	}
	return ""
}

func (c *docxContext) headingLevel(styleID string) int {
	return c.headingLevels[styleID]
}

func (c *docxContext) formatListItem(paragraph parsedParagraph, text string) string {
	indent := strings.Repeat("  ", paragraph.ListLevel)
	format := c.numberFormats[paragraph.NumID][paragraph.ListLevel]
	if format == "bullet" {
		return indent + "- " + text
	}

	counters := c.listCounters[paragraph.NumID]
	if len(counters) <= paragraph.ListLevel {
		counters = append(counters, make([]int, paragraph.ListLevel-len(counters)+1)...)
	}
	counters[paragraph.ListLevel]++
	for i := paragraph.ListLevel + 1; i < len(counters); i++ {
		counters[i] = 0
	}
	c.listCounters[paragraph.NumID] = counters
	return fmt.Sprintf("%s%d. %s", indent, counters[paragraph.ListLevel], text)
}

func decodeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&apos;", "'")
	return s
}
