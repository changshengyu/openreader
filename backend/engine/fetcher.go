package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var defaultClient = &http.Client{Timeout: 12 * time.Second}

func SetHTTPClient(client *http.Client) func() {
	previous := defaultClient
	if client == nil {
		defaultClient = &http.Client{Timeout: 12 * time.Second}
	} else {
		defaultClient = client
	}
	return func() {
		defaultClient = previous
	}
}

func FetchDocument(url, charset string) (*goquery.Document, error) {
	return FetchDocumentContext(context.Background(), url, charset)
}

func FetchDocumentContext(ctx context.Context, url, charset string) (*goquery.Document, error) {
	return FetchDocumentWithHeadersContext(ctx, url, charset, nil)
}

func FetchDocumentWithHeaders(url, charset string, headers map[string]string) (*goquery.Document, error) {
	return FetchDocumentWithHeadersContext(context.Background(), url, charset, headers)
}

func FetchDocumentWithHeadersContext(ctx context.Context, url, charset string, headers map[string]string) (*goquery.Document, error) {
	decoded, err := FetchTextRequestContext(ctx, http.MethodGet, url, "", charset, headers)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(decoded))
}

func FetchDocumentRequestContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (*goquery.Document, error) {
	document, _, err := FetchDocumentRequestWithURLContext(ctx, method, url, body, charset, headers)
	return document, err
}

func FetchDocumentRequestWithURLContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (*goquery.Document, string, error) {
	decoded, responseURL, err := FetchTextRequestWithURLContext(ctx, method, url, body, charset, headers)
	if err != nil {
		return nil, responseURL, err
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(decoded))
	return document, responseURL, err
}

func FetchSourceDocumentWithURLContext(ctx context.Context, request SourceRequest) (*goquery.Document, string, error) {
	decoded, responseURL, err := FetchSourceTextWithURLContext(ctx, request)
	if err != nil {
		return nil, responseURL, err
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(decoded))
	return document, responseURL, err
}

func FetchText(url, charset string) (string, error) {
	return FetchTextContext(context.Background(), url, charset)
}

func FetchTextContext(ctx context.Context, url, charset string) (string, error) {
	return FetchTextWithHeadersContext(ctx, url, charset, nil)
}

func FetchTextWithHeaders(url, charset string, headers map[string]string) (string, error) {
	return FetchTextWithHeadersContext(context.Background(), url, charset, headers)
}

func FetchTextWithHeadersContext(ctx context.Context, url, charset string, headers map[string]string) (string, error) {
	return FetchTextRequestContext(ctx, http.MethodGet, url, "", charset, headers)
}

func FetchTextRequestContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (string, error) {
	decoded, _, err := FetchTextRequestWithURLContext(ctx, method, url, body, charset, headers)
	return decoded, err
}

func FetchTextRequestWithURLContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (string, string, error) {
	return fetchTextRequestWithURLContext(ctx, method, url, body, charset, headers, 0, "")
}

func FetchSourceTextWithURLContext(ctx context.Context, request SourceRequest) (string, string, error) {
	return fetchTextRequestWithURLContext(
		ctx,
		request.Method,
		request.URL,
		request.Body,
		request.Charset,
		request.Headers,
		request.Retry,
		request.Type,
	)
}

func fetchTextRequestWithURLContext(
	ctx context.Context,
	method string,
	url string,
	body string,
	charset string,
	headers map[string]string,
	retry int,
	responseType string,
) (string, string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if retry < 0 {
		retry = 0
	}

	for attempt := 0; attempt <= retry; attempt++ {
		var requestBody io.Reader
		if body != "" {
			requestBody = strings.NewReader(body)
		}
		request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
		if err != nil {
			return "", url, err
		}
		for name, value := range headers {
			name = strings.TrimSpace(name)
			if name == "" || strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") {
				continue
			}
			request.Header.Set(name, value)
		}
		if request.Header.Get("User-Agent") == "" {
			request.Header.Set("User-Agent", "OpenReader/0.1 (+self-hosted reader)")
		}

		response, err := defaultClient.Do(request)
		if err != nil {
			return "", url, err
		}

		responseURL := url
		if response.Request != nil && response.Request.URL != nil {
			responseURL = response.Request.URL.String()
		}
		responseBody, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return "", responseURL, readErr
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			if attempt < retry {
				continue
			}
		}

		if strings.TrimSpace(responseType) != "" {
			return hex.EncodeToString(responseBody), responseURL, nil
		}
		decoded, err := DecodeBody(responseBody, charset)
		if err != nil {
			return "", responseURL, err
		}
		return decoded, responseURL, nil
	}
	return "", url, nil
}

func DecodeBody(body []byte, charset string) (string, error) {
	if utf8.Valid(body) && !isGBK(charset) {
		return string(body), nil
	}

	if isGBK(charset) {
		reader := transform.NewReader(bytes.NewReader(body), simplifiedchinese.GBK.NewDecoder())
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return string(body), nil
}

func isGBK(charset string) bool {
	normalized := strings.ToLower(strings.TrimSpace(charset))
	return normalized == "gbk" || normalized == "gb2312" || normalized == "gb18030"
}
