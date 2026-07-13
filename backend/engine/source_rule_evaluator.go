package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PaesslerAG/jsonpath"
	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

// ErrUnsupportedSourceRule distinguishes a preserved upstream rule that cannot
// safely run in the Go process from a selector that simply has no matches.
var ErrUnsupportedSourceRule = errors.New("unsupported book source rule")

type sourceRuleDocument struct {
	raw       string
	document  *goquery.Document
	xpathRoot *html.Node

	jsonOnce sync.Once
	jsonData any
	jsonErr  error
}

type sourceRuleValue struct {
	document  *sourceRuleDocument
	selection *goquery.Selection
	xpathNode *html.Node
	jsonData  any
	text      string
	captures  []string
}

func newSourceRuleDocument(raw string) (*sourceRuleDocument, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}
	xpathRoot, err := htmlquery.Parse(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return &sourceRuleDocument{raw: raw, document: document, xpathRoot: xpathRoot}, nil
}

func fetchSourceRuleDocumentContext(ctx context.Context, request sourceRequest) (*sourceRuleDocument, sourceRequest, error) {
	raw, responseURL, err := FetchSourceTextWithURLContext(ctx, request)
	if responseURL != "" {
		request.URL = responseURL
	}
	if err != nil {
		return nil, request, err
	}
	document, err := newSourceRuleDocument(raw)
	return document, request, err
}

func (d *sourceRuleDocument) Root() sourceRuleValue {
	return sourceRuleValue{document: d, selection: d.document.Selection, xpathNode: d.xpathRoot}
}

func (d *sourceRuleDocument) jsonValue() (any, error) {
	d.jsonOnce.Do(func() {
		d.jsonErr = json.Unmarshal([]byte(d.raw), &d.jsonData)
	})
	if d.jsonErr != nil {
		return nil, fmt.Errorf("decode JSON source response: %w", d.jsonErr)
	}
	return d.jsonData, nil
}

func sourceRuleElements(value sourceRuleValue, rule string) ([]sourceRuleValue, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return nil, nil
	}
	if sourceRuleUsesJavaScript(rule) {
		return nil, fmt.Errorf("%w: JavaScript/WebJS execution is disabled", ErrUnsupportedSourceRule)
	}
	if strings.HasPrefix(rule, ":") {
		return sourceRuleRegexElements(value, strings.TrimSpace(rule[1:]))
	}
	if jsonPath, ok := sourceRuleJSONPath(rule); ok {
		return sourceRuleJSONElements(value, jsonPath)
	}
	if xpath, ok := sourceRuleXPath(rule); ok {
		return sourceRuleXPathElements(value, xpath)
	}
	return sourceRuleCSSElements(value, sourceRuleCSSSelector(rule))
}

func sourceRuleStrings(value sourceRuleValue, rule string) ([]string, error) {
	items, err := sourceRuleElements(value, rule)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		parsed, err := sourceRuleValueString(item, rule)
		if err != nil {
			return nil, err
		}
		if parsed != "" {
			values = append(values, parsed)
		}
	}
	return values, nil
}

func sourceRuleString(value sourceRuleValue, rule string) (string, error) {
	if capture, ok := sourceRuleCapture(rule); ok {
		if capture < len(value.captures) {
			return value.captures[capture], nil
		}
		return "", nil
	}
	values, err := sourceRuleStrings(value, rule)
	if err != nil || len(values) == 0 {
		return "", err
	}
	return values[0], nil
}

