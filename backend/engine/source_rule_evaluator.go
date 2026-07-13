package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/PaesslerAG/jsonpath"
	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"openreader/backend/models"
)

var (
	// ErrUnsupportedSourceRule distinguishes a preserved upstream rule that
	// cannot safely run in the Go process from a selector that simply has no
	// matches.
	ErrUnsupportedSourceRule = errors.New("unsupported book source rule")
	// ErrInvalidSourceRule marks a local source-rule syntax error. Callers use
	// this to keep author-fixable rules out of remote-source failure caches.
	ErrInvalidSourceRule = errors.New("invalid book source rule")
	sourceRuleGetPattern = regexp.MustCompile(`(?i)@get:\{([^{}]+)\}`)
)

const (
	maxSourceRuleVariables       = models.MaxSourceRuleVariables
	maxSourceRuleVariableKeySize = models.MaxSourceRuleVariableKeySize
	maxSourceRuleVariableValue   = models.MaxSourceRuleVariableValue
	maxSourceRuleVariableBytes   = models.MaxSourceRuleVariableBytes
	maxSourceRuleVariableDepth   = 8
)

// sourceRuleRuntime represents reader-dev's variable lookup order. Temporary
// variables are confined to one parser operation. Book and chapter maps are
// loaded from, and later returned to, explicitly supplied persistent state;
// the runtime itself never writes a database/cache row or crosses requests.
type sourceRuleRuntime struct {
	variables        map[string]string
	bookVariables    map[string]string
	chapterVariables map[string]string
	bookName         string
	chapterTitle     string
	depth            int
}

func newSourceRuleRuntime() *sourceRuleRuntime {
	return &sourceRuleRuntime{variables: make(map[string]string)}
}

func newSourceRuleRuntimeWithBookVariables(raw, bookName string) (*sourceRuleRuntime, error) {
	variables, err := sourceRuleVariableMap(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: persisted book variables", ErrInvalidSourceRule)
	}
	return &sourceRuleRuntime{
		variables:     make(map[string]string),
		bookVariables: variables,
		bookName:      bookName,
	}, nil
}

func (r *sourceRuleRuntime) clone() *sourceRuleRuntime {
	if r == nil {
		return newSourceRuleRuntime()
	}
	return &sourceRuleRuntime{
		variables:        cloneSourceRuleVariableMap(r.variables),
		bookVariables:    cloneSourceRuleVariableMap(r.bookVariables),
		chapterVariables: cloneSourceRuleVariableMap(r.chapterVariables),
		bookName:         r.bookName,
		chapterTitle:     r.chapterTitle,
		depth:            r.depth,
	}
}

func cloneSourceRuleVariableMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// asSearchBookRuntime copies list/request variables into the individual
// search result's persistent book map. This is the same boundary used by
// reader-dev's BookList before it evaluates per-item rules.
func (r *sourceRuleRuntime) asSearchBookRuntime() *sourceRuleRuntime {
	cloned := r.clone()
	cloned.bookVariables = cloneSourceRuleVariableMap(cloned.variables)
	cloned.variables = make(map[string]string)
	cloned.chapterVariables = nil
	cloned.chapterTitle = ""
	return cloned
}

func (r *sourceRuleRuntime) withChapterVariables(raw, title string) (*sourceRuleRuntime, error) {
	if r == nil {
		r = newSourceRuleRuntime()
	}
	chapterVariables, err := sourceRuleVariableMap(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: persisted chapter variables", ErrInvalidSourceRule)
	}
	cloned := r.clone()
	cloned.chapterVariables = chapterVariables
	cloned.chapterTitle = title
	return cloned, nil
}

// NormalizeSourceRuleVariables is the API-safe boundary for values received
// from a search result, a restore archive, or a persisted database row.
func NormalizeSourceRuleVariables(raw string) (string, error) {
	normalized, err := models.NormalizeSourceRuleVariables(raw)
	if err != nil {
		return "", fmt.Errorf("%w: persisted source variables", ErrInvalidSourceRule)
	}
	return normalized, nil
}

func sourceRuleVariableMap(raw string) (map[string]string, error) {
	variables, err := models.SourceRuleVariableMap(raw)
	if err != nil {
		return nil, err
	}
	return variables, nil
}

