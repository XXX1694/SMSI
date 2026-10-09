package bluesky

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

const (
	facetLink = "app.bsky.richtext.facet#link"
	facetTag  = "app.bsky.richtext.facet#tag"
	maxTagLen = 64 // graphemes [V]
)

type facet struct {
	Index    byteSlice      `json:"index"`
	Features []facetFeature `json:"features"`
}

type byteSlice struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type facetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// graphemeCount is how Bluesky counts text length.
func graphemeCount(s string) int { return uniseg.GraphemeClusterCount(s) }

var (
	urlPattern = regexp.MustCompile(`(?:^|[\s(])(https?://[^\s]+)`)
	tagPattern = regexp.MustCompile(`(?:^|\s)[#＃]([^\s#＃]+)`)
)

// buildFacets finds URLs and hashtags. Go strings are UTF-8, so regexp indexes are
// already the byte offsets the protocol requires. Mentions are skipped: they need a
// handle-to-DID lookup per mention.
func buildFacets(text string) []facet {
	var out []facet
	for _, m := range urlPattern.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2], m[3]
		end = start + len(trimURLTail(text[start:end]))
		if end-start <= len("https://") {
			continue
		}
		out = append(out, facet{Index: byteSlice{start, end},
			Features: []facetFeature{{Type: facetLink, URI: text[start:end]}}})
	}
	for _, m := range tagPattern.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2]-hashWidth(text, m[2]), m[3]
		tag := trimTagTail(text[m[2]:end])
		end = m[2] + len(tag)
		if tag == "" || strings.IndexFunc(tag, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 ||
			graphemeCount(tag) > maxTagLen || overlaps(out, start, end) {
			continue
		}
		out = append(out, facet{Index: byteSlice{start, end},
			Features: []facetFeature{{Type: facetTag, Tag: tag}}})
	}
	return out
}

// hashWidth is the byte width of the '#' or fullwidth '＃' before pos.
func hashWidth(text string, pos int) int {
	_, w := utf8.DecodeLastRuneInString(text[:pos])
	return w
}

func overlaps(fs []facet, start, end int) bool {
	for _, f := range fs {
		if start < f.Index.ByteEnd && end > f.Index.ByteStart {
			return true
		}
	}
	return false
}

// trimURLTail drops sentence punctuation and unbalanced closing brackets.
func trimURLTail(u string) string {
	for u != "" {
		r, w := utf8.DecodeLastRuneInString(u)
		switch {
		case strings.ContainsRune(`.,;:!?'"`, r):
		case r == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
		case r == ']' || r == '}':
		default:
			return u
		}
		u = u[:len(u)-w]
	}
	return u
}

func trimTagTail(t string) string {
	return strings.TrimRightFunc(t, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
}
