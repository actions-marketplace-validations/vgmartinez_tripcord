package kit

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// JSONLines maps each value of a strict JSON document to the 1-based
// line it starts on, keyed by its JSON Pointer (RFC 6901), such as
// "/hooks/Stop/0/hooks/0/command". The root is "". An exact duplicate
// key keeps the last occurrence, as DecodeJSON does. A document that
// does not parse yields the lines read before the error; callers use
// the map only for display, so a missing entry is line 0 (unknown).
func JSONLines(raw []byte) map[string]int {
	lines := map[string]int{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	p := &jsonPos{dec: dec, raw: raw, lines: lines}
	_ = p.value(0, "")
	return lines
}

type jsonPos struct {
	dec   *json.Decoder
	raw   []byte
	lines map[string]int
	off   int // bytes of raw counted into line so far
	line  int // 1-based line of raw[off]
}

// at is the 1-based line of the last byte of the token just read. A
// JSON string cannot hold a raw newline, so a scalar token ends on the
// line it starts on, and a delimiter is one byte.
func (p *jsonPos) at() int {
	end := min(int(p.dec.InputOffset()), len(p.raw)) - 1
	if p.line == 0 {
		p.line = 1
	}
	if end > p.off {
		p.line += bytes.Count(p.raw[p.off:end], []byte{'\n'})
		p.off = end
	}
	return p.line
}

func (p *jsonPos) value(depth int, ptr string) error {
	tok, err := p.dec.Token()
	if err != nil {
		return err
	}
	if _, seen := p.lines[ptr]; seen {
		// A later duplicate key replaces the earlier value whole.
		for k := range p.lines {
			if strings.HasPrefix(k, ptr+"/") {
				delete(p.lines, k)
			}
		}
	}
	p.lines[ptr] = p.at()
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= MaxNesting {
		return nil
	}
	switch delim {
	case '{':
		for p.dec.More() {
			keyTok, err := p.dec.Token()
			if err != nil {
				return err
			}
			key, _ := keyTok.(string)
			if err := p.value(depth+1, ptr+"/"+escapePointer(key)); err != nil {
				return err
			}
		}
	case '[':
		for i := 0; p.dec.More(); i++ {
			if err := p.value(depth+1, ptr+"/"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
	}
	_, err = p.dec.Token()
	return err
}

// JSONPointer joins tokens into a JSON Pointer, escaping "~" and "/".
func JSONPointer(tokens ...string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(escapePointer(t))
	}
	return b.String()
}

func escapePointer(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
