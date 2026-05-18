package parser

import (
	"strings"
	"testing"
)

func TestExtractTextFromDocxXMLPreservesHeadingListAndBoundaries(t *testing.T) {
	docXML := []byte(`
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>第一章 总览</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>
      <w:r><w:t>准备环境</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="1"/></w:numPr></w:pPr>
      <w:r><w:t>安装依赖</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr>
      <w:r><w:t>检查配置</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>分页前</w:t></w:r>
      <w:r><w:br w:type="page"/></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:sectPr/></w:pPr>
      <w:r><w:t>分节前</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`)
	stylesXML := []byte(`
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:pPr><w:outlineLvl w:val="0"/></w:pPr>
  </w:style>
</w:styles>`)
	numberingXML := []byte(`
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="10">
    <w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl>
    <w:lvl w:ilvl="1"><w:numFmt w:val="decimal"/></w:lvl>
  </w:abstractNum>
  <w:abstractNum w:abstractNumId="20">
    <w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl>
  </w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="10"/></w:num>
  <w:num w:numId="2"><w:abstractNumId w:val="20"/></w:num>
</w:numbering>`)

	got, err := extractTextFromDocxXML(docXML, stylesXML, numberingXML)
	if err != nil {
		t.Fatalf("extractTextFromDocxXML returned error: %v", err)
	}

	wantParts := []string{
		"# 第一章 总览",
		"1. 准备环境",
		"  1. 安装依赖",
		"- 检查配置",
		"分页前",
		"\f",
		"分节前",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("expected output to contain %q, got:\n%s", part, got)
		}
	}
}