func sourceRuleValueString(value sourceRuleValue, rule string) (string, error) {
	if value.text != "" || value.captures != nil {
		return strings.TrimSpace(value.text), nil
	}
	if value.jsonData != nil {
		return strings.TrimSpace(sourceRuleJSONText(value.jsonData)), nil
	}
	operation := sourceRuleCSSOperation(rule)
	if value.selection != nil {
		switch {
		case operation == "html":
			htmlValue, err := value.selection.Html()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(htmlValue), nil
		case strings.HasPrefix(operation, "attr:"):
			attribute := strings.TrimSpace(strings.TrimPrefix(operation, "attr:"))
			if attribute == "" {
				return "", nil
			}
			matched, found := value.selection.Attr(attribute)
			if !found {
				return "", nil
			}
			return strings.TrimSpace(matched), nil
		default:
			return strings.TrimSpace(value.selection.Text()), nil
		}
	}
	if value.xpathNode != nil {
		if operation == "html" {
			return strings.TrimSpace(htmlquery.OutputHTML(value.xpathNode, false)), nil
		}
		return strings.TrimSpace(htmlquery.InnerText(value.xpathNode)), nil
	}
	return "", nil
}

func sourceRuleCSSElements(value sourceRuleValue, selector string) ([]sourceRuleValue, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil
	}
	selection := value.selection
	if selection == nil && value.xpathNode != nil {
		selection = goquery.NewDocumentFromNode(value.xpathNode).Selection
	}
	if selection == nil {
		return nil, fmt.Errorf("CSS rule cannot be evaluated against a non-HTML value")
	}
	items := make([]sourceRuleValue, 0)
	selection.Find(selector).Each(func(_ int, item *goquery.Selection) {
		var node *html.Node
		if item.Length() > 0 {
			node = item.Get(0)
		}
		items = append(items, sourceRuleValue{document: value.document, selection: item, xpathNode: node})
	})
	return items, nil
}

func sourceRuleJSONElements(value sourceRuleValue, path string) ([]sourceRuleValue, error) {
	input := value.jsonData
	if input == nil {
		if value.document == nil {
			return nil, fmt.Errorf("JSONPath rule has no source document")
		}
		decoded, err := value.document.jsonValue()
		if err != nil {
			return nil, err
		}
		input = decoded
	}
	matched, err := jsonpath.Get(path, input)
	if err != nil {
		// The upstream analyzer treats an absent optional JSON field as no match.
		// Paessler's JSONPath reports that normal case as "unknown key"; keep
		// malformed expressions as errors so source authors can repair them.
		if strings.Contains(strings.ToLower(err.Error()), "unknown key") {
			return nil, nil
		}
		return nil, fmt.Errorf("parse JSONPath rule: %w", err)
	}
	values := sourceRuleFlattenJSON(matched)
	items := make([]sourceRuleValue, 0, len(values))
	for _, item := range values {
		items = append(items, sourceRuleValue{document: value.document, jsonData: item})
	}
	return items, nil
}

