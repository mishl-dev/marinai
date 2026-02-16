package media

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

func init() {
	license.SetLicenseKey("your-license-key", "Marinai")
}

type PDFParser struct {
	maxPages int
}

func NewPDFParser() *PDFParser {
	return &PDFParser{
		maxPages: 100,
	}
}

func (p *PDFParser) SetMaxPages(max int) {
	if max > 0 {
		p.maxPages = max
	}
}

func (p *PDFParser) ExtractText(data []byte) (string, *PDFInfo, error) {
	reader := bytes.NewReader(data)

	pdfReader, err := model.NewPdfReader(reader)
	if err != nil {
		return "", nil, fmt.Errorf("create PDF reader: %w", err)
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return "", nil, fmt.Errorf("get page count: %w", err)
	}

	info := &PDFInfo{
		PageCount: numPages,
	}

	if meta, err := pdfReader.GetPdfInfo(); err == nil {
		if meta.Title != nil {
			info.Title = meta.Title.String()
		}
		if meta.Author != nil {
			info.Author = meta.Author.String()
		}
	}

	if numPages > p.maxPages {
		numPages = p.maxPages
	}

	var text strings.Builder

	for i := 1; i <= numPages; i++ {
		page, err := pdfReader.GetPage(i)
		if err != nil {
			continue
		}

		pageText, err := p.extractPageText(page)
		if err != nil {
			continue
		}

		text.WriteString(pageText)
		text.WriteString("\n\n")
	}

	return strings.TrimSpace(text.String()), info, nil
}

func (p *PDFParser) extractPageText(page *model.PdfPage) (string, error) {
	ex, err := extractor.New(page)
	if err != nil {
		return "", err
	}

	text, err := ex.ExtractText()
	if err != nil {
		return "", err
	}

	return text, nil
}

func (p *PDFParser) ExtractPageRange(data []byte, start, end int) (string, error) {
	reader := bytes.NewReader(data)

	pdfReader, err := model.NewPdfReader(reader)
	if err != nil {
		return "", fmt.Errorf("create PDF reader: %w", err)
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return "", fmt.Errorf("get page count: %w", err)
	}

	if start < 1 {
		start = 1
	}
	if end > numPages {
		end = numPages
	}

	var text strings.Builder

	for i := start; i <= end; i++ {
		page, err := pdfReader.GetPage(i)
		if err != nil {
			continue
		}

		pageText, err := p.extractPageText(page)
		if err != nil {
			continue
		}

		text.WriteString(fmt.Sprintf("--- Page %d ---\n", i))
		text.WriteString(pageText)
		text.WriteString("\n\n")
	}

	return strings.TrimSpace(text.String()), nil
}