func (r *sourceRuleRuntime) setBookName(name string) {
	if r != nil && r.bookVariables != nil {
		r.bookName = name
	}
}

func (r *sourceRuleRuntime) setChapterTitle(title string) {
	if r != nil && r.chapterVariables != nil {
		r.chapterTitle = title
	}
}

func (r *sourceRuleRuntime) persistentBookVariables() string {
	return sourceRuleVariableJSON(r.bookVariables)
}

func (r *sourceRuleRuntime) persistentChapterVariables() string {
	return sourceRuleVariableJSON(r.chapterVariables)
}

func sourceRuleVariableJSON(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	encoded, err := models.NormalizeSourceRuleVariables(mustMarshalSourceRuleVariables(values))
	if err != nil {
		return ""
	}
	return encoded
}

func mustMarshalSourceRuleVariables(values map[string]string) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func (r *sourceRuleRuntime) variableTarget() map[string]string {
	if r.chapterVariables != nil {
		return r.chapterVariables
	}
	if r.bookVariables != nil {
		return r.bookVariables
	}
	if r.variables == nil {
		r.variables = make(map[string]string)
	}
	return r.variables
}

func (r *sourceRuleRuntime) variableValue(key string) string {
	if r == nil {
		return ""
	}
	if key == "bookName" && r.bookVariables != nil {
		return r.bookName
	}
	if key == "title" && r.chapterVariables != nil {
		return r.chapterTitle
	}
	if r.chapterVariables != nil {
		if value, ok := r.chapterVariables[key]; ok {
			return value
		}
	}
	if r.bookVariables != nil {
		if value, ok := r.bookVariables[key]; ok {
			return value
		}
	}
	return r.variables[key]
}

func (r *sourceRuleRuntime) enterVariableRule() error {
	if r == nil {
		return fmt.Errorf("%w: missing variable runtime", ErrInvalidSourceRule)
	}
	if r.depth >= maxSourceRuleVariableDepth {
		return fmt.Errorf("%w: variable rule nesting exceeds %d", ErrInvalidSourceRule, maxSourceRuleVariableDepth)
	}
	r.depth++
	return nil
}

func (r *sourceRuleRuntime) leaveVariableRule() {
	if r != nil && r.depth > 0 {
		r.depth--
	}
}

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
	runtime   *sourceRuleRuntime
}

// sourceRuleTransform is the trailing ##regex##replacement[##first] stage of
// reader-dev's SourceRule. It intentionally runs after selector evaluation so
// every scalar value is transformed independently.
type sourceRuleTransform struct {
	rule         string
	pattern      string
	replacement  string
	replaceFirst bool
	compiled     *regexp.Regexp
	hasTransform bool
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
	return d.RootWithRuntime(nil)
}

func (d *sourceRuleDocument) RootWithRuntime(runtime *sourceRuleRuntime) sourceRuleValue {
	if runtime == nil {
		runtime = newSourceRuleRuntime()
	}
	return sourceRuleValue{document: d, selection: d.document.Selection, xpathNode: d.xpathRoot, runtime: runtime}
}

func (v sourceRuleValue) withRuntime(runtime *sourceRuleRuntime) sourceRuleValue {
	v.runtime = runtime
	return v
}

