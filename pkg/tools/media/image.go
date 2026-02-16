package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/disintegration/imaging"
)

type ImageProcessor struct {
	opts CompressionOptions
}

func NewImageProcessor(opts CompressionOptions) *ImageProcessor {
	return &ImageProcessor{opts: opts}
}

func (p *ImageProcessor) Compress(data []byte) ([]byte, *ImageInfo, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	info := &ImageInfo{
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		Format:    format,
		SizeBytes: int64(len(data)),
	}

	if int64(len(data)) < p.opts.Threshold {
		return data, info, nil
	}

	if p.opts.MaxWidth > 0 || p.opts.MaxHeight > 0 {
		img = p.resizeImage(img)
		info.Width = img.Bounds().Dx()
		info.Height = img.Bounds().Dy()
	}

	compressed, err := p.encodeImage(img, format)
	if err != nil {
		return nil, nil, fmt.Errorf("encode image: %w", err)
	}

	info.SizeBytes = int64(len(compressed))

	return compressed, info, nil
}

func (p *ImageProcessor) resizeImage(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	targetWidth := width
	targetHeight := height

	if p.opts.MaxWidth > 0 && width > p.opts.MaxWidth {
		ratio := float64(p.opts.MaxWidth) / float64(width)
		targetWidth = p.opts.MaxWidth
		targetHeight = int(float64(height) * ratio)
	}

	if p.opts.MaxHeight > 0 && targetHeight > p.opts.MaxHeight {
		ratio := float64(p.opts.MaxHeight) / float64(targetHeight)
		targetHeight = p.opts.MaxHeight
		targetWidth = int(float64(targetWidth) * ratio)
	}

	if targetWidth != width || targetHeight != height {
		return imaging.Resize(img, targetWidth, targetHeight, imaging.Lanczos)
	}

	return img
}

func (p *ImageProcessor) encodeImage(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer

	switch format {
	case "jpeg", "jpg":
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: p.opts.Quality})
		return buf.Bytes(), err

	case "png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err := encoder.Encode(&buf, img)
		return buf.Bytes(), err

	case "gif":
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: p.opts.Quality})
		return buf.Bytes(), err

	case "webp":
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: p.opts.Quality})
		return buf.Bytes(), err

	default:
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: p.opts.Quality})
		return buf.Bytes(), err
	}
}

func (p *ImageProcessor) ResizeImage(data []byte, width, height int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: p.opts.Quality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (p *ImageProcessor) Thumbnail(data []byte, size int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	thumb := imaging.Thumbnail(img, size, size, imaging.Lanczos)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: p.opts.Quality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
