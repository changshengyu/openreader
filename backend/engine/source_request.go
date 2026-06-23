package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/htmlindex"
)

type sourceRequest struct {
	URL            string
	Method         string
	Body           string
	Charset        string
	Headers        map[string]string
	Retry          int
	Type           string
	Proxy          string
	SourceKey      string
	ConcurrentRate string
	Descriptor     string
}

type sourceURLOption struct {
	Method  string `json:"method"`
	Charset string `json:"charset"`
	Headers any    `json:"headers"`
	Body    any    `json:"body"`
	Retry   any    `json:"retry"`
	Type    string `json:"type"`
}

var sourcePageChoicePattern = regexp.MustCompile(`<([^<>]*)>`)

type SourceRequest = sourceRequest

type SourceRequestPolicy struct {
	SourceKey      string
	ConcurrentRate string
}

func PrepareSourceRequest(rawURL, keyword string, page int, defaultCharset string, sourceHeaders map[string]string, policies ...SourceRequestPolicy) (SourceRequest, error) {
	return prepareSourceRequest(rawURL, keyword, page, defaultCharset, sourceHeaders, policies...)
}

func ResolveSourceURLTemplate(baseURL, value string) string {
	return resolveSourceURLTemplate(baseURL, value)
}

func SourceRequestKey(request SourceRequest) string {
	return sourceRequestKey(request)
}

func prepareSourceRequest(rawURL, keyword string, page int, defaultCharset string, sourceHeaders map[string]string, policies ...SourceRequestPolicy) (sourceRequest, error) {
	urlTemplate, optionText := splitSourceURLOption(rawURL)
	option := sourceURLOption{}
	if optionText != "" {
		decoder := json.NewDecoder(strings.NewReader(optionText))
		decoder.UseNumber()
		if err := decoder.Decode(&option); err != nil {
			return sourceRequest{}, fmt.Errorf("parse URL options: %w", err)
		}
	}

	requestCharset := strings.TrimSpace(defaultCharset)
	if option.Charset != "" {
		requestCharset = strings.TrimSpace(option.Charset)
	}
	request := sourceRequest{
		URL:     replaceSourceURLPlaceholders(urlTemplate, keyword, page, requestCharset),
		Method:  http.MethodGet,
		Charset: requestCharset,
		Headers: cloneHeaders(sourceHeaders),
	}
	request.Proxy = takeHeader(request.Headers, "proxy")
	if len(policies) > 0 {
		request.SourceKey = strings.TrimSpace(policies[0].SourceKey)
		request.ConcurrentRate = strings.TrimSpace(policies[0].ConcurrentRate)
	}
	if strings.EqualFold(strings.TrimSpace(option.Method), http.MethodPost) {
		request.Method = http.MethodPost
	}
	request.Retry = decodeSourceRetry(option.Retry)
	request.Type = strings.TrimSpace(option.Type)
	optionHeaders, err := decodeSourceOptionHeaders(option.Headers)
	if err != nil {
		return sourceRequest{}, fmt.Errorf("parse request headers: %w", err)
	}
	descriptorHeaders := make(map[string]string, len(optionHeaders))
	for name, value := range optionHeaders {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		rendered := replaceSourceBodyPlaceholders(fmt.Sprint(value), keyword, page)
		request.Headers[name] = rendered
		descriptorHeaders[name] = rendered
	}
	if option.Body != nil {
		body, err := marshalSourceRequestBody(option.Body)
		if err != nil {
			return sourceRequest{}, fmt.Errorf("encode request body: %w", err)
		}
		request.Body = replaceSourceBodyPlaceholders(body, keyword, page)
	}
	if request.Method == http.MethodPost {
		prepareSourcePOSTBody(&request)
	}
	request.Descriptor = buildSourceRequestDescriptor(request, optionText != "", descriptorHeaders)
	return request, nil
}

func splitSourceURLOption(value string) (string, string) {
	for index := 0; index < len(value); index++ {
		if value[index] != ',' {
			continue
		}
		remainder := strings.TrimSpace(value[index+1:])
		if strings.HasPrefix(remainder, "{") {
			return strings.TrimSpace(value[:index]), remainder
		}
	}
	return strings.TrimSpace(value), ""
}

func resolveSourceURLTemplate(baseURL, value string) string {
	urlPart, optionPart := splitSourceURLOption(value)
	baseURL, _ = splitSourceURLOption(baseURL)
	resolved := resolveURL(baseURL, urlPart)
	if optionPart == "" {
		return resolved
	}
	return resolved + ", " + optionPart
}

