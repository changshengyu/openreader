package chapterimage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type imageReference struct {
	URL string
	Key string
}

func extractImageReferences(content, chapterURL string, maxImages int) []imageReference {
	if maxImages <= 0 || strings.TrimSpace(content) == "" {
		return nil
	}
	base, _ := url.Parse(strings.TrimSpace(chapterURL))
	tokenizer := html.NewTokenizer(strings.NewReader(content))
	seen := make(map[string]struct{})
	references := make([]imageReference, 0)
	for len(references) < maxImages {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if !strings.EqualFold(token.Data, "img") {
			continue
		}
		raw := firstImageAttribute(token.Attr, "src", "data-src", "data-original", "data-url")
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if !parsed.IsAbs() {
			if base == nil || base.Scheme == "" || base.Host == "" {
				continue
			}
			parsed = base.ResolveReference(parsed)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		if parsed.Host == "" || parsed.User != nil {
			continue
		}
		parsed.Fragment = ""
		normalized := parsed.String()
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		references = append(references, imageReference{URL: normalized, Key: imageKey(normalized)})
	}
	return references
}

func firstImageAttribute(attributes []html.Attribute, names ...string) string {
	for _, name := range names {
		for _, attribute := range attributes {
			if strings.EqualFold(strings.TrimSpace(attribute.Key), name) {
				if value := strings.TrimSpace(attribute.Val); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func imageKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validImageKey(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
