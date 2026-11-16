package server

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/microcosm-cc/bluemonday/css"
)

var paymentNotePolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("span", "p", "br", "strong", "b", "em", "i", "u", "s", "ul", "ol", "li", "a")
	p.AllowAttrs("style").Globally()
	p.AllowStyles("color").MatchingHandler(css.ColorHandler).Globally()
	p.AllowStyles("font-size").Matching(regexp.MustCompile(`(?i)^(?:(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:px|pt|em|rem|%)|xx-small|x-small|small|medium|large|x-large|xx-large|xxx-large|smaller|larger)$`)).Globally()
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("https", "http", "mailto")
	p.RequireNoFollowOnLinks(true)
	return p
}()

func sanitizePaymentNote(note string) string { return paymentNotePolicy.Sanitize(note) }