func prepareResolvedSourceRequest(baseURL, rawURL, keyword string, page int, defaultCharset string, sourceHeaders map[string]string, policies ...SourceRequestPolicy) (sourceRequest, error) {
	return prepareSourceRequest(
		resolveSourceURLTemplate(baseURL, rawURL),
		keyword,
		page,
		defaultCharset,
		sourceHeaders,
		policies...,
	)
}

func sourceRequestKey(request sourceRequest) string {
	headerNames := make([]string, 0, len(request.Headers))
	normalizedHeaders := make(map[string]string, len(request.Headers))
	for name, value := range request.Headers {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" {
			continue
		}
		if _, exists := normalizedHeaders[normalizedName]; !exists {
			headerNames = append(headerNames, normalizedName)
		}
		normalizedHeaders[normalizedName] = value
	}
	sort.Strings(headerNames)

	var key strings.Builder
	key.WriteString(strings.ToUpper(strings.TrimSpace(request.Method)))
	key.WriteByte('\n')
	key.WriteString(request.URL)
	key.WriteByte('\n')
	key.WriteString(request.Body)
	key.WriteByte('\n')
	key.WriteString(strings.ToLower(strings.TrimSpace(request.Charset)))
	key.WriteByte('\n')
	key.WriteString(strconv.Itoa(request.Retry))
	key.WriteByte('\n')
	key.WriteString(strings.ToLower(strings.TrimSpace(request.Type)))
	key.WriteByte('\n')
	key.WriteString(strings.TrimSpace(request.Proxy))
	for _, name := range headerNames {
		key.WriteByte('\n')
		key.WriteString(name)
		key.WriteByte(':')
		key.WriteString(normalizedHeaders[name])
	}
	return key.String()
}

func replaceSourceURLPlaceholders(value, keyword string, page int, charset string) string {
	value = strings.ReplaceAll(value, "{keyword}", keyword)
	value = strings.ReplaceAll(value, "{page}", strconv.Itoa(normalizeSourcePage(page)))
	value = replaceSourcePageChoices(value, page)
	return normalizeSourceURLFields(value, charset)
}

func replaceSourceBodyPlaceholders(value, keyword string, page int) string {
	value = strings.ReplaceAll(value, "{keyword}", keyword)
	value = strings.ReplaceAll(value, "{page}", strconv.Itoa(normalizeSourcePage(page)))
	return replaceSourcePageChoices(value, page)
}

func replaceSourcePageChoices(value string, page int) string {
	page = normalizeSourcePage(page)
	return sourcePageChoicePattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := sourcePageChoicePattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		choices := strings.Split(groups[1], ",")
		if len(choices) == 0 {
			return ""
		}
		index := page - 1
		if index >= len(choices) {
			index = len(choices) - 1
		}
		return strings.TrimSpace(choices[index])
	})
}

