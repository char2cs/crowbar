package svgicon_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/core/svgicon"
)

func TestValidate_AcceptsPlainSVG(t *testing.T) {
	cases := map[string]string{
		"a shipped-style path": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="M22.28 9.82a5.98 5.98 0 0 0-.51-4.91z"/></svg>`,
		"no namespace":         `<svg viewBox="0 0 1 1"><rect width="1" height="1"/></svg>`,
		"a titled icon":        `<svg viewBox="0 0 1 1"><title>Agent &amp; co &lt;3</title><desc>a dot</desc><circle cx="0.5" cy="0.5" r="0.5"/></svg>`,
		"a gradient fill": `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 2 2">` +
			`<defs><linearGradient id="g" x1="0" x2="1" gradientTransform="rotate(45)"><stop offset="0" stop-color="rgb(1, 2, 3)"/></linearGradient>` +
			`<radialGradient id="h" xlink:href="#g" href="#g"/></defs>` +
			`<g transform="matrix(1 0 0 1 0 0) translate(1,1)" clip-path="url(#c)"><polygon points="0,0 1,1" fill="url('#g') red" stroke="hsl(10 20% 30%)" stroke-width="2" stroke-linecap="round"/></g>` +
			`<clipPath id="c"><ellipse rx="1" ry="1"/></clipPath><mask id="m"><line x1="0" y1="0" x2="1" y2="1"/></mask></svg>`,
		"a plain style": `<svg viewBox="0 0 1 1"><path style="fill:#fff;stroke-width:2" d="M0 0"/></svg>`,
	}
	for name, icon := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, svgicon.Validate(icon))
		})
	}
}

func TestValidate_RejectsAnythingThatCanRunOrFetch(t *testing.T) {
	cases := map[string]string{
		"script element":              `<svg><script>alert(1)</script></svg>`,
		"onload handler":              `<svg onload="alert(1)"/>`,
		"mixed-case handler":          `<svg><path onClick="alert(1)" d="M0 0"/></svg>`,
		"foreignObject":               `<svg><foreignObject><div xmlns="http://www.w3.org/1999/xhtml">x</div></foreignObject></svg>`,
		"javascript: href":            `<svg><linearGradient id="g" href="javascript:alert(1)"/></svg>`,
		"javascript: xlink:href":      `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><linearGradient id="g" xlink:href="javascript:alert(1)"/></svg>`,
		"external image":              `<svg><image href="https://example.com/x.png"/></svg>`,
		"use of an external document": `<svg><use href="https://example.com/s.svg#a"/></svg>`,
		"use of a local symbol":       `<svg><use href="#a"/></svg>`,
		"animate":                     `<svg><animate attributeName="href" to="javascript:alert(1)"/></svg>`,
		"set":                         `<svg><set attributeName="onmouseover" to="alert(1)"/></svg>`,
		"iframe":                      `<svg><iframe src="https://example.com"/></svg>`,
		"object":                      `<svg><object data="https://example.com"/></svg>`,
		"style element":               `<svg><style>*{background:url(https://example.com)}</style></svg>`,
		"style with url":              `<svg><path style="fill:url(#g)" d="M0 0"/></svg>`,
		"style with expression":       `<svg><path style="width:expression(alert(1))" d="M0 0"/></svg>`,
		"style with image-set":        `<svg><path style="mask:image-set('https://example.com/x.png' 1x)" d="M0 0"/></svg>`,
		"style with an escape":        `<svg><path style="fill:u\72l(https://example.com)" d="M0 0"/></svg>`,
		"external fill":               `<svg><path fill="url(https://example.com/s.svg#g)" d="M0 0"/></svg>`,
		"function mask":               `<svg><path mask="image-set('https://example.com/x.png' 1x)" d="M0 0"/></svg>`,
		"external href on gradient":   `<svg><linearGradient id="g" href="https://example.com/s.svg#g"/></svg>`,
		"CDATA breaking out of title": `<svg><title><![CDATA[</title><img src=x onerror=alert(1)>]]></title></svg>`,
		"comment":                     `<svg><!-- --><path d="M0 0"/></svg>`,
		"processing instruction":      `<svg><?x > <img src=x onerror=alert(1)> ?></svg>`,
		"doctype":                     `<!DOCTYPE svg><svg/>`,
		"element inside title":        `<svg><title><b>x</b></title></svg>`,
		"text outside title":          `<svg>hello<path d="M0 0"/></svg>`,
		"second root":                 `<svg/><svg onload="alert(1)"/>`,
		"root is not svg":             `<g><path d="M0 0"/></g>`,
		"html breakout element":       `<svg><p>x</p></svg>`,
		"namespaced attribute":        `<svg xmlns:ev="http://www.w3.org/2001/xml-events" ev:event="click"/>`,
		"other default namespace":     `<svg xmlns="http://www.w3.org/1999/xhtml"/>`,
		"prefixed element":            `<x:svg xmlns:x="http://www.w3.org/2000/svg"/>`,
		"undeclared entity":           `<svg><title>&nbsp;</title></svg>`,
		"malformed":                   `<svg><path d="M0 0"></svg>`,
		"unknown attribute":           `<svg filter="url(#f)"/>`,
		"empty":                       ``,
		"bare text":                   `hello`,
	}
	for name, icon := range cases {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, svgicon.Validate(icon), svgicon.ErrUnsafe)
		})
	}
}