func sourceRuleRuntimeFor(value *sourceRuleValue) *sourceRuleRuntime {
	if value.runtime == nil {
		value.runtime = newSourceRuleRuntime()
	}
	return value.runtime
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
	prepared, err := prepareSourceRule(&value, rule)
	if err != nil {
		return nil, err
	}
	rule = prepared.rule
	if prepared.literal {
		if rule == "" {
			return nil, nil
		}
		return []sourceRuleValue{{document: value.document, text: rule, runtime: value.runtime}}, nil
	}
	if rule == "" {
		if prepared.transform.hasTransform {
			return []sourceRuleValue{value}, nil
		}
		return nil, nil
	}
	if parts, operator := sourceRuleCompositeParts(rule); len(parts) > 1 {
		lists := make([][]sourceRuleValue, 0, len(parts))
		for _, part := range parts {
			items, err := sourceRuleElements(value, part)
			if err != nil {
				return nil, err
			}
			if operator == "||" && len(items) > 0 {
				return items, nil
			}
			if len(items) > 0 {
				lists = append(lists, items)
			}
		}
		return combineSourceRuleValueLists(lists, operator), nil
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
	prepared, err := prepareSourceRule(&value, rule)
	if err != nil {
		return nil, err
	}
	transform := prepared.transform
	rule = prepared.rule
	if prepared.literal {
		parsed, err := transform.apply(rule)
		if err != nil {
			return nil, err
		}
		return []string{parsed}, nil
	}
	if rule == "" {
		if !transform.hasTransform {
			return nil, nil
		}
		parsed, err := sourceRuleValueString(value, "")
		if err != nil || parsed == "" {
			return nil, err
		}
		parsed, err = transform.apply(parsed)
		if err != nil {
			return nil, err
		}
		return []string{parsed}, nil
	}
	if capture, ok := sourceRuleCapture(rule); ok {
		if capture >= len(value.captures) {
			return nil, nil
		}
		parsed, err := transform.apply(value.captures[capture])
		if err != nil || parsed == "" {
			return nil, err
		}
		return []string{parsed}, nil
	}
	if parts, operator := sourceRuleCompositeParts(rule); len(parts) > 1 {
		lists := make([][]string, 0, len(parts))
		for _, part := range parts {
			values, err := sourceRuleStrings(value, part)
			if err != nil {
				return nil, err
			}
			if operator == "||" && len(values) > 0 {
				return values, nil
			}
			if len(values) > 0 {
				lists = append(lists, values)
			}
		}
		values := combineSourceRuleStringLists(lists, operator)
		return sourceRuleApplyTransform(values, transform)
	}
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
			parsed, err = transform.apply(parsed)
			if err != nil {
				return nil, err
			}
			values = append(values, parsed)
		}
	}
	return values, nil
}

func combineSourceRuleValueLists(lists [][]sourceRuleValue, operator string) []sourceRuleValue {
	if len(lists) == 0 {
		return nil
	}
	if operator != "%%" {
		result := make([]sourceRuleValue, 0)
		for _, list := range lists {
			result = append(result, list...)
		}
		return result
	}
	result := make([]sourceRuleValue, 0)
	for index := range lists[0] {
		for _, list := range lists {
			if index < len(list) {
				result = append(result, list[index])
			}
		}
	}
	return result
}

func combineSourceRuleStringLists(lists [][]string, operator string) []string {
	if len(lists) == 0 {
		return nil
	}
	if operator != "%%" {
		result := make([]string, 0)
		for _, list := range lists {
			result = append(result, list...)
		}
		return result
	}
	result := make([]string, 0)
	for index := range lists[0] {
		for _, list := range lists {
			if index < len(list) {
				result = append(result, list[index])
			}
		}
	}
	return result
}

func sourceRuleString(value sourceRuleValue, rule string) (string, error) {
	values, err := sourceRuleStrings(value, rule)
	if err != nil || len(values) == 0 {
		return "", err
	}
	return values[0], nil
}

type preparedSourceRule struct {
	rule      string
	transform sourceRuleTransform
	literal   bool
}

func prepareSourceRule(value *sourceRuleValue, rawRule string) (preparedSourceRule, error) {
	runtime := sourceRuleRuntimeFor(value)
	rule, putRules, err := sourceRuleExtractPutRules(rawRule)
	if err != nil {
		return preparedSourceRule{}, err
	}
	if err := sourceRuleUnsupportedError(rule); err != nil {
		return preparedSourceRule{}, err
	}
	if err := sourceRuleApplyPuts(*value, runtime, putRules); err != nil {
		return preparedSourceRule{}, err
	}
	transform, err := parseSourceRuleTransform(rule)
	if err != nil {
		return preparedSourceRule{}, err
	}
	if !sourceRuleGetPattern.MatchString(transform.rule) {
		return preparedSourceRule{rule: transform.rule, transform: transform}, nil
	}
	literal, err := sourceRuleInterpolateGets(transform.rule, runtime)
	if err != nil {
		return preparedSourceRule{}, err
	}
	transform.rule = literal
	return preparedSourceRule{rule: literal, transform: transform, literal: true}, nil
}