func marshalSourceRequestBody(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSourceOptionHeaders(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	if headers, ok := value.(map[string]any); ok {
		return headers, nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var headers map[string]any
	if err := json.Unmarshal([]byte(text), &headers); err != nil {
		return nil, err
	}
	return headers, nil
}

func decodeSourceRetry(value any) int {
	var retry int
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err == nil {
			retry = parsed
		}
	case float64:
		retry = int(typed)
	case string:
		retry, _ = strconv.Atoi(strings.TrimSpace(typed))
	}
	if retry < 0 {
		return 0
	}
	return retry
}

func prepareSourcePOSTBody(request *sourceRequest) {
	contentType := headerValue(request.Headers, "Content-Type")
	trimmedBody := strings.TrimSpace(request.Body)
	if contentType == "" {
		switch {
		case json.Valid([]byte(trimmedBody)):
			setHeader(request.Headers, "Content-Type", "application/json; charset=utf-8")
		case strings.HasPrefix(trimmedBody, "<"):
			setHeader(request.Headers, "Content-Type", "application/xml; charset=utf-8")
		default:
			setHeader(request.Headers, "Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		}
		contentType = headerValue(request.Headers, "Content-Type")
	}
	if strings.Contains(strings.ToLower(contentType), "application/x-www-form-urlencoded") {
		request.Body = normalizeSourceFormFields(request.Body, request.Charset)
	}
}

func normalizeSourceURLFields(rawURL, charset string) string {
	queryStart := strings.Index(rawURL, "?")
	if queryStart < 0 {
		return rawURL
	}
	queryEnd := len(rawURL)
	if fragmentStart := strings.Index(rawURL[queryStart:], "#"); fragmentStart >= 0 {
		queryEnd = queryStart + fragmentStart
	}
	return rawURL[:queryStart+1] +
		normalizeSourceFormFields(rawURL[queryStart+1:queryEnd], charset) +
		rawURL[queryEnd:]
}

func normalizeSourceFormFields(fields, charset string) string {
	parts := strings.Split(fields, "&")
	for index, part := range parts {
		name, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		parts[index] = name + "=" + encodeSourceFieldValue(value, charset)
	}
	return strings.Join(parts, "&")
}

func encodeSourceFieldValue(value, charset string) string {
	if sourceFieldAlreadyEncoded(value) {
		return value
	}
	if strings.EqualFold(strings.TrimSpace(charset), "escape") {
		return escapeSourceFieldValue(value)
	}
	data := []byte(value)
	normalizedCharset := strings.TrimSpace(charset)
	if normalizedCharset != "" && !strings.EqualFold(normalizedCharset, "utf-8") && !strings.EqualFold(normalizedCharset, "utf8") {
		if encoding, err := htmlindex.Get(normalizedCharset); err == nil {
			if encoded, encodeErr := encoding.NewEncoder().Bytes(data); encodeErr == nil {
				data = encoded
			}
		}
	}
	const hexDigits = "0123456789ABCDEF"
	var encoded strings.Builder
	for _, current := range data {
		switch {
		case current >= 'a' && current <= 'z',
			current >= 'A' && current <= 'Z',
			current >= '0' && current <= '9',
			current == '-', current == '_', current == '.', current == '*':
			encoded.WriteByte(current)
		case current == ' ':
			encoded.WriteByte('+')
		default:
			encoded.WriteByte('%')
			encoded.WriteByte(hexDigits[current>>4])
			encoded.WriteByte(hexDigits[current&0x0f])
		}
	}
	return encoded.String()
}

func sourceFieldAlreadyEncoded(value string) bool {
	for index := 0; index < len(value); index++ {
		current := value[index]
		switch {
		case current >= 'a' && current <= 'z',
			current >= 'A' && current <= 'Z',
			current >= '0' && current <= '9',
			strings.ContainsRune("-_.~!*'();:@&=+$,/?#[]+", rune(current)):
			continue
		case current == '%' && index+2 < len(value) && isSourceHex(value[index+1]) && isSourceHex(value[index+2]):
			index += 2
			continue
		default:
			return false
		}
	}
	return true
}

func isSourceHex(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}

func escapeSourceFieldValue(value string) string {
	var escaped strings.Builder
	for _, codeUnit := range utf16.Encode([]rune(value)) {
		switch {
		case codeUnit >= '0' && codeUnit <= '9',
			codeUnit >= 'A' && codeUnit <= 'Z',
			codeUnit >= 'a' && codeUnit <= 'z':
			escaped.WriteRune(rune(codeUnit))
		case codeUnit < 0x10:
			fmt.Fprintf(&escaped, "%%0%x", codeUnit)
		case codeUnit < 0x100:
			fmt.Fprintf(&escaped, "%%%x", codeUnit)
		default:
			fmt.Fprintf(&escaped, "%%u%x", codeUnit)
		}
	}
	return escaped.String()
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers)+1)
	for name, value := range headers {
		cloned[name] = value
	}
	return cloned
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func setHeader(headers map[string]string, name, value string) {
	for key := range headers {
		if strings.EqualFold(key, name) {
			delete(headers, key)
		}
	}
	headers[name] = value
}

func takeHeader(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			delete(headers, key)
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildSourceRequestDescriptor(request sourceRequest, hasOptions bool, optionHeaders map[string]string) string {
	if !hasOptions {
		return request.URL
	}
	options := make(map[string]any)
	if request.Method != http.MethodGet {
		options["method"] = request.Method
	}
	if request.Body != "" {
		options["body"] = request.Body
	}
	if len(optionHeaders) > 0 {
		options["headers"] = optionHeaders
	}
	if request.Charset != "" {
		options["charset"] = request.Charset
	}
	if request.Retry > 0 {
		options["retry"] = request.Retry
	}
	if request.Type != "" {
		options["type"] = request.Type
	}
	if len(options) == 0 {
		return request.URL
	}
	data, err := json.Marshal(options)
	if err != nil {
		return request.URL
	}
	return request.URL + ", " + string(data)
}

func normalizeSourcePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}
