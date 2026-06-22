package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type sourceRequest struct {
	URL     string
	Method  string
	Body    string
	Charset string
	Headers map[string]string
}

type sourceURLOption struct {
	Method  string `json:"method"`
	Charset string `json:"charset"`
	Headers any    `json:"headers"`
	Body    any    `json:"body"`
	Type    string `json:"type"`
}

func prepareSourceRequest(rawURL, keyword string, page int, defaultCharset string, sourceHeaders map[string]string) (sourceRequest, error) {
	urlTemplate, optionText := splitSourceURLOption(rawURL)
	option := sourceURLOption{}
	if optionText != "" {
		decoder := json.NewDecoder(strings.NewReader(optionText))
		decoder.UseNumber()
		if err := decoder.Decode(&option); err != nil {
			return sourceRequest{}, fmt.Errorf("parse URL options: %w", err)
		}
	}

	request := sourceRequest{
		URL:     replaceSourceURLPlaceholders(urlTemplate, keyword, page),
		Method:  http.MethodGet,
		Charset: strings.TrimSpace(defaultCharset),
		Headers: cloneHeaders(sourceHeaders),
	}
	if strings.EqualFold(strings.TrimSpace(option.Method), http.MethodPost) {
		request.Method = http.MethodPost
	}
	if option.Charset != "" {
		request.Charset = strings.TrimSpace(option.Charset)
	}
	optionHeaders, err := decodeSourceOptionHeaders(option.Headers)
	if err != nil {
		return sourceRequest{}, fmt.Errorf("parse request headers: %w", err)
	}
	for name, value := range optionHeaders {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		request.Headers[name] = replaceSourceBodyPlaceholders(fmt.Sprint(value), keyword, page)
	}
	if option.Body != nil {
		body, err := marshalSourceRequestBody(option.Body)
		if err != nil {
			return sourceRequest{}, fmt.Errorf("encode request body: %w", err)
		}
		request.Body = replaceSourceBodyPlaceholders(body, keyword, page)
	}
	if request.Method == http.MethodPost {
		prepareSourcePOSTBody(&request, option.Type)
	}
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
	resolved := resolveURL(baseURL, urlPart)
	if optionPart == "" {
		return resolved
	}
	return resolved + ", " + optionPart
}

func replaceSourceURLPlaceholders(value, keyword string, page int) string {
	value = strings.ReplaceAll(value, "{keyword}", url.QueryEscape(keyword))
	return strings.ReplaceAll(value, "{page}", strconv.Itoa(normalizeSourcePage(page)))
}

func replaceSourceBodyPlaceholders(value, keyword string, page int) string {
	value = strings.ReplaceAll(value, "{keyword}", keyword)
	return strings.ReplaceAll(value, "{page}", strconv.Itoa(normalizeSourcePage(page)))
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

func prepareSourcePOSTBody(request *sourceRequest, optionType string) {
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
		request.Body = normalizeSourceFormBody(request.Body)
	}
	if strings.TrimSpace(optionType) != "" && request.Body == "" {
		setHeader(request.Headers, "Content-Type", optionType)
	}
}

func normalizeSourceFormBody(body string) string {
	values, err := url.ParseQuery(body)
	if err != nil {
		return body
	}
	return values.Encode()
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

func normalizeSourcePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}
