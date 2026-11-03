package wiki

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// fenceMarker returns the backtick or tilde run that opens or closes a
// fenced code block on line, or "" when line is not a fence. info reports
// whether text follows the run, which only an opening fence may have.
func fenceMarker(line string) (marker string, info bool) {
	t := strings.TrimLeft(strings.TrimRight(line, "\r\n"), " \t")
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return "", false
	}
	n := 0
	for n < len(t) && t[n] == t[0] {
		n++
	}
	if n < 3 {
		return "", false
	}
	return t[:n], strings.TrimSpace(t[n:]) != ""
}

// proseLines calls fn with each line of md, in order, and whether it is
// outside fenced code. Fence lines themselves are code.
func proseLines(md string, fn func(line string, prose bool)) {
	open := ""
	for _, line := range strings.SplitAfter(md, "\n") {
		if line == "" {
			continue
		}
		marker, info := fenceMarker(line)
		switch {
		case open != "":
			if marker != "" && !info && marker[0] == open[0] && len(marker) >= len(open) {
				open = ""
			}
			fn(line, false)
		case marker != "":
			open = marker
			fn(line, false)
		default:
			fn(line, true)
		}
	}
}

// mapLinks replaces the target of every inline link and image outside
// fenced code and code spans with fn's result. Errors from fn are joined;
// the target is left as it was on error.
func mapLinks(md string, fn func(target string) (string, error)) (string, error) {
	var out strings.Builder
	var errs []error
	proseLines(md, func(line string, prose bool) {
		if !prose {
			out.WriteString(line)
			return
		}
		out.WriteString(mapLineLinks(line, fn, &errs))
	})
	return out.String(), errors.Join(errs...)
}

func mapLineLinks(line string, fn func(string) (string, error), errs *[]error) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] == '`' {
			n := 1
			for i+n < len(line) && line[i+n] == '`' {
				n++
			}
			run := line[i : i+n]
			if end := strings.Index(line[i+n:], run); end >= 0 {
				stop := i + n + end + n
				b.WriteString(line[i:stop])
				i = stop
				continue
			}
			b.WriteString(run)
			i += n
			continue
		}
		if strings.HasPrefix(line[i:], "](") {
			j := i + 2
			for j < len(line) && line[j] != ')' && line[j] != ' ' && line[j] != '\n' {
				j++
			}
			if j < len(line) && line[j] == ')' {
				target := line[i+2 : j]
				next, err := fn(target)
				if err != nil {
					*errs = append(*errs, err)
					next = target
				}
				b.WriteString("](")
				b.WriteString(next)
				b.WriteString(")")
				i = j + 1
				continue
			}
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// links returns every inline link and image target in md outside code.
func links(md string) []string {
	var out []string
	_, _ = mapLinks(md, func(t string) (string, error) {
		out = append(out, t)
		return t, nil
	})
	return out
}

var (
	headingRE    = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)
	inlineLinkRE = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	htmlTagRE    = regexp.MustCompile(`<[^>]+>`)
)

// Slug is the anchor GitHub gives a heading: the rendered text in lower
// case, with spaces turned into hyphens and every other character that is
// not a letter, digit, hyphen, or underscore removed.
func Slug(heading string) string {
	text := inlineLinkRE.ReplaceAllString(heading, "$1")
	text = htmlTagRE.ReplaceAllString(text, "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// Anchors returns the heading anchors of md. A repeated slug gets -1, -2,
// and so on, in document order, as GitHub numbers them.
func Anchors(md string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	proseLines(md, func(line string, prose bool) {
		if !prose {
			return
		}
		m := headingRE.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
		if m == nil {
			return
		}
		s := Slug(m[1])
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	})
	return out
}