func sourceRuleExtractPutRules(rawRule string) (string, []map[string]string, error) {
	lowerRule := strings.ToLower(rawRule)
	putRules := make([]map[string]string, 0)
	var result strings.Builder
	for offset := 0; offset < len(rawRule); {
		match := strings.Index(lowerRule[offset:], "@put:")
		if match < 0 {
			result.WriteString(rawRule[offset:])
			break
		}
		start := offset + match
		result.WriteString(rawRule[offset:start])
		objectStart := start + len("@put:")
		if objectStart >= len(rawRule) || rawRule[objectStart] != '{' {
			return "", nil, fmt.Errorf("%w: @put requires a JSON object", ErrInvalidSourceRule)
		}
		objectEnd, err := sourceRuleJSONObjectEnd(rawRule, objectStart)
		if err != nil {
			return "", nil, fmt.Errorf("%w: invalid @put JSON object: %v", ErrInvalidSourceRule, err)
		}
		object := rawRule[objectStart:objectEnd]
		if len(object) > maxSourceRuleVariableBytes {
			return "", nil, fmt.Errorf("%w: @put JSON exceeds %d bytes", ErrInvalidSourceRule, maxSourceRuleVariableBytes)
		}
		values := make(map[string]string)
		if err := json.Unmarshal([]byte(object), &values); err != nil {
			return "", nil, fmt.Errorf("%w: invalid @put JSON: %v", ErrInvalidSourceRule, err)
		}
		if err := validateSourceRuleVariables(values); err != nil {
			return "", nil, err
		}
		putRules = append(putRules, values)
		offset = objectEnd
	}
	return result.String(), putRules, nil
}

func sourceRuleJSONObjectEnd(value string, start int) (int, error) {
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index + 1, nil
			}
			if depth < 0 {
				return 0, errors.New("unexpected closing brace")
			}
		}
	}
	return 0, errors.New("unterminated object")
}

func validateSourceRuleVariables(values map[string]string) error {
	if len(values) == 0 || len(values) > maxSourceRuleVariables {
		return fmt.Errorf("%w: @put requires 1 to %d string variables", ErrInvalidSourceRule, maxSourceRuleVariables)
	}
	total := 0
	for key, value := range values {
		if key != strings.TrimSpace(key) || key == "" || len(key) > maxSourceRuleVariableKeySize {
			return fmt.Errorf("%w: variable key must be 1 to %d bytes", ErrInvalidSourceRule, maxSourceRuleVariableKeySize)
		}
		if len(value) > maxSourceRuleVariableValue {
			return fmt.Errorf("%w: variable value rule exceeds %d bytes", ErrInvalidSourceRule, maxSourceRuleVariableValue)
		}
		total += len(key) + len(value)
	}
	if total > maxSourceRuleVariableBytes {
		return fmt.Errorf("%w: @put variables exceed %d bytes", ErrInvalidSourceRule, maxSourceRuleVariableBytes)
	}
	return nil
}

func sourceRuleApplyPuts(value sourceRuleValue, runtime *sourceRuleRuntime, putRules []map[string]string) error {
	if len(putRules) == 0 {
		return nil
	}
	working := runtime.clone()
	if err := working.enterVariableRule(); err != nil {
		return err
	}
	defer working.leaveVariableRule()
	for _, putRule := range putRules {
		keys := make([]string, 0, len(putRule))
		for key := range putRule {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parsed, err := sourceRuleString(value.withRuntime(working), putRule[key])
			if err != nil {
				return err
			}
			if err := sourceRuleSetVariable(working, key, parsed); err != nil {
				return err
			}
		}
	}
	runtime.variables = working.variables
	runtime.bookVariables = working.bookVariables
	runtime.chapterVariables = working.chapterVariables
	runtime.bookName = working.bookName
	runtime.chapterTitle = working.chapterTitle
	return nil
}

