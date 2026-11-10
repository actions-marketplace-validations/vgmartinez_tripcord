package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vgmartinez/tripcord/internal/embedded"
	"github.com/vgmartinez/tripcord/internal/project"
)

// The pin citations README.md and docs/**/*.md may make. A workflow or
// action citation captures whatever follows the @, so a branch, a tag,
// or a short sha is a citation too and is never current. A prose
// citation is only an inline code span that is exactly main@ and 7 to
// 40 lowercase hex digits; a `# main@` comment belongs to the citation
// before it. A bare sha code span (`6a12795`, or `W = 6a12795`) is a
// citation too when its line presents it as current: today, currently,
// or now. Elsewhere it names a commit, not a pin.
var (
	citedWorkflowPin = regexp.MustCompile(regexp.QuoteMeta(project.Repository) + `/\.github/workflows/tripcord-pr(?:-comment)?\.yml@([^\s#` + "`" + `"']+)`)
	citedActionPin   = regexp.MustCompile(regexp.QuoteMeta(project.Repository) + `/\.github/actions/tripcord@([^\s#` + "`" + `"']+)`)
	citedProsePin    = regexp.MustCompile("`main@([0-9a-f]{7,40})`")
	citedBarePin     = regexp.MustCompile("`(?:[A-Za-z_.]+ = )?([0-9a-f]{7,40})`")
	currentMarker    = regexp.MustCompile(`(?i)\b(?:today|currently|now)\b`)
)

// historicalMarker is what makes a citation explicitly historical, and
// only on the citation's own line in docs/release-notes.md.
var historicalMarker = regexp.MustCompile(`(?i)\b(?:previously|formerly)\b`)

const historicalDoc = "docs/release-notes.md"

type citationKind string

const (
	citeWorkflow  citationKind = "workflow"
	citeAction    citationKind = "action"
	citePreCommit citationKind = "pre-commit rev"
	citeProse     citationKind = "prose"
	citeBare      citationKind = "bare sha"
)

type pinCitation struct {
	where string
	line  int
	text  string
	kind  citationKind
	pin   string
}

type currentPinSet struct {
	workflow string
	action   string
}

func findCitations(where, text string) []pinCitation {
	var out []pinCitation
	add := func(kind citationKind, re *regexp.Regexp) {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			start := m[2]
			lineStart := strings.LastIndexByte(text[:start], '\n') + 1
			lineEnd := strings.IndexByte(text[start:], '\n')
			if lineEnd < 0 {
				lineEnd = len(text)
			} else {
				lineEnd += start
			}
			if kind == citeBare && !currentMarker.MatchString(text[lineStart:lineEnd]) {
				continue
			}
			out = append(out, pinCitation{
				where: where,
				line:  strings.Count(text[:start], "\n") + 1,
				text:  text[lineStart:lineEnd],
				kind:  kind,
				pin:   text[m[2]:m[3]],
			})
		}
	}
	add(citeWorkflow, citedWorkflowPin)
	add(citeAction, citedActionPin)
	add(citePreCommit, anyPreCommitRev)
	add(citeProse, citedProsePin)
	add(citeBare, citedBarePin)
	return out
}

func (c pinCitation) current(cur currentPinSet) bool {
	switch c.kind {
	case citeWorkflow, citePreCommit:
		return c.pin == cur.workflow
	case citeAction:
		return c.pin == cur.action
	case citeProse, citeBare:
		return strings.HasPrefix(cur.workflow, c.pin) || strings.HasPrefix(cur.action, c.pin)
	default:
		panic(fmt.Sprintf("unhandled citation kind %q", c.kind))
	}
}

// knownCitedPin reports that pin is a KnownWorkflowPins entry: the full
// sha, or a prefix of at least 7 digits that only one entry has.
func knownCitedPin(pin string, known []string) bool {
	if len(pin) < 7 {
		return false
	}
	n := 0
	for _, k := range known {
		if strings.HasPrefix(k, pin) {
			n++
		}
	}
	return n == 1
}

// historical reports a citation with explicit historical context: it
// is in docs/release-notes.md and its own line says previously or
// formerly.
func (c pinCitation) historical() bool {
	return c.where == historicalDoc && historicalMarker.MatchString(c.text)
}