func sourceRuleFlattenJSON(value any) []any {
	if value == nil {
		return nil
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

func sourceRuleXPathElements(value sourceRuleValue, expression string) ([]sourceRuleValue, error) {
	node := value.xpathNode
	if node == nil && value.document != nil {
		node = value.document.xpathRoot
	}
	if node == nil {
		return nil, fmt.Errorf("XPath rule cannot be evaluated against a non-HTML value")
	}
	matched, err := htmlquery.QueryAll(node, expression)
	if err != nil {
		return nil, fmt.Errorf("parse XPath rule: %w", err)
	}
	items := make([]sourceRuleValue, 0, len(matched))
	for _, item := range matched {
		items = append(items, sourceRuleValue{document: value.document, xpathNode: item})
	}
	return items, nil
}

func sourceRuleRegexElements(value sourceRuleValue, expression string) ([]sourceRuleValue, error) {
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("parse regex rule: %w", err)
	}
	matches := compiled.FindAllStringSubmatch(sourceRuleRaw(value), -1)
	items := make([]sourceRuleValue, 0, len(matches))
	for _, match := range matches {
		if len(match) == 0 {
			continue
		}
		text := match[0]
		if len(match) > 1 {
			text = match[1]
		}
		items = append(items, sourceRuleValue{document: value.document, text: text, captures: match})
	}
	return items, nil
}

func sourceRuleRaw(value sourceRuleValue) string {
	if value.text != "" {
		return value.text
	}
	if value.jsonData != nil {
		data, err := json.Marshal(value.jsonData)
		if err == nil {
			return string(data)
		}
	}
	if value.selection != nil {
		htmlValue, err := value.selection.Html()
		if err == nil {
			return htmlValue
		}
	}
	if value.xpathNode != nil {
		return htmlquery.OutputHTML(value.xpathNode, true)
	}
	if value.document != nil {
		return value.document.raw
	}
	return ""
}

func sourceRuleJSONText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func sourceRuleJSONPath(rule string) (string, bool) {
	trimmed := strings.TrimSpace(rule)
	if len(trimmed) >= len("@json:") && strings.EqualFold(trimmed[:len("@json:")], "@json:") {
		return strings.TrimSpace(trimmed[len("@json:"):]), true
	}
	return trimmed, strings.HasPrefix(trimmed, "$.") || strings.HasPrefix(trimmed, "$[")
}

func sourceRuleXPath(rule string) (string, bool) {
	trimmed := strings.TrimSpace(rule)
	if len(trimmed) >= len("@xpath:") && strings.EqualFold(trimmed[:len("@xpath:")], "@xpath:") {
		return strings.TrimSpace(trimmed[len("@xpath:"):]), true
	}
	// A bare relative URL is a valid source field value. Only the upstream's
	// unambiguous // shorthand is inferred as XPath; absolute XPath must use
	// the explicit @XPath: prefix.
	return trimmed, strings.HasPrefix(trimmed, "//")
}

func sourceRuleCSSSelector(rule string) string {
	trimmed := strings.TrimSpace(rule)
	if len(trimmed) >= len("@css:") && strings.EqualFold(trimmed[:len("@css:")], "@css:") {
		trimmed = strings.TrimSpace(trimmed[len("@css:"):])
	}
	if before, _, found := strings.Cut(trimmed, "|"); found {
		return strings.TrimSpace(before)
	}
	if at := strings.LastIndex(trimmed, "@"); at > 0 {
		operation := strings.TrimSpace(trimmed[at+1:])
		if sourceRuleCSSAtOperation(operation) {
			return strings.TrimSpace(trimmed[:at])
		}
	}
	return trimmed
}

func sourceRuleCSSOperation(rule string) string {
	trimmed := strings.TrimSpace(rule)
	if before, after, found := strings.Cut(trimmed, "|"); found && strings.TrimSpace(before) != "" {
		return strings.ToLower(strings.TrimSpace(after))
	}
	if at := strings.LastIndex(trimmed, "@"); at > 0 {
		operation := strings.TrimSpace(trimmed[at+1:])
		if sourceRuleCSSAtOperation(operation) {
			switch strings.ToLower(operation) {
			case "text":
				return "text"
			case "html":
				return "html"
			default:
				return "attr:" + operation
			}
		}
	}
	return "text"
}

func sourceRuleCSSAtOperation(value string) bool {
	if value == "" || strings.ContainsAny(value, " /|@[](){}") {
		return false
	}
	return true
}

func sourceRuleCapture(rule string) (int, bool) {
	if len(rule) < 2 || rule[0] != '$' {
		return 0, false
	}
	value, err := strconv.Atoi(rule[1:])
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func sourceRuleUsesJavaScript(rule string) bool {
	lower := strings.ToLower(strings.TrimSpace(rule))
	return strings.HasPrefix(lower, "@js:") || strings.Contains(lower, "<js>")
}

func sourceRuleNeedsEvaluator(rule string) bool {
	trimmed := strings.TrimSpace(rule)
	if trimmed == "" {
		return false
	}
	if sourceRuleUsesJavaScript(trimmed) || strings.HasPrefix(trimmed, ":") ||
		(len(trimmed) >= len("@css:") && strings.EqualFold(trimmed[:len("@css:")], "@css:")) {
		return true
	}
	_, isJSON := sourceRuleJSONPath(trimmed)
	if isJSON {
		return true
	}
	_, isXPath := sourceRuleXPath(trimmed)
	return isXPath
}
