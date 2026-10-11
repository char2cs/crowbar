// Package svgicon admits a provider icon only as inert, plain SVG. The webview
// injects the icon as HTML, so markup that can run script, load a resource or
// parse differently as HTML than as XML would run inside the app.
package svgicon

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ErrUnsafe signals an icon that is not plain SVG.
var ErrUnsafe = errors.New("svgicon: not plain SVG")

const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

var elements = set(
	"svg", "g", "path", "circle", "ellipse", "rect", "line", "polyline", "polygon",
	"defs", "linearGradient", "radialGradient", "stop", "clipPath", "mask", "title", "desc",
)

// textOnly elements are HTML integration points: an HTML parser reads their
// content as HTML, so nothing but text may sit inside them.
var textOnly = set("title", "desc")

// attributes adds to the listed set only the gradient and opacity attributes a
// gradient-filled icon needs; stroke-* is admitted by prefix.
var attributes = set(
	"d", "fill", "fill-opacity", "fill-rule", "stroke", "viewBox", "width", "height",
	"transform", "opacity", "offset", "stop-color", "stop-opacity", "points",
	"cx", "cy", "r", "rx", "ry", "x", "y", "x1", "x2", "y1", "y2", "fx", "fy",
	"clip-path", "clip-rule", "mask", "id", "class", "style", "href",
	"gradientUnits", "gradientTransform", "spreadMethod",
)

var transformAttributes = set("transform", "gradientTransform")

var (
	sameDocumentRef = regexp.MustCompile(`^#[A-Za-z_][\w.:-]*$`)
	cssFunction     = regexp.MustCompile(`([A-Za-z-]*)\s*\(`)
	sameDocumentURL = regexp.MustCompile(`^url\(\s*(['"]?)#[A-Za-z_][\w.:-]*['"]?\s*\)`)
)

var (
	colorFunctions     = set("rgb", "rgba", "hsl", "hsla")
	transformFunctions = set("matrix", "translate", "scale", "rotate", "skewX", "skewY")
)

// Validate reports why icon is not a single plain-SVG document, or nil. It
// rejects comments, CDATA, doctypes and processing instructions outright: an
// HTML parser ends those at a different byte than XML does.
func Validate(
	icon string,
) error {
	if strings.Contains(icon, "<!") || strings.Contains(icon, "<?") {
		return fmt.Errorf("%w: comments, CDATA, doctypes and processing instructions are not allowed", ErrUnsafe)
	}
	decoder := xml.NewDecoder(strings.NewReader(icon))
	w := &walker{}
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return w.finish()
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnsafe, err)
		}
		if err := w.visit(tok); err != nil {
			return err
		}
	}
}

type walker struct {
	open   []string
	rooted bool
}

func (w *walker) visit(
	tok xml.Token,
) error {
	switch t := tok.(type) {
	case xml.StartElement:
		return w.start(t)
	case xml.EndElement:
		w.open = w.open[:len(w.open)-1]
		return nil
	case xml.CharData:
		return w.text(t)
	default:
		return fmt.Errorf("%w: unexpected %T", ErrUnsafe, tok)
	}
}

func (w *walker) start(
	el xml.StartElement,
) error {
	name := el.Name.Local
	switch {
	case len(w.open) == 0 && (w.rooted || name != "svg"):
		return fmt.Errorf("%w: the document must be a single <svg> element", ErrUnsafe)
	case len(w.open) > 0 && textOnly[w.open[len(w.open)-1]]:
		return fmt.Errorf("%w: <%s> may hold only text", ErrUnsafe, w.open[len(w.open)-1])
	case !elements[name] || (el.Name.Space != "" && el.Name.Space != svgNamespace):
		return fmt.Errorf("%w: element <%s> is not allowed", ErrUnsafe, qualified(el.Name))
	}
	for _, attr := range el.Attr {
		if err := checkAttribute(attr); err != nil {
			return err
		}
	}
	w.rooted = true
	w.open = append(w.open, name)
	return nil
}

func (w *walker) text(
	data xml.CharData,
) error {
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	if len(w.open) > 0 && textOnly[w.open[len(w.open)-1]] {
		return nil
	}
	return fmt.Errorf("%w: text is allowed only inside <title> and <desc>", ErrUnsafe)
}

func (w *walker) finish() error {
	if !w.rooted {
		return fmt.Errorf("%w: no <svg> element", ErrUnsafe)
	}
	return nil
}

func checkAttribute(
	attr xml.Attr,
) error {
	name := qualified(attr.Name)
	switch {
	case strings.HasPrefix(strings.ToLower(attr.Name.Local), "on"):
		return fmt.Errorf("%w: event handler %s is not allowed", ErrUnsafe, name)
	case attr.Name.Space == "" && attr.Name.Local == "xmlns":
		return expectValue(attr, svgNamespace)
	case attr.Name.Space == "xmlns" && attr.Name.Local == "xlink":
		return expectValue(attr, xlinkNamespace)
	case attr.Name.Local == "href" && isHrefSpace(attr.Name.Space):
		return checkHref(attr)
	case attr.Name.Space != "" || !isAllowedAttribute(attr.Name.Local):
		return fmt.Errorf("%w: attribute %s is not allowed", ErrUnsafe, name)
	}
	return checkValue(attr)
}

func isHrefSpace(
	space string,
) bool {
	return space == "" || space == "xlink" || space == xlinkNamespace
}

func isAllowedAttribute(
	local string,
) bool {
	return attributes[local] || strings.HasPrefix(local, "stroke-")
}

func expectValue(
	attr xml.Attr,
	want string,
) error {
	if attr.Value != want {
		return fmt.Errorf("%w: %s must be %q", ErrUnsafe, qualified(attr.Name), want)
	}
	return nil
}

func checkHref(
	attr xml.Attr,
) error {
	if !sameDocumentRef.MatchString(attr.Value) {
		return fmt.Errorf("%w: %s may only reference an element of the icon itself (#id)", ErrUnsafe, qualified(attr.Name))
	}
	return nil
}

// checkValue admits only CSS functions that cannot fetch or run anything:
// colours, transforms in a transform attribute, url(#id) outside style. No
// escapes or at-rules, so no other function can be spelled past the check.
func checkValue(
	attr xml.Attr,
) error {
	name := attr.Name.Local
	if strings.ContainsAny(attr.Value, `\@`) {
		return fmt.Errorf("%w: %s may not hold CSS escapes or at-rules", ErrUnsafe, name)
	}
	for _, at := range cssFunction.FindAllStringSubmatchIndex(attr.Value, -1) {
		fn := attr.Value[at[2]:at[3]]
		if !allowedFunction(name, fn, attr.Value[at[2]:]) {
			return fmt.Errorf("%w: %s may not call %s()", ErrUnsafe, name, fn)
		}
	}
	return nil
}

func allowedFunction(
	attr, fn, call string,
) bool {
	if transformAttributes[attr] {
		return transformFunctions[fn]
	}
	if strings.EqualFold(fn, "url") {
		return attr != "style" && sameDocumentURL.MatchString(call)
	}
	return colorFunctions[strings.ToLower(fn)]
}

func qualified(
	name xml.Name,
) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

func set(
	names ...string,
) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}