// staleCitations returns an error for each citation in texts that is
// not the current pin, unless the pin is in known and the citation is
// historical. An action citation is never historical: KnownWorkflowPins
// holds workflow pins only.
func staleCitations(texts map[string]string, cur currentPinSet, known []string) []string {
	var errs []string
	for _, where := range slices.Sorted(func(yield func(string) bool) {
		for k := range texts {
			if !yield(k) {
				return
			}
		}
	}) {
		for _, c := range findCitations(where, texts[where]) {
			if c.current(cur) {
				continue
			}
			if c.kind != citeAction && knownCitedPin(c.pin, known) && c.historical() {
				continue
			}
			want := cur.workflow
			switch c.kind {
			case citeAction:
				want = cur.action
			case citeProse, citeBare:
				want = cur.workflow + " or " + cur.action
			case citeWorkflow, citePreCommit:
			default:
				panic(fmt.Sprintf("unhandled citation kind %q", c.kind))
			}
			errs = append(errs, fmt.Sprintf("%s:%d: %s citation %s is not the current pin %s; cite the current pin, or, for a pin in KnownWorkflowPins, keep it only in %s on a line that says previously or formerly", c.where, c.line, c.kind, c.pin, want, historicalDoc))
		}
	}
	return errs
}

// docTexts is README.md and every docs/**/*.md, keyed by slash path
// relative to root.
func docTexts(t *testing.T, root string) map[string]string {
	t.Helper()
	texts := pinTexts(t, root)
	delete(texts, "generated tripcord.yml")
	return texts
}

// treeActionPin is the composite action pin both reusable workflows in
// this tree call. They must agree.
func treeActionPin(t *testing.T, root string) string {
	t.Helper()
	var pins []string
	for _, name := range []string{"tripcord-pr.yml", "tripcord-pr-comment.yml"} {
		raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		pins = append(pins, actionPins(t, name, string(raw))...)
	}
	slices.Sort(pins)
	pins = slices.Compact(pins)
	if len(pins) != 1 {
		t.Fatalf("the reusable workflows pin the composite action at %v, want exactly one sha", pins)
	}
	return pins[0]
}

// TestReleaseGateDocsCiteCurrentPins fails on a pin citation in
// README.md or docs/**/*.md that is not the current pin: a
// tripcord-pr.yml or tripcord-pr-comment.yml ref or a pre-commit rev
// other than WorkflowPin, an action ref other than the one this tree's
// reusable workflows call, or a `main@` code span, or a bare sha code
// span on a line that says it is current, that is a prefix of neither. The only exception is a KnownWorkflowPins entry on a line of
// docs/release-notes.md that says previously or formerly. It reads only
// this tree, so it runs on every pull request as well as in the
// release gate.
func TestReleaseGateDocsCiteCurrentPins(t *testing.T) {
	root := filepath.Join("..", "..")
	texts := docTexts(t, root)
	cur := currentPinSet{workflow: embedded.WorkflowPin, action: treeActionPin(t, root)}
	kinds := map[citationKind]int{}
	for where, text := range texts {
		for _, c := range findCitations(where, text) {
			kinds[c.kind]++
		}
	}
	if kinds[citeWorkflow] == 0 || kinds[citePreCommit] == 0 || kinds[citeProse] == 0 {
		t.Fatalf("found too few pin citations to check: %v", kinds)
	}
	for _, e := range staleCitations(texts, cur, embedded.KnownWorkflowPins) {
		t.Error("release gate: " + e)
	}
}

