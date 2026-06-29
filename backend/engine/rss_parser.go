package engine

import (
	stdhtml "html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

type RSSRuleSet struct {
	Articles    string
	Title       string
	PubDate     string
	Description string
	Image       string
	Link        string
	LinkBaseURL string
}

type RSSRuleArticle struct {
	Title       string
	PubDate     string
	Description string
	Image       string
	Link        string
}

type RSSRulePage struct {
	Articles []RSSRuleArticle
	NextURL  string
}

var (
	simpleXPathAttrPattern     = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\[@([a-zA-Z0-9_-]+)=['"]([^'"]+)['"]\]$`)
	simpleXPathContainsPattern = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\[contains\(@([a-zA-Z0-9_-]+),\s*['"]([^'"]+)['"]\)\]$`)
)

func ParseRSSRuleArticles(body string, baseURL string, rules RSSRuleSet) ([]RSSRuleArticle, error) {
	page, err := ParseRSSRulePage(body, baseURL, rules, "")
	return page.Articles, err
}

func ParseRSSRulePage(body string, baseURL string, rules RSSRuleSet, nextPageRule string) (RSSRulePage, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return RSSRulePage{}, err
	}
	articleRule := strings.TrimSpace(rules.Articles)
	reverse := strings.HasPrefix(articleRule, "-")
	articleRule = strings.TrimSpace(strings.TrimPrefix(articleRule, "-"))
	selector := rssCSSSelector(articleRule)
	if selector == "" {
		return RSSRulePage{}, nil
	}
	linkBaseURL := strings.TrimSpace(rules.LinkBaseURL)
	if linkBaseURL == "" {
		linkBaseURL = baseURL
	}
	articles := make([]RSSRuleArticle, 0)
	document.Find(selector).Each(func(_ int, item *goquery.Selection) {
		article := RSSRuleArticle{
			Title:       rssRuleValue(item, rules.Title, false),
			PubDate:     rssRuleValue(item, rules.PubDate, false),
			Description: rssRuleValue(item, rules.Description, true),
			Image:       resolveRSSURL(baseURL, rssRuleValue(item, rules.Image, false)),
			Link:        resolveRSSRequestURL(linkBaseURL, rssRuleValue(item, rules.Link, false)),
		}
		article.Title = strings.TrimSpace(article.Title)
		if article.Title != "" {
			articles = append(articles, article)
		}
	})
	if reverse {
		for left, right := 0, len(articles)-1; left < right; left, right = left+1, right-1 {
			articles[left], articles[right] = articles[right], articles[left]
		}
	}
	nextURL := ""
	nextPageRule = strings.TrimSpace(nextPageRule)
	if strings.EqualFold(nextPageRule, "PAGE") {
		nextURL = baseURL
	} else if nextPageRule != "" {
		nextURL = resolveRSSRequestURL(baseURL, rssRuleValue(document.Selection, nextPageRule, false))
	}
	return RSSRulePage{Articles: articles, NextURL: nextURL}, nil
}

func ExtractRSSRuleContent(body string, baseURL string, rule string) (string, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", err
	}
	value := rssRuleValue(document.Selection, rule, true)
	return SanitizeRSSHTML(value, baseURL), nil
}

func SanitizeRSSHTML(value string, baseURL string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader("<div id=\"openreader-rss-root\">" + value + "</div>"))
	if err != nil {
		return stdhtml.EscapeString(value)
	}
	root := document.Find("#openreader-rss-root").First()
	root.Find("script,style,iframe,object,embed,form,input,button,textarea,select,meta,link").Remove()
	root.Find("*").Each(func(_ int, selection *goquery.Selection) {
		node := selection.Get(0)
		if node == nil {
			return
		}
		for _, attr := range append([]nethtml.Attribute(nil), node.Attr...) {
			name := strings.ToLower(attr.Key)
			if strings.HasPrefix(name, "on") || name == "style" || name == "srcdoc" {
				selection.RemoveAttr(attr.Key)
			}
		}
		for _, name := range []string{"href", "src"} {
			raw, exists := selection.Attr(name)
			if !exists {
				continue
			}
			resolved := resolveRSSURL(baseURL, raw)
			if resolved == "" {
				selection.RemoveAttr(name)
			} else {
				selection.SetAttr(name, resolved)
			}
		}
	})
	html, err := root.Html()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(html)
}

