package epubreader

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const epubBridgeScript = `(function () {
  "use strict";
  if (window.__openreaderEpubBridge) return;
  window.__openreaderEpubBridge = true;
  var parentOrigin = window.location.origin;
  var styleElement = document.createElement("style");
  styleElement.id = "openreader-epub-reader-style";
  (document.head || document.documentElement).appendChild(styleElement);

  function resourcePath() {
    var match = window.location.pathname.match(/^\/api\/epub-resource\/[^/]+\/(.*)$/);
    if (!match) return "";
    try { return decodeURIComponent(match[1]); } catch (_) { return match[1]; }
  }
  function notify(event, data) {
    if (window.self === window.top) return;
    window.top.postMessage(JSON.stringify({ event: event, data: data }), parentOrigin);
  }
  function notifyHeight() {
    notify("setHeight", Math.max(
      document.documentElement ? document.documentElement.scrollHeight : 0,
      document.body ? document.body.scrollHeight : 0
    ));
  }
  function loaded() {
    notifyHeight();
    notify("load", { path: resourcePath(), href: window.location.href });
  }
  function receive(event) {
    if (event.origin !== parentOrigin || event.source !== window.top) return;
    var message = event.data;
    try { message = typeof message === "string" ? JSON.parse(message) : message; } catch (_) { return; }
    if (!message || typeof message.event !== "string") return;
    if (message.event === "setStyle") {
      styleElement.textContent = String(message.style || "");
      notifyHeight();
      window.setTimeout(notifyHeight, 100);
    } else if (message.event === "requestHeight") {
      notifyHeight();
    }
  }
  function closestElement(target, selector) {
    return target && target.closest ? target.closest(selector) : null;
  }

  window.addEventListener("message", receive);
  window.addEventListener("resize", notifyHeight);
  window.addEventListener("keydown", function (event) {
    event.preventDefault();
    event.stopPropagation();
    notify("keydown", { key: event.key, keyCode: event.keyCode });
  });
  document.addEventListener("click", function (event) {
    var link = closestElement(event.target, "a");
    var image = closestElement(event.target, "img");
    if (link) {
      var targetURL;
      try { targetURL = new URL(link.getAttribute("href") || "", window.location.href); } catch (_) {
        event.preventDefault();
        return;
      }
      var rootMatch = window.location.pathname.match(/^(\/api\/epub-resource\/[^/]+\/)/);
      var insideResourceRoot = targetURL.origin === window.location.origin &&
        rootMatch && targetURL.pathname.indexOf(rootMatch[1]) === 0;
      if (!insideResourceRoot) {
        event.preventDefault();
        notify("externalLink", { href: targetURL.href });
        return;
      }
      if (targetURL.pathname === window.location.pathname && targetURL.hash) {
        var fragment = "";
        try { fragment = decodeURIComponent(targetURL.hash.slice(1)); } catch (_) { fragment = targetURL.hash.slice(1); }
        var target = document.getElementById(fragment);
        if (target) {
          event.preventDefault();
          notify("clickHash", target.getBoundingClientRect());
        } else {
          event.preventDefault();
          notify("navigate", { href: targetURL.href });
        }
      }
      return;
    }
    if (image) {
      event.preventDefault();
      var images = Array.prototype.slice.call(document.images || []);
      notify("previewImageList", {
        imageList: images.map(function (item) { return item.currentSrc || item.src; }),
        imageIndex: Math.max(0, images.indexOf(image))
      });
      return;
    }
    notify("click", {
      target: event.target && event.target.nodeName,
      clientX: event.clientX,
      clientY: event.clientY
    });
  });

  if (window.ResizeObserver) {
    new ResizeObserver(notifyHeight).observe(document.documentElement);
  } else if (window.MutationObserver) {
    new MutationObserver(notifyHeight).observe(document.documentElement, { childList: true, subtree: true });
  }
  notify("inited");
  if (document.readyState === "complete") {
    window.setTimeout(loaded, 0);
  } else {
    window.addEventListener("load", loaded, { once: true });
  }
})();`

func sanitizeAndInjectDocument(data []byte, startFragment, endFragment string) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid XHTML document", ErrInvalidArchive)
	}
	doc.Find("script, iframe, frame, frameset, object, embed, form, base").Remove()
	doc.Find(`meta[http-equiv]`).Each(func(_ int, selection *goquery.Selection) {
		if strings.EqualFold(strings.TrimSpace(selection.AttrOr("http-equiv", "")), "refresh") {
			selection.Remove()
		}
	})
	doc.Find("*").Each(func(_ int, selection *goquery.Selection) {
		node := selection.Get(0)
		if node == nil {
			return
		}
		attributes := node.Attr[:0]
		for _, attribute := range node.Attr {
			name := strings.ToLower(strings.TrimSpace(attribute.Key))
			if strings.HasPrefix(name, "on") || name == "srcdoc" {
				continue
			}
			attributes = append(attributes, attribute)
		}
		node.Attr = attributes
	})
	sliceDocumentToFragments(doc, startFragment, endFragment)

	head := doc.Find("head").First()
	if head.Length() == 0 {
		htmlNode := doc.Find("html").First()
		if htmlNode.Length() == 0 {
			return nil, fmt.Errorf("%w: EPUB document has no html root", ErrInvalidArchive)
		}
		htmlNode.PrependHtml("<head></head>")
		head = doc.Find("head").First()
	}
	head.AppendHtml(`<script id="openreader-epub-bridge">` + epubBridgeScript + `</script>`)
	rendered, err := doc.Html()
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

func sliceDocumentToFragments(doc *goquery.Document, startFragment, endFragment string) {
	if doc == nil || (startFragment == "" && endFragment == "") {
		return
	}
	body := doc.Find("body").First()
	if body.Length() == 0 {
		return
	}
	if start := findDocumentElementByID(body, startFragment); start.Length() > 0 {
		start.PrevAll().Remove()
	}
	if endFragment != "" && endFragment != startFragment {
		if end := findDocumentElementByID(body, endFragment); end.Length() > 0 {
			end.NextAll().Remove()
			end.Remove()
		}
	}
}

func findDocumentElementByID(root *goquery.Selection, id string) *goquery.Selection {
	if root == nil || id == "" {
		return &goquery.Selection{}
	}
	return root.Find("[id]").FilterFunction(func(_ int, selection *goquery.Selection) bool {
		return selection.AttrOr("id", "") == id
	}).First()
}

func documentCSP() string {
	sum := sha256.Sum256([]byte(epubBridgeScript))
	hash := base64.StdEncoding.EncodeToString(sum[:])
	return "default-src 'none'; " +
		"script-src 'sha256-" + hash + "'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"font-src 'self' data:; " +
		"media-src 'self'; " +
		"connect-src 'none'; frame-src 'none'; object-src 'none'; " +
		"base-uri 'none'; form-action 'none'; sandbox allow-scripts allow-same-origin"
}
