package assets

import (
	"path/filepath"
	"strings"
)

const (
	MaxImageBytes int64 = 8 * 1024 * 1024
	MaxFontBytes  int64 = 32 * 1024 * 1024
)

// KindDirectory converts the public upload type into its persisted directory name.
func KindDirectory(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "cover", "covers":
		return "covers"
	case "background", "backgrounds":
		return "backgrounds"
	case "font", "fonts":
		return "fonts"
	default:
		return "misc"
	}
}

func IsKindDirectory(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "covers", "backgrounds", "fonts", "misc":
		return true
	default:
		return false
	}
}

func SizeLimitForKind(kind string) int64 {
	if KindDirectory(kind) == "fonts" {
		return MaxFontBytes
	}
	return MaxImageBytes
}

func AllowedExtension(kind, extension string) bool {
	kind = KindDirectory(kind)
	extension = strings.ToLower(strings.TrimSpace(filepath.Ext("asset" + extension)))
	image := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	cover := map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
	font := map[string]bool{".ttf": true, ".otf": true, ".woff": true, ".woff2": true}
	switch kind {
	case "covers":
		return cover[extension]
	case "backgrounds":
		return image[extension]
	case "fonts":
		return font[extension]
	default:
		return image[extension] || font[extension]
	}
}