func ExtractRSSFirstImage(value string, baseURL string) string {
	return resolveRSSURL(baseURL, ExtractRSSFirstImageSource(value))
}

func ExtractRSSFirstImageSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader("<div id=\"openreader-rss-image-root\">" + value + "</div>"))
	if err != nil {
		return ""
	}
	image := document.Find("#openreader-rss-image-root img[src]").First()
	if image.Length() == 0 {
		return ""
	}
	src, _ := image.Attr("src")
	return strings.TrimSpace(src)
}

func rssRuleValue(scope *goquery.Selection, rule string, preferHTML bool) string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return ""
	}
	selector, operation := splitRSSRule(rule, preferHTML)
	target := scope
	if selector != "" && selector != "." {
		cssSelector := rssCSSSelector(selector)
		if cssSelector == "" {
			return ""
		}
		target = scope.Find(cssSelector).First()
	}
	if target.Length() == 0 {
		return ""
	}
	switch {
	case operation == "html":
		value, _ := target.Html()
		return strings.TrimSpace(value)
	case strings.HasPrefix(operation, "attr:"):
		value, _ := target.Attr(strings.TrimSpace(strings.TrimPrefix(operation, "attr:")))
		return strings.TrimSpace(value)
	default:
		value := strings.TrimSpace(target.Text())
		if value == "" && target.Get(0) != nil && strings.EqualFold(target.Get(0).Data, "link") {
			for sibling := target.Get(0).NextSibling; sibling != nil; sibling = sibling.NextSibling {
				if sibling.Type == nethtml.TextNode {
					if text := strings.TrimSpace(sibling.Data); text != "" {
						return text
					}
					continue
				}
				break
			}
		}
		return value
	}
}

func splitRSSRule(rule string, preferHTML bool) (string, string) {
	if strings.HasPrefix(rule, "@") {
		return ".", "attr:" + strings.TrimSpace(strings.TrimPrefix(rule, "@"))
	}
	if index := strings.LastIndex(rule, "|"); index >= 0 {
		return strings.TrimSpace(rule[:index]), strings.TrimSpace(rule[index+1:])
	}
	if index := strings.LastIndex(rule, "@"); index > 0 && !strings.Contains(rule[index:], "]") {
		return strings.TrimSpace(rule[:index]), "attr:" + strings.TrimSpace(rule[index+1:])
	}
	if rule == "text" {
		return ".", "text"
	}
	if rule == "html" {
		return ".", "html"
	}
	if preferHTML {
		return rule, "html"
	}
	return rule, "text"
}

func rssCSSSelector(rule string) string {
	rule = strings.TrimSpace(rule)
	if rule == "" || rule == "." {
		return rule
	}
	if strings.HasPrefix(rule, "//") {
		segments := strings.Split(strings.TrimPrefix(rule, "//"), "/")
		converted := make([]string, 0, len(segments))
		for _, segment := range segments {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			converted = append(converted, rssXPathSegmentToCSS(segment))
		}
		return strings.Join(converted, " ")
	}
	return rule
}

func rssXPathSegmentToCSS(segment string) string {
	if matches := simpleXPathAttrPattern.FindStringSubmatch(segment); len(matches) == 4 {
		return matches[1] + "[" + matches[2] + "=" + strconv.Quote(matches[3]) + "]"
	}
	if matches := simpleXPathContainsPattern.FindStringSubmatch(segment); len(matches) == 4 {
		return matches[1] + "[" + matches[2] + "*=" + strconv.Quote(matches[3]) + "]"
	}
	return segment
}

func resolveRSSURL(baseURL string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return parsed.String()
		}
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}

func resolveRSSRequestURL(baseURL string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	resolved := resolveSourceURLTemplate(baseURL, value)
	urlPart, _ := splitSourceURLOption(resolved)
	parsed, err := url.Parse(urlPart)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return resolved
}
