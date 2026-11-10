package shellcmd

import (
	"slices"
	"strings"
)

// Words returns the words of the first simple command in line, with
// quotes removed and no expansions applied. ok is false when line is
// empty or a command substitution ($(...), backticks, or <(...)) would
// have to be expanded to know the words. An operator ends the command,
// so the words of a pipeline stop at the pipe.
func Words(line string) ([]string, bool) {
	toks, _ := lex(line, false)
	var words []string
	for _, t := range toks {
		if t.op != "" {
			break
		}
		if strings.Contains(t.word, subst) {
			return nil, false
		}
		words = append(words, t.word)
	}
	if len(words) == 0 {
		return nil, false
	}
	return words, true
}

// Commands returns the words of every simple command in line, with
// quotes removed, no expansions applied, and redirection targets
// dropped. A command with a word that needs a command substitution is
// left out, and so is every command inside a substitution.
func Commands(line string) [][]string {
	toks, _ := lex(line, false)
	var out [][]string
	for _, c := range split(toks) {
		if slices.ContainsFunc(c.words, func(w string) bool { return strings.Contains(w, subst) }) {
			continue
		}
		out = append(out, c.words)
	}
	return out
}