func sourceRuleSetVariable(runtime *sourceRuleRuntime, key, value string) error {
	if runtime == nil {
		return fmt.Errorf("%w: missing variable runtime", ErrInvalidSourceRule)
	}
	if key == "" || len(key) > maxSourceRuleVariableKeySize || len(value) > maxSourceRuleVariableValue {
		return fmt.Errorf("%w: variable key or value exceeds persistent bounds", ErrInvalidSourceRule)
	}
	target := runtime.variableTarget()
	if _, exists := target[key]; !exists && len(target) >= maxSourceRuleVariables {
		return fmt.Errorf("%w: variable runtime exceeds %d entries", ErrInvalidSourceRule, maxSourceRuleVariables)
	}
	total := 0
	for existingKey, existingValue := range target {
		total += len(existingKey)
		if existingKey == key {
			total += len(value)
			continue
		}
		total += len(existingValue)
	}
	if _, exists := target[key]; !exists {
		total += len(key) + len(value)
	}
	if total > maxSourceRuleVariableBytes {
		return fmt.Errorf("%w: variable runtime exceeds %d bytes", ErrInvalidSourceRule, maxSourceRuleVariableBytes)
	}
	target[key] = value
	return nil
}

func sourceRuleInterpolateGets(rule string, runtime *sourceRuleRuntime) (string, error) {
	matches := sourceRuleGetPattern.FindAllStringSubmatchIndex(rule, -1)
	if len(matches) == 0 {
		return rule, nil
	}
	var result strings.Builder
	result.Grow(len(rule))
	offset := 0
	for _, match := range matches {
		key := strings.TrimSpace(rule[match[2]:match[3]])
		if key == "" || len(key) > maxSourceRuleVariableKeySize {
			return "", fmt.Errorf("%w: invalid @get variable key", ErrInvalidSourceRule)
		}
		result.WriteString(rule[offset:match[0]])
		result.WriteString(runtime.variableValue(key))
		offset = match[1]
	}
	result.WriteString(rule[offset:])
	return result.String(), nil
}

func parseSourceRuleTransform(rule string) (sourceRuleTransform, error) {
	rawRule := strings.TrimSpace(rule)
	if err := sourceRuleUnsupportedError(rawRule); err != nil {
		return sourceRuleTransform{}, err
	}
	parts := strings.Split(rawRule, "##")
	transform := sourceRuleTransform{rule: strings.TrimSpace(parts[0])}
	if len(parts) < 2 {
		return transform, nil
	}
	transform.hasTransform = true
	transform.pattern = parts[1]
	if len(parts) > 2 {
		transform.replacement = parts[2]
	}
	transform.replaceFirst = len(parts) > 3
	if transform.pattern == "" {
		return transform, nil
	}
	compiled, err := regexp.Compile(transform.pattern)
	if err != nil {
		return sourceRuleTransform{}, fmt.Errorf("%w: replacement regex %q: %v", ErrInvalidSourceRule, transform.pattern, err)
	}
	transform.compiled = compiled
	return transform, nil
}

func (t sourceRuleTransform) apply(value string) (string, error) {
	if !t.hasTransform || t.pattern == "" {
		return value, nil
	}
	if t.compiled == nil {
		return "", fmt.Errorf("%w: replacement regex was not compiled", ErrInvalidSourceRule)
	}
	if !t.replaceFirst {
		return t.compiled.ReplaceAllString(value, t.replacement), nil
	}
	match := t.compiled.FindStringIndex(value)
	if match == nil {
		// reader-dev returns an empty string here. Keeping the original value is
		// the documented safety/usability adaptation: a harmless no-match must
		// not make a book title, chapter title or URL disappear.
		return value, nil
	}
	return value[:match[0]] + t.compiled.ReplaceAllString(value[match[0]:match[1]], t.replacement) + value[match[1]:], nil
}

