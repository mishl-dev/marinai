package media

type ImageInfo struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
}

type PDFInfo struct {
	PageCount int    `json:"page_count"`
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
}

type TranscriptionResult struct {
	Text     string                 `json:"text"`
	Language string                 `json:"language"`
	Duration float64                `json:"duration_seconds"`
	Segments []TranscriptionSegment `json:"segments,omitempty"`
}

type TranscriptionSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type CompressionOptions struct {
	Quality   int   `json:"quality"`
	MaxWidth  int   `json:"max_width"`
	MaxHeight int   `json:"max_height"`
	Threshold int64 `json:"threshold"`
}

func DefaultCompressionOptions() CompressionOptions {
	return CompressionOptions{
		Quality:   75,
		MaxWidth:  1920,
		MaxHeight: 1080,
		Threshold: 1024 * 1024,
	}
}