func TestReleaseGateStaleCitations(t *testing.T) {
	t.Parallel()
	repo := project.Repository
	cur := currentPinSet{workflow: strings.Repeat("c", 40), action: strings.Repeat("a", 40)}
	old := "0123456789abcdef0123456789abcdef01234567"
	unknown := strings.Repeat("e", 40)
	known := []string{old}
	workflow := func(pin string) string {
		return "    uses: " + repo + "/.github/workflows/tripcord-pr.yml@" + pin + " # main@" + pin[:min(7, len(pin))] + "\n"
	}
	comment := func(pin string) string {
		return "    uses: " + repo + "/.github/workflows/tripcord-pr-comment.yml@" + pin + "\n"
	}
	action := func(pin string) string {
		return "      - uses: " + repo + "/.github/actions/tripcord@" + pin + "\n"
	}
	rev := func(pin string) string {
		return "  - repo: https://github.com/" + repo + "\n    rev: " + pin + " # main@" + pin[:7] + "\n"
	}
	cases := []struct {
		name  string
		where string
		text  string
		stale int
	}{
		{"current everywhere", "README.md", workflow(cur.workflow) + comment(cur.workflow) + rev(cur.workflow) + action(cur.action) + "at `main@cccccccc` and `main@aaaaaaa`.\n", 0},
		{"stale workflow in README", "README.md", workflow(old), 1},
		{"stale comment workflow in docs", "docs/github-action.md", comment(old), 1},
		{"stale pre-commit rev in docs", "docs/pre-commit.md", rev(old), 1},
		{"stale action in docs", "docs/github-action.md", action(old), 1},
		{"stale prose in docs", "docs/init.md", "pins the reusable workflow at `main@0123456`.\n", 1},
		{"branch ref", "README.md", workflow("main"), 1},
		{"short action ref", "docs/x.md", action("aaaaaaa"), 1},
		{"nested docs file", "docs/rules/x.md", "at `main@0123456`\n", 1},
		{"historical prose in release notes", historicalDoc, "Embeds previously pinned at `main@0123456` are outdated, low.\n", 0},
		{"historical full ref in release notes", historicalDoc, "Formerly:\n" + "Formerly " + strings.TrimLeft(workflow(old), " "), 0},
		{"historical in release notes, no marker", historicalDoc, "Embeds pinned at `main@0123456` are outdated, low.\n", 1},
		{"historical marker on another line", historicalDoc, "Previously:\n`main@0123456`\n", 1},
		{"historical marker, unknown pin", historicalDoc, "previously `main@eeeeeee`\n" + "previously " + strings.TrimLeft(workflow(unknown), " "), 2},
		{"historical marker, short prefix too short", historicalDoc, "previously `main@012345`\n", 0},
		{"historical marker outside release notes", "docs/init.md", "previously `main@0123456`\n", 1},
		{"historical marker in README", "README.md", "previously " + strings.TrimLeft(workflow(old), " "), 1},
		{"historical action is never allowed", historicalDoc, "previously " + strings.TrimLeft(action(old), " "), 1},
		{"word containing the marker", historicalDoc, "unpreviously `main@0123456`\n", 1},
		{"bare sha said to be today's", "docs/design/x.md", "the pin (today `0123456`).\n", 1},
		{"bare assignment said to be today's", "docs/design/x.md", "the action (today `A = 0123456`).\n", 1},
		{"bare sha currently", "README.md", "currently pinned at `eeeeeee`\n", 1},
		{"bare current sha today", "docs/x.md", "today `ccccccc` and `aaaaaaa`\n", 0},
		{"bare sha without a current marker", "docs/design/x.md", "commit `0123456` added the check.\n", 0},
		{"bare sha, marker inside a word", "docs/x.md", "knowledge `0123456`\n", 0},
		{"bare sha, marker on another line", "docs/x.md", "today:\n`0123456`\n", 0},
		{"longer hex span is not a sha", "docs/x.md", "today `" + strings.Repeat("0", 64) + "`\n", 0},
	}
	for _, c := range cases {
		errs := staleCitations(map[string]string{c.where: c.text}, cur, known)
		if len(errs) != c.stale {
			t.Errorf("%s: %d stale citations, want %d: %q", c.name, len(errs), c.stale, errs)
		}
	}
}

func TestReleaseGateStaleCitationLine(t *testing.T) {
	t.Parallel()
	cur := currentPinSet{workflow: strings.Repeat("c", 40), action: strings.Repeat("a", 40)}
	errs := staleCitations(map[string]string{"docs/init.md": "one\ntwo\nat `main@0123456`\n"}, cur, nil)
	if len(errs) != 1 || !strings.HasPrefix(errs[0], "docs/init.md:3: prose citation 0123456 ") {
		t.Fatalf("errs = %q", errs)
	}
}
