package media

import (
	"strings"
)

func DetectAudioFormat(filename string) string {
	ext := strings.ToLower(filename)

	switch {
	case strings.HasSuffix(ext, ".mp3"):
		return "mp3"
	case strings.HasSuffix(ext, ".wav"):
		return "wav"
	case strings.HasSuffix(ext, ".m4a"):
		return "m4a"
	case strings.HasSuffix(ext, ".ogg"):
		return "ogg"
	case strings.HasSuffix(ext, ".flac"):
		return "flac"
	case strings.HasSuffix(ext, ".aac"):
		return "aac"
	case strings.HasSuffix(ext, ".webm"):
		return "webm"
	default:
		return ""
	}
}

func DetectImageFormat(filename string) string {
	ext := strings.ToLower(filename)

	switch {
	case strings.HasSuffix(ext, ".jpg") || strings.HasSuffix(ext, ".jpeg"):
		return "jpeg"
	case strings.HasSuffix(ext, ".png"):
		return "png"
	case strings.HasSuffix(ext, ".gif"):
		return "gif"
	case strings.HasSuffix(ext, ".webp"):
		return "webp"
	case strings.HasSuffix(ext, ".bmp"):
		return "bmp"
	default:
		return ""
	}
}

func IsAudioFile(filename string) bool {
	return DetectAudioFormat(filename) != ""
}

func IsImageFile(filename string) bool {
	return DetectImageFormat(filename) != ""
}

func IsPDFFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".pdf")
}

func GetSupportedAudioFormats() []string {
	return []string{"mp3", "wav", "m4a", "ogg", "flac", "aac", "webm"}
}

func GetSupportedImageFormats() []string {
	return []string{"jpeg", "png", "gif", "webp", "bmp"}
}
