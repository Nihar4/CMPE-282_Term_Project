package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	data := []byte("name,role\nAlice,Engineer\nBob,Manager\n")
	chunks, rows, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parseCSV returned error: %v", err)
	}
	if rows != 2 {
		t.Fatalf("expected 2 rows, got %d", rows)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected at least one chunk")
	}
	if !strings.Contains(chunks[0], "Alice") {
		t.Fatalf("expected chunk to include CSV content, got: %q", chunks[0])
	}
}

func TestParseTXTHandlesVeryLongLines(t *testing.T) {
	long := strings.Repeat("A", 200000)
	chunks, err := parseTXT([]byte(long))
	if err != nil {
		t.Fatalf("parseTXT returned error for long text: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks for long text")
	}
}

func TestParseDocx(t *testing.T) {
	docxData, err := makeDocx("Hello from DOCX")
	if err != nil {
		t.Fatalf("failed to build test docx: %v", err)
	}
	chunks, err := parseDocx(docxData)
	if err != nil {
		t.Fatalf("parseDocx returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected docx chunks")
	}
	if !strings.Contains(strings.Join(chunks, " "), "Hello from DOCX") {
		t.Fatalf("expected extracted DOCX text, got: %q", strings.Join(chunks, " "))
	}
}

func TestParsePDFNeverReturnsEmptyChunks(t *testing.T) {
	// This is not a full valid PDF; parser should still return fallback content.
	pdfLike := []byte("%PDF-1.4\nstream\nBinary\x00\x01\x02Data\nendstream\n")
	chunks, err := parsePDF(pdfLike)
	if err != nil {
		t.Fatalf("parsePDF returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected at least one chunk for fallback PDF parse")
	}
}

func makeDocx(text string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		return nil, err
	}
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>
  </w:body>
</w:document>`
	if _, err := f.Write([]byte(xml)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
