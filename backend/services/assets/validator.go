package assets

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
)

var (
	ErrContentMismatch = errors.New("file content does not match type")
	ErrImageDimensions = errors.New("image dimensions are too large")
)

const (
	maxImageDimension = 32_768
	maxImagePixels    = 64_000_000
	maxHeaderBytes    = 1024 * 1024
)

// ValidateUpload verifies that an admitted filename extension matches the uploaded bytes.
// It intentionally inspects only bounded metadata: browser assets do not need to be decoded
// or rewritten by the server, but spoofed/empty/truncated headers must never be published.
func ValidateUpload(input io.Reader, size int64, _ string, extension string) error {
	if input == nil || size <= 0 {
		return ErrContentMismatch
	}
	extension = strings.ToLower(strings.TrimSpace(extension))
	switch extension {
	case ".jpg", ".jpeg", ".png", ".gif":
		return validateDecodedImage(input, extension)
	case ".webp":
		return validateWebP(input, size)
	case ".ttf", ".otf", ".woff", ".woff2":
		return validateFont(input, extension)
	default:
		return ErrContentMismatch
	}
}

func validateDecodedImage(input io.Reader, extension string) error {
	config, format, err := image.DecodeConfig(io.LimitReader(input, maxHeaderBytes))
	if err != nil {
		return ErrContentMismatch
	}
	expected := strings.TrimPrefix(extension, ".")
	if expected == "jpg" {
		expected = "jpeg"
	}
	if format != expected {
		return ErrContentMismatch
	}
	return validateImageDimensions(config.Width, config.Height)
}

func validateWebP(input io.Reader, size int64) error {
	header, err := io.ReadAll(io.LimitReader(input, 64))
	if err != nil || len(header) < 20 ||
		!bytes.Equal(header[0:4], []byte("RIFF")) ||
		!bytes.Equal(header[8:12], []byte("WEBP")) {
		return ErrContentMismatch
	}
	riffSize := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	chunkSize := int64(binary.LittleEndian.Uint32(header[16:20]))
	if riffSize > size || chunkSize < 1 || chunkSize > size-20 {
		return ErrContentMismatch
	}

	var width, height int
	switch string(header[12:16]) {
	case "VP8X":
		if chunkSize < 10 || len(header) < 30 {
			return ErrContentMismatch
		}
		width = 1 + int(header[24]) + int(header[25])<<8 + int(header[26])<<16
		height = 1 + int(header[27]) + int(header[28])<<8 + int(header[29])<<16
	case "VP8L":
		if chunkSize < 5 || len(header) < 25 || header[20] != 0x2f {
			return ErrContentMismatch
		}
		bits := binary.LittleEndian.Uint32(header[21:25])
		width = int(bits&0x3fff) + 1
		height = int((bits>>14)&0x3fff) + 1
	case "VP8 ":
		if chunkSize < 10 || len(header) < 30 || !bytes.Equal(header[23:26], []byte{0x9d, 0x01, 0x2a}) {
			return ErrContentMismatch
		}
		width = int(binary.LittleEndian.Uint16(header[26:28]) & 0x3fff)
		height = int(binary.LittleEndian.Uint16(header[28:30]) & 0x3fff)
	default:
		return ErrContentMismatch
	}
	return validateImageDimensions(width, height)
}

func validateFont(input io.Reader, extension string) error {
	header := make([]byte, 12)
	if _, err := io.ReadFull(input, header); err != nil {
		return ErrContentMismatch
	}
	valid := false
	switch extension {
	case ".ttf":
		valid = bytes.Equal(header[:4], []byte{0x00, 0x01, 0x00, 0x00}) ||
			bytes.Equal(header[:4], []byte("true")) ||
			bytes.Equal(header[:4], []byte("typ1")) ||
			bytes.Equal(header[:4], []byte("ttcf"))
	case ".otf":
		valid = bytes.Equal(header[:4], []byte("OTTO"))
	case ".woff":
		valid = bytes.Equal(header[:4], []byte("wOFF"))
	case ".woff2":
		valid = bytes.Equal(header[:4], []byte("wOF2"))
	}
	if !valid {
		return ErrContentMismatch
	}
	return nil
}

func validateImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return ErrContentMismatch
	}
	if width > maxImageDimension || height > maxImageDimension ||
		int64(width)*int64(height) > maxImagePixels {
		return ErrImageDimensions
	}
	return nil
}