func sourceRuleApplyTransform(values []string, transform sourceRuleTransform) ([]string, error) {
	if !transform.hasTransform || len(values) == 0 {
		return values, nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		parsed, err := transform.apply(value)
		if err != nil {
			return nil, err
		}
		if parsed != "" {
			result = append(result, parsed)
		}
	}
	return result, nil
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
		items = append(items, sourceRuleValue{document: value.document, selection: item, xpathNode: node, runtime: value.runtime})
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
		items = append(items, sourceRuleValue{document: value.document, jsonData: item, runtime: value.runtime})
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
		items = append(items, sourceRuleValue{document: value.document, xpathNode: item, runtime: value.runtime})
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
		items = append(items, sourceRuleValue{document: value.document, text: text, captures: match, runtime: value.runtime})
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

func sourceRuleUnsupportedError(rule string) error {
	if sourceRuleUsesJavaScript(rule) {
		return fmt.Errorf("%w: JavaScript/WebJS execution is disabled", ErrUnsupportedSourceRule)
	}
	if sourceRuleUsesTemplate(rule) {
		return fmt.Errorf("%w: template JavaScript requires an isolated runtime", ErrUnsupportedSourceRule)
	}
	return nil
}

func sourceRuleUsesVariables(rule string) bool {
	lower := strings.ToLower(rule)
	return strings.Contains(lower, "@put:") ||
		strings.Contains(lower, "@get:") ||
		sourceRuleUsesTemplate(rule)
}

func sourceRuleUsesTemplate(rule string) bool {
	return strings.Contains(rule, "{{") && strings.Contains(rule, "}}")
}

// IsSourceRuleError reports a local, author-fixable rule failure. It is
// intentionally distinct from ErrSourceRequest so APIs can avoid caching it
// as an unhealthy remote source.
func IsSourceRuleError(err error) bool {
	return errors.Is(err, ErrUnsupportedSourceRule) || errors.Is(err, ErrInvalidSourceRule)
}

// sourceRuleCompositeParts mirrors reader-dev's RuleAnalyzer for the three
// collection operators. Only top-level operators split a rule: XPath and
// JSONPath filters may contain the same tokens inside brackets, parentheses,
// braces or quoted strings.
func sourceRuleCompositeParts(rule string) ([]string, string) {
	trimmed := strings.TrimSpace(rule)
	if trimmed == "" {
		return nil, ""
	}
	prefix := ""
	lower := strings.ToLower(trimmed)
	for _, candidate := range []string{"@css:", "@xpath:", "@json:"} {
		if strings.HasPrefix(lower, candidate) {
			prefix = trimmed[:len(candidate)]
			trimmed = strings.TrimSpace(trimmed[len(candidate):])
			break
		}
	}

	operator := ""
	parts := make([]string, 0)
	start := 0
	brackets, parentheses, braces := 0, 0, 0
	var quote byte
	escaped := false
	for index := 0; index < len(trimmed); index++ {
		current := trimmed[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '(':
			parentheses++
		case ')':
			if parentheses > 0 {
				parentheses--
			}
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		}
		if brackets != 0 || parentheses != 0 || braces != 0 || index+1 >= len(trimmed) {
			continue
		}
		candidate := trimmed[index : index+2]
		if candidate != "&&" && candidate != "||" && candidate != "%%" {
			continue
		}
		if operator == "" {
			operator = candidate
			parts = append(parts, strings.TrimSpace(trimmed[start:index]))
			start = index + 2
			index++
			continue
		}
		if candidate == operator {
			parts = append(parts, strings.TrimSpace(trimmed[start:index]))
			start = index + 2
			index++
		}
	}
	if operator == "" {
		return nil, ""
	}
	parts = append(parts, strings.TrimSpace(trimmed[start:]))
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if prefix != "" && !sourceRuleHasExplicitMode(part) {
			part = prefix + part
		}
		filtered = append(filtered, part)
	}
	if len(filtered) < 2 {
		return nil, ""
	}
	return filtered, operator
}

func sourceRuleHasExplicitMode(rule string) bool {
	lower := strings.ToLower(strings.TrimSpace(rule))
	return strings.HasPrefix(lower, "@css:") ||
		strings.HasPrefix(lower, "@xpath:") ||
		strings.HasPrefix(lower, "@json:") ||
		strings.HasPrefix(lower, "@js:") ||
		strings.HasPrefix(rule, "$.") ||
		strings.HasPrefix(rule, "$[") ||
		strings.HasPrefix(rule, "//")
}

func sourceRuleNeedsEvaluator(rule string) bool {
	trimmed := strings.TrimSpace(rule)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "##") || sourceRuleUsesVariables(trimmed) {
		return true
	}
	if parts, _ := sourceRuleCompositeParts(trimmed); len(parts) > 1 {
		return true
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
