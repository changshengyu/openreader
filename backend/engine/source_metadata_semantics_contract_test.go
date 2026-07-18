package engine

import (
	"testing"

	"openreader/backend/models"
)

func TestSourceMetadataNormalizationMatchesReaderDevAcrossRuleModes(t *testing.T) {
	htmlBody := `<article class="book">
		<h2 class="name">示例书 作者：网页作者</h2>
		<span class="author">作者：李四 著</span>
		<div class="intro" data-value="第一段&lt;br&gt;第二段"></div>
		<a class="url" href="/book/1">详情</a>
	</article>`
	jsonBody := `{"books":[{"name":"示例书 作者：网页作者","author":"作者：李四 著","intro":"第一段<br>第二段","url":"/book/1"}]}`

	tests := []struct {
		name  string
		body  string
		rules models.BookSourceRule
	}{
		{
			name: "CSS",
			body: htmlBody,
			rules: models.BookSourceRule{
				BookListRule:   ".book",
				BookNameRule:   ".name",
				BookAuthorRule: ".author",
				BookIntroRule:  ".intro|attr:data-value",
				BookURLRule:    ".url|attr:href",
			},
		},
		{
			name: "JSONPath",
			body: jsonBody,
			rules: models.BookSourceRule{
				BookListRule:   "$.books[*]",
				BookNameRule:   "$.name",
				BookAuthorRule: "$.author",
				BookIntroRule:  "$.intro",
				BookURLRule:    "$.url",
			},
		},
		{
			name: "XPath",
			body: htmlBody,
			rules: models.BookSourceRule{
				BookListRule:   "@XPath://article[@class='book']",
				BookNameRule:   "@XPath:.//h2[@class='name']/text()",
				BookAuthorRule: "@XPath:.//span[@class='author']/text()",
				BookIntroRule:  "@XPath:.//div[@class='intro']/@data-value",
				BookURLRule:    "@XPath:.//a[@class='url']/@href",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := newSourceRuleDocument(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			items, err := parseBookResultsFromSourceDocument(
				document,
				tt.rules,
				models.BookSource{ID: 1, Name: tt.name},
				sourceRequest{URL: "https://source.example/search"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("items = %#v", items)
			}
			item := items[0]
			if item.Title != "示例书" || item.Author != "李四" || item.Intro != "第一段\n　　第二段" {
				t.Fatalf("normalized %s metadata = %#v", tt.name, item)
			}
			if item.BookURL != "https://source.example/book/1" {
				t.Fatalf("book URL = %q", item.BookURL)
			}
		})
	}
}

func TestSourceDetailNormalizationAndCanRenameAreConfigurationSemantics(t *testing.T) {
	htmlBody := `<main>
		<h1 class="name">详情书 张三 著</h1>
		<span class="author">作 者 ： 王五 著</span>
		<div class="intro" data-value="首段&lt;br/&gt;次段"></div>
		<span class="rename"></span>
	</main>`
	jsonBody := `{"book":{"name":"详情书 张三 著","author":"作 者 ： 王五 著","intro":"首段<br/>次段","rename":false}}`

	tests := []struct {
		name  string
		body  string
		rules models.BookSourceRule
	}{
		{
			name: "CSS empty selector value",
			body: htmlBody,
			rules: models.BookSourceRule{
				BookInfoNameRule:      ".name",
				BookInfoAuthorRule:    ".author",
				BookInfoIntroRule:     ".intro|attr:data-value",
				BookInfoCanRenameRule: ".rename",
			},
		},
		{
			name: "JSONPath false value",
			body: jsonBody,
			rules: models.BookSourceRule{
				BookInfoInitRule:      "$.book",
				BookInfoNameRule:      "$.name",
				BookInfoAuthorRule:    "$.author",
				BookInfoIntroRule:     "$.intro",
				BookInfoCanRenameRule: "$.rename",
			},
		},
		{
			name: "XPath missing value",
			body: htmlBody,
			rules: models.BookSourceRule{
				BookInfoInitRule:      "@XPath://main",
				BookInfoNameRule:      "@XPath:.//h1[@class='name']/text()",
				BookInfoAuthorRule:    "@XPath:.//span[@class='author']/text()",
				BookInfoIntroRule:     "@XPath:.//div[@class='intro']/@data-value",
				BookInfoCanRenameRule: "@XPath:.//missing/text()",
			},
		},
		{
			name: "non-executed script marker",
			body: jsonBody,
			rules: models.BookSourceRule{
				BookInfoInitRule:      "$.book",
				BookInfoNameRule:      "$.name",
				BookInfoAuthorRule:    "$.author",
				BookInfoIntroRule:     "$.intro",
				BookInfoCanRenameRule: "@js:must-not-run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := newSourceRuleDocument(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			info, err := parseRemoteBookInfoFromSourceDocument(document, tt.rules, "https://source.example/book")
			if err != nil {
				t.Fatal(err)
			}
			if info.Title != "详情书" || info.Author != "王五" || info.Intro != "首段\n　　次段" {
				t.Fatalf("normalized %s detail = %#v", tt.name, info)
			}
			if !info.CanRename {
				t.Fatalf("configured canReName must be true without evaluating its value: %#v", info)
			}
		})
	}

	document, err := newSourceRuleDocument(jsonBody)
	if err != nil {
		t.Fatal(err)
	}
	info, err := parseRemoteBookInfoFromSourceDocument(document, models.BookSourceRule{
		BookInfoInitRule:   "$.book",
		BookInfoNameRule:   "$.name",
		BookInfoAuthorRule: "$.author",
	}, "https://source.example/book")
	if err != nil {
		t.Fatal(err)
	}
	if info.CanRename {
		t.Fatalf("missing canReName configuration must remain false: %#v", info)
	}
}

func TestDirectDetailSearchUsesTheSameMetadataNormalization(t *testing.T) {
	document, err := newSourceRuleDocument(`{"book":{"name":"回退书 作者：某人","author":"作者：赵六 著","intro":"一<br>二"}}`)
	if err != nil {
		t.Fatal(err)
	}
	rules := models.BookSourceRule{
		BookListRule:       "$.missing[*]",
		BookInfoInitRule:   "$.book",
		BookInfoNameRule:   "$.name",
		BookInfoAuthorRule: "$.author",
		BookInfoIntroRule:  "$.intro",
	}
	items, err := parseBookResultsFromSourceDocument(
		document,
		rules,
		models.BookSource{ID: 1, Name: "详情回退源"},
		sourceRequest{URL: "https://source.example/detail", Descriptor: "https://source.example/detail"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "回退书" || items[0].Author != "赵六" || items[0].Intro != "一\n　　二" {
		t.Fatalf("direct detail metadata = %#v", items)
	}
}
