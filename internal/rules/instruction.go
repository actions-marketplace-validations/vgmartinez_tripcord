package rules

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/vgmartinez/tripcord/internal/hiddenchar"
	"github.com/vgmartinez/tripcord/internal/model"
)

var (
	instructionPrefix = regexp.MustCompile(`(?i)^[ \t]*(?:(?:#{1,6}|>+|<!--[ \t]*(?:(?:assistant|user|system|human):[ \t]+)?)[ \t]*|(?:[-*+]|\d+[.)])[ \t]+|\*\*[ \t]*)*(?:(?:please|always|first|you[ \t]+(?:must|should)|make[ \t]+sure[ \t]+to)[,:; \t]+)*`)
	// Longer verbs come first so get-content is not read as get and
	// printenv is not read as print. look up is one phrasal verb.
	instructionRead = regexp.MustCompile(`(?i)\b(?:look[ \t]+up|get-content|printenv|access|echo|retrieve|read|cat|cp|copy|open|load|source|print|dump|collect|extract|show|curl|wget|type|get)\b`)
	// Copulas, auxiliaries, and clause words end the noun phrase. A
	// hyphenated token is not plain, so it is not listed here.
	instructionGapStop  = regexp.MustCompile(`(?i)^(?:not|never|no|don'?t|without|avoid|if|when|where|while|unless|once|and|or|but|then|before|after|is|are|was|were|be|been|being|am|despite|cannot|can|could|should|would|will|shall|may|might|must|do|does|did|have|has|had|first|next|above|below|however|because|since|although|though|until)$|^(?:won|can|don|isn|aren|wasn|weren|couldn|shouldn|wouldn|doesn|didn|hasn|haven|hadn)['’]t$`)
	instructionGapPrep  = regexp.MustCompile(`(?i)^(?:from|in|of|on|at|into|inside|within|via|through|using|with|for)$`)
	instructionGapDet   = regexp.MustCompile(`(?i)^(?:the|all|your|my|our|this|these|any|every|each|its|their|those|a|an)$`)
	instructionGapPlain = regexp.MustCompile(`^[A-Za-z][A-Za-z']*$`)
	// A secret value names what is read, not where it is stored. Outside a
	// direct object of a read verb it needs a local path after it.
	instructionSecretValue = regexp.MustCompile(`(?i)^(?:environment[ \t]+variables?|credentials|secrets?|api[_ ]?keys?|tokens?|passwords?|private[ \t]+key)$`)
	instructionStoredPrep  = regexp.MustCompile(`(?i)^(?:from|in|inside|within|under|of|via|using)$`)
	instructionLocalPath   = regexp.MustCompile(`^(?:~/|\$\{?HOME\}?/|\.{1,2}/|/|\.[\p{L}\p{N}_]|[\p{L}\p{N}_.-]+/)`)
	instructionRedirect    = regexp.MustCompile(`(?:^|[^-])>`)
	// "access token" and "access key" are compound nouns, not the verb.
	instructionAccessCompound = regexp.MustCompile(`(?i)^[ \t]*(?:tokens?|keys?)\b`)
	instructionOpenPR         = regexp.MustCompile(`(?i)^[ \t]+(?:a[ \t]+)?(?:pull[ \t]+request|pr)\b`)
	instructionSend           = regexp.MustCompile(`(?i)\b(?:send|post|upload|email|exfiltrate|transmit|forward)\b`)
	instructionSecret         = regexp.MustCompile(`(?i)(?:\.env(?:\.(?:local|production|development|prod|dev))?(?:\.?(?:[^\w.]|$)|$)|\bprintenv\b|\bprocess\.env\b|\bos\.environ\b|environment variables?|\bcredentials\b|\bsecrets?\b|api[_ ]?keys?\b|\btokens?\b|\bpasswords?\b|private key|(?:~|\$HOME|\$\{HOME\}|/root|/home/[^\s/]+)/\.(?:aws|ssh)|/proc/(?:self|\$(?:\{PPID\}|PPID)|\d+)/environ(?:\.?(?:[^\w.]|$)|$)|\bid_rsa\b|\b(?:aws_[a-z0-9_]+|github_token)\b)`)
	instructionObject         = regexp.MustCompile(`(?i)^[ \t]*(?:(?:--?[a-z][a-z0-9-]*(?:[= ][^ \t]*)?[ \t]+))*?(?:https?://[^ \t]+[ \t]+)?(?:(?:the|all|your|my|our|this|these|any|every|each|its|their|those)[ \t]+)*(?:(?:contents?|values?)[ \t]+of[ \t]+)?(?:\$env:)?(?:[` + "`" + `'"@][ \t]*)?`)
	instructionModifier       = regexp.MustCompile(`(?i)^[ \t]*(?:-[a-z]|(?:sections?|polic(?:y|ies)|flows?|counts?|offsets?|usage|budgets?|docs?|documentation|readme|tables?|runbooks?|design|handling|management|templates?|reset|fields?|requirements?|pages?|code|rotation|formats?|limits?|rules?|guides?|examples?|scopes?|refresh|expiry|expiration|validation|forms?)\b)`)
	instructionClauseEnd      = regexp.MustCompile(`[;]|(?:\.(?:[ \t]|$))`)
	// A period ends a prose sentence. A curl span keeps ';' and the shell
	// operators, which separate commands rather than sentences.
	instructionProseEnd = regexp.MustCompile(`\.(?:[ \t]|$)`)
	// Plain http(s) URLs are ignored, except a URL that embeds $( or a
	// backtick: stripping that URL would hide $(cat .env) inside it.
	// Header and user flags are stripped by stripCurlFlags, which also
	// takes a glued quote, an equals form, and an unquoted header whose
	// value continues past the first word (Authorization: Bearer $TOKEN).
	instructionURLNoise = regexp.MustCompile(`(?i)\bhttps?://(?:(?:[^ \t$` + "`" + `]|\$[^(\t ])*\$?)(?:[ \t]|$)`)
	// "come from" / "comes from" names where a credential originates.
	// It is not an instruction to source a file.
	instructionComesFrom = regexp.MustCompile(`(?i)(?:^|[ \t])(?:come|comes)[ \t]+from\b`)
	instructionURL       = regexp.MustCompile(`(?i)\bhttps?://`)
	instructionNamedSink = regexp.MustCompile(`(?i)\b(?:webhooks?|gist|pastebin|slack|discord)\b`)
	instructionDataSink  = regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^\r\n]*(?:\B-(?:d|F)\b|--data(?:-[a-z]+)?(?:=|[ \t])|--form(?:=|[ \t])|(?:-X|--request)[ \t=]*POST\b)`)
	instructionToSink    = regexp.MustCompile(`(?i)\bto\b`)

	// A GitHub Actions secrets expression in an example workflow is a
	// placeholder the runner fills in, not a value the agent reads.
	instructionCIExpr = regexp.MustCompile(`\$\{\{[^}]*\bsecrets\.[^}]*\}\}`)
	// A lead-in that ends with ':' and negates carries to the list under it.
	instructionNegLeadIn = regexp.MustCompile(`(?i)\b(?:never|do[ \t]+not|don['’]t|must[ \t]+not|avoid|forbidden)\b.*:[ \t*_]*$`)
	instructionListItem  = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d+[.)])[ \t]+`)
	// A bare token or password that names a UI element, a count, a
	// design value, or a parser unit, not a credential.
	instructionBareTokenWord = regexp.MustCompile(`(?i)^(?:tokens?|passwords?)$`)
	instructionHomonymTail   = regexp.MustCompile(`(?i)^[ \t]+(?:(?:cost|count|budget|usage|limit|input|field|toggle|strength|indicator|mapping)s?\b|for[ \t]+each[ \t]+state\b)`)
	instructionHomonymVia    = regexp.MustCompile(`(?i)^[ \t]+(?:through|via)[ \t]+(?:(?:the|a|an)[ \t]+)?(` + "`" + `[^` + "`" + `]*` + "`" + `?)`)
	// "only to", "only when", "only if" after the secret restrict where or
	// when it is used; they do not ask for it.
	instructionRestrictTail = regexp.MustCompile(`(?i)^[ \t]+only[ \t]+(?:to|when|if)\b`)
	// printenv with stdout sent to /dev/null only tests that a name is set.
	instructionDevNull = regexp.MustCompile(`(?:^|[ \t])[1&]?>[ \t]*/dev/null\b`)
	instructionSSHPub  = regexp.MustCompile(`^(?:~|\$HOME|\$\{HOME\}|/root|/home/[^\s/]+)/\.ssh/[^\s/]+\.pub(?:[` + "`" + `'")\].,;:]*)$`)
	// Secure-coding guidance: the secret comes from the environment or a
	// secret store, with a word that makes it a rule about code.
	instructionFromRuntimeStore = regexp.MustCompile(`(?i)\b(?:secrets?|credentials|api[_ ]?keys?|tokens?|passwords?)[ \t]+from[ \t]+(?:the[ \t]+)?` + "`?" + `(?:environment(?:[ \t]+variables?)?\b|(?:(?:an?|the)[ \t]+)?(?:(?:approved|secure)[ \t]+)?(?:secrets?|credentials?)[ \t]+(?:managers?|stores?|storage)\b|(?:(?:an?|the)[ \t]+)?(?:secure[ \t]+)?vaults?\b|os\.getenv\b|process\.env\b|os\.environ\b|dotenv\b)`)
	instructionCodeRule         = regexp.MustCompile(`(?i)\b(?:never|only|exclusively)\b|\bin[ \t]+application[ \t]+code\b`)
	instructionCodeSink         = regexp.MustCompile(`(?i)\b(?:send|post|upload|paste|print|echo|log)\b`)
	// Steps a person does by hand in a browser or an editor.
	// A committed dotenv template holds placeholder names, not values.
	instructionEnvTemplate = regexp.MustCompile(`(?i)^(?:[\w.-]*/)?\.env\.(?:example|sample|template)(?:[` + "`" + `'")\].,;:]*)$`)
	instructionHumanStep   = regexp.MustCompile(`(?i)\bin[ \t]+(?:the|a|your)[ \t]+(?:web[ \t]+)?(?:browser|text[ \t]+editor|editor)\b|\bsettings[ \t]+page\b`)
	// A source that is not on the agent's machine: a capture, a protocol on
	// the wire, a memory image, a test target, or a placeholder host.
	instructionRemoteSource    = regexp.MustCompile(`(?i)\b(?:pcaps?|captures?|captured[ \t]+traffic|traffic|protocols?|memory[ \t]+images?|dump[ \t]+files?|target[ \t]+hosts?|targets?)\b`)
	instructionPlaceholderHost = regexp.MustCompile(`(?i)https?://(?:(?:[\w-]+\.)*(?:target|example)\.(?:com|org|net)\b|[\w.-]+\.(?:test|example|invalid)\b|\$\{?TARGET\b)`)
	// "to" / "into" immediately before a path names a destination
	// ("Copy this file to .env"), not the object being read.
	instructionCopyDest = regexp.MustCompile(`(?i)(?:^|[ \t])(?:to|into)$`)
)

// instructionMatchLine returns the first 1-based line that asks the agent to
// read a secret target or to send one to an outbound sink. It never returns
// the line text: findings expose only this coordinate.
func instructionMatchLine(content string) int {
	negList, negItems := false, false
	for line, text := range strings.Split(content, "\n") {
		text = stripInstructionHidden(text)
		text = instructionCIExpr.ReplaceAllString(text, " ")
		if negList {
			switch {
			case instructionListItem.MatchString(text):
				negItems = true
				continue
			case strings.TrimSpace(text) == "" && !negItems:
				continue
			case negItems && strings.TrimSpace(text) != "" && (text[0] == ' ' || text[0] == '\t'):
				continue
			}
			negList = false
		}
		if instructionNegLeadIn.MatchString(text) {
			negList, negItems = true, false
		}
		prefix := instructionPrefix.FindStringIndex(text)
		if prefix == nil {
			continue
		}
		rest := closeLeadingBold(text[prefix[1]:])
		if read := instructionRead.FindStringIndex(rest); read != nil {
			if read[0] == 0 {
				if strings.EqualFold(rest[read[0]:read[1]], "open") && instructionOpenPR.MatchString(rest[read[1]:]) {
					continue
				}
				verb := strings.ToLower(rest[read[0]:read[1]])
				// printenv is the read: it names no further object. A
				// documentation modifier ("printenv documentation") is quiet.
				if verb == "printenv" {
					if instructionModifier.MatchString(rest[read[1]:]) || instructionPrintenvQuiet(rest[read[1]:]) {
						continue
					}
					return line + 1
				}
				if verb == "curl" || verb == "wget" {
					// instructionCurlSpan keeps the commands after &&, ||,
					// |, and ;. instructionClause would drop them at ';'.
					raw := instructionCurlSpan(rest[read[1]:])
					span := stripInstructionCurlNoise(raw)
					if instructionSubstReadsPath(span) {
						return line + 1
					}
					// A request to a placeholder host is a test payload:
					// a value word there is a column or a form field.
					if instructionSecret.MatchString(span) &&
						(!instructionPlaceholderHost.MatchString(raw) || instructionStorageTarget(span)) {
						return line + 1
					}
				}
				clause := instructionClause(rest[read[1]:])
				if (verb == "copy" || verb == "open") && instructionHumanStep.MatchString(clause) {
					continue
				}
				if instructionNotLocal(clause) || instructionSecureCoding(instructionClause(rest)) {
					continue
				}
				if verb == "source" && instructionBareToken(clause) {
					continue
				}
				if verb == "echo" {
					if instructionEchoSecret(clause) {
						return line + 1
					}
					continue
				}
				if verb == "access" && instructionAccessCompound.MatchString(clause) {
					continue
				}
				if !instructionGapTarget(clause, 0, false, instructionAcquire(verb)) {
					continue
				}
				if (verb == "copy" || verb == "open") && !instructionCopyReadsLocal(clause) {
					continue
				}
				return line + 1
			}
		}
		if send := instructionSend.FindStringIndex(rest); send != nil {
			if send[0] != 0 {
				continue
			}
			after := instructionClause(rest[send[1]:])
			if !instructionDirectSecret(after) {
				continue
			}
			if instructionURL.MatchString(after) || instructionNamedSink.MatchString(after) ||
				instructionDataSink.MatchString(after) || instructionToSink.MatchString(after) {
				return line + 1
			}
		}
	}
	return 0
}

// closeLeadingBold drops a ** that closes bold right after the verb that
// starts rest, so **Read** the .env file reads as Read the .env file. The
// opening ** is already part of the prefix.
func closeLeadingBold(rest string) string {
	for _, verb := range []*regexp.Regexp{instructionRead, instructionSend} {
		if loc := verb.FindStringIndex(rest); loc != nil && loc[0] == 0 && strings.HasPrefix(rest[loc[1]:], "**") {
			return rest[:loc[1]] + rest[loc[1]+2:]
		}
	}
	return rest
}

// stripInstructionHidden drops bidi controls and zero-width characters
// so they cannot sit in front of a verb or split a target. Tag
// characters and other letters, including homoglyphs, stay.
func stripInstructionHidden(text string) string {
	return strings.Map(func(r rune) rune {
		switch hiddenchar.Class(r) {
		case hiddenchar.Bidi, hiddenchar.ZeroWidth:
			return -1
		default:
			return r
		}
	}, text)
}

// instructionWordGap is the most words that may sit between a read verb
// and its target. Every skipped token counts, including a determiner.
const instructionWordGap = 6

// instructionHit is a secret at the head of a phrase. value is set when it
// names the secret itself (an API key, a token) rather than where secrets
// are stored (.env, ~/.ssh, process.env, GITHUB_TOKEN). tail follows it.
type instructionHit struct {
	at, blocked, value bool
	tail               string
}

func instructionDirectSecret(text string) bool {
	h := instructionHead(text)
	return h.at && !h.blocked
}

// instructionHead reports whether a secret is the direct object of text.
// blocked is set when that secret is followed by a documentation modifier,
// which ends the search instead of letting the gap walk past it.
func instructionHead(text string) instructionHit {
	text = instructionObject.ReplaceAllString(text, "")
	secret := instructionSecret.FindStringIndex(text)
	if secret == nil || secret[0] != 0 {
		return instructionHit{}
	}
	tail := text[secret[1]:]
	if pub, rest := firstInstructionWord(text); instructionSSHPub.MatchString(pub) && !instructionSecret.MatchString(rest) {
		return instructionHit{}
	}
	word := text[:secret[1]]
	blocked := instructionModifier.MatchString(tail) || instructionRestrictTail.MatchString(tail)
	if instructionBareTokenWord.MatchString(word) && instructionHomonym(tail) {
		blocked = true
	}
	return instructionHit{
		at:      true,
		blocked: blocked,
		value:   instructionSecretValue.MatchString(strings.TrimRight(text[:secret[1]], " \t")),
		tail:    tail,
	}
}

// instructionHomonym reports a tail that makes a bare token or password
// something other than a credential: "token cost", "password input",
// "tokens for each state", or "tokens through Theme.colors". A code
// identifier after through or via that names a storage target, such as
// .env, keeps the match.
func instructionHomonym(tail string) bool {
	if instructionHomonymTail.MatchString(tail) {
		return true
	}
	m := instructionHomonymVia.FindStringSubmatch(tail)
	if m == nil || m[1] == "" {
		return false
	}
	ident := strings.Trim(m[1], "`")
	return ident != "" && !instructionStorageTarget(ident) && !instructionLocalPath.MatchString(ident)
}

// instructionBareToken reports a clause whose direct object is a bare
// token or password ("source tokens", "the password"). source runs a
// file, and a bare value word is not one.
func instructionBareToken(clause string) bool {
	text := instructionObject.ReplaceAllString(clause, "")
	word, _ := firstInstructionWord(text)
	return instructionBareTokenWord.MatchString(strings.TrimRight(word, ".,;:"))
}

// instructionStorageTarget reports a secret target in s that names where
// secrets are stored (.env, ~/.aws, process.env, AWS_*), not only a
// value word such as password or token.
func instructionStorageTarget(s string) bool {
	for _, m := range instructionSecret.FindAllString(s, -1) {
		if !instructionSecretValue.MatchString(strings.TrimRight(m, " \t")) {
			return true
		}
	}
	return false
}

// instructionPrintenvQuiet reports printenv whose stdout goes to
// /dev/null, which tests that a name is set and reads no value. The
// check stops at the first pipe or command separator, so
// printenv NAME | curl ... is not quiet.
func instructionPrintenvQuiet(after string) bool {
	cmd := after
	if i := strings.IndexAny(cmd, "|;"); i >= 0 {
		cmd = cmd[:i]
	}
	if i := strings.Index(cmd, "&&"); i >= 0 {
		cmd = cmd[:i]
	}
	return instructionDevNull.MatchString(cmd)
}

// instructionCopyReadsLocal reports whether copy or open is reading a
// local secret path or storage target as its object or source. A path
// that appears only as a destination ("Copy this file to .env") or a
// dotenv template (.env.example) does not count. "Copy .env to the
// scratch directory" and "Copy the token from ~/.config/tool/token
// into the report" still count.
func instructionCopyReadsLocal(clause string) bool {
	text := clause
	for text != "" {
		h := instructionHead(text)
		secret := instructionSecret.FindStringIndex(text)
		if secret == nil {
			return false
		}
		before := strings.TrimRight(strings.TrimSpace(text[:secret[0]]), "`'\"([")
		before = strings.TrimSpace(before)
		matched := strings.TrimRight(strings.Trim(text[secret[0]:secret[1]], "`'\""), " \t")
		if instructionCopyDest.MatchString(before) || instructionEnvTemplate.MatchString(matched) || (h.at && h.blocked && secret[0] == 0) {
			text = text[secret[1]:]
			continue
		}
		if !instructionSecretValue.MatchString(matched) {
			return true
		}
		if instructionStored(h.tail) {
			return true
		}
		text = text[secret[1]:]
	}
	return false
}

// instructionNotLocal reports a clause about a source that is not on the
// agent's machine, such as "credentials from the captured traffic" or
// "from memory of the target host", with no storage target or local path.
func instructionNotLocal(clause string) bool {
	if !instructionRemoteSource.MatchString(clause) && !instructionPlaceholderHost.MatchString(clause) {
		return false
	}
	return !instructionStorageTarget(clause) && !instructionHasLocalPath(clause)
}

// instructionSecureCoding reports guidance about application code: the
// secret comes from the environment or a secret store, a word such as
// never, only, or exclusively makes it a rule, and nothing sends or
// prints it or names a local secret path.
func instructionSecureCoding(clause string) bool {
	if !instructionFromRuntimeStore.MatchString(clause) || !instructionCodeRule.MatchString(clause) {
		return false
	}
	if instructionCodeSink.MatchString(clause) || instructionHasLocalPath(clause) {
		return false
	}
	return !instructionStorageTargetBesides(clause)
}

// instructionStorageTargetBesides reports a storage target other than
// process.env and os.environ, which instructionSecureCoding names as the
// runtime source.
func instructionStorageTargetBesides(clause string) bool {
	for _, m := range instructionSecret.FindAllString(clause, -1) {
		l := strings.ToLower(strings.TrimRight(m, " \t"))
		if l == "process.env" || l == "os.environ" || instructionSecretValue.MatchString(l) {
			continue
		}
		return true
	}
	return false
}

// instructionStored reports whether a secret value is read from a local
// path: "API key from the .env file", "credentials in ~/.config". A
// person, product, URL, or UI ("from 1Password", "in Notion", "at
// https://...") is where a credential is obtained, not a file the agent
// opens.
func instructionStored(tail string) bool {
	word, rest := firstInstructionWord(tail)
	if !instructionStoredPrep.MatchString(word) {
		return false
	}
	for {
		word, rest = firstInstructionWord(rest)
		if !instructionGapDet.MatchString(word) {
			break
		}
	}
	word = strings.TrimLeft(word, "`'\"")
	if instructionEnvTemplate.MatchString(word) {
		return false
	}
	return !strings.Contains(word, "://") && instructionLocalPath.MatchString(word)
}

// instructionAcquire reports verbs that as often mean obtaining a
// credential or opening a page ("Get an API key at ...", "Access the
// secret manager UI") as reading one. Their object must say where the
// secret is stored.
func instructionAcquire(verb string) bool {
	switch strings.Join(strings.Fields(verb), " ") {
	case "get", "retrieve", "access", "look up":
		return true
	default:
		return false
	}
}

// instructionEchoSecret matches echo of a secret value. echo prints its
// operands: a quoted literal, a path such as .env, or output redirected
// with > is written, not read.
func instructionEchoSecret(clause string) bool {
	trimmed := strings.TrimLeft(clause, " \t")
	if strings.HasPrefix(trimmed, `"`) || strings.HasPrefix(trimmed, "'") || strings.HasPrefix(trimmed, "`") {
		return false
	}
	if instructionRedirect.MatchString(clause) {
		return false
	}
	h := instructionHead(clause)
	return h.at && !h.blocked && h.value
}

// instructionGapTarget matches a secret that is the direct object, or
// that follows at most instructionWordGap words inside the same noun
// phrase. A negation, a clause word, a documentation word, or a token
// that is not a plain word stops the walk. A preposition after a content
// word also stops, so a later secret is not picked up out of a new phrase.
// Once a word other than a determiner is skipped, or for an acquire
// verb, a secret value must be followed by a local path.
func instructionGapTarget(text string, skipped int, widened, strict bool) bool {
	h := instructionHead(text)
	if h.blocked {
		return false
	}
	if h.at {
		// "Source credentials for the run come from the connection"
		// names an origin. A direct-object secret value with no local
		// path in that tail is not a request to read a file.
		if h.value && instructionComesFrom.MatchString(h.tail) && !instructionHasLocalPath(h.tail) {
			return false
		}
		return !h.value || (!widened && !strict) || instructionStored(h.tail)
	}
	if skipped >= instructionWordGap {
		return false
	}
	word, rest := firstInstructionWord(text)
	if word == "" {
		return false
	}
	switch {
	case instructionGapStop.MatchString(word) || instructionModifier.MatchString(word):
		return false
	case instructionGapDet.MatchString(word):
		return instructionGapTarget(rest, skipped+1, widened, strict)
	case instructionGapPrep.MatchString(word):
		if widened {
			return false
		}
		return instructionGapTarget(rest, skipped+1, true, strict)
	case instructionGapPlain.MatchString(word):
		return instructionGapTarget(rest, skipped+1, true, strict)
	default:
		return false
	}
}

func firstInstructionWord(text string) (string, string) {
	text = strings.TrimLeft(text, " \t")
	if text == "" {
		return "", ""
	}
	i := 0
	for i < len(text) && text[i] != ' ' && text[i] != '\t' {
		i++
	}
	return text[:i], text[i:]
}

// curlNoiseFlags are the Authorization and user flags the rule ignores.
// Longer flags come first so --header is not read as -H.
var curlNoiseFlags = []string{"--oauth2-bearer", "--header", "--user", "-H", "-u"}

// stripInstructionCurlNoise drops auth-header and user-flag values, then
// plain http(s) URLs. A URL that embeds $( or a backtick is kept.
func stripInstructionCurlNoise(clause string) string {
	return instructionURLNoise.ReplaceAllString(stripCurlFlags(clause), " ")
}

func stripCurlFlags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		flag, ok := curlFlagAt(s, i)
		if !ok {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteByte(' ')
		i = consumeCurlValue(s, i+len(flag), flag)
	}
	return b.String()
}

func curlFlagAt(s string, i int) (string, bool) {
	if i > 0 && s[i-1] != ' ' && s[i-1] != '\t' {
		return "", false
	}
	for _, flag := range curlNoiseFlags {
		if len(s) < i+len(flag) || !strings.EqualFold(s[i:i+len(flag)], flag) {
			continue
		}
		end := i + len(flag)
		if end == len(s) {
			return flag, true
		}
		switch s[end] {
		case ' ', '\t', '=', '"', '\'':
			return flag, true
		}
	}
	return "", false
}

// consumeCurlValue reads the value of a curl auth flag. A quoted value is
// one string. An unquoted header that contains ":" continues through the
// following words, so "Authorization: Bearer $TOKEN" is one value. The
// walk stops at the next flag, at an http(s) URL (which the URL pass
// still has to see), and at a shell operator or a command substitution
// that follows the value, so the next command stays in the clause.
func consumeCurlValue(s string, i int, flag string) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i < len(s) && s[i] == '=' {
		i++
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
	}
	if i >= len(s) {
		return i
	}
	if s[i] == '"' || s[i] == '\'' {
		if headerFlag(flag) && i+1 < len(s) && s[i+1] == '@' {
			return i
		}
		return skipQuoted(s, i)
	}
	start := i
	if !headerFlag(flag) {
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		return i
	}
	end, stopped := consumeHeaderWord(s, start)
	if !stopped && strings.Contains(s[start:end], ":") {
		return consumeHeaderTail(s, end)
	}
	return end
}

// consumeHeaderWord reads the first word of an unquoted header value, such
// as Authorization:$TOKEN or Authorization:"Bearer $TOKEN". A word that
// starts with '@' names a file curl reads the headers from, so it is not
// consumed. A shell operator or a substitution ends the word at that byte
// (stopped is set), so Authorization:$(cat .env) keeps $(cat .env) in the
// clause. A double-quoted part is one string unless it holds $( or a
// backtick. A single-quoted part is a literal and is skipped.
func consumeHeaderWord(s string, start int) (end int, stopped bool) {
	if start < len(s) && s[start] == '@' {
		return start, true
	}
	i := start
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		switch {
		case headerStop(s, i):
			return i, true
		case s[i] == '\'':
			i = skipQuoted(s, i)
		case s[i] == '"':
			next := skipQuoted(s, i)
			if quotedSubstitution(s[i:next]) {
				return i, true
			}
			i = next
		default:
			i++
		}
	}
	return i, false
}

// skipQuoted returns the index after the quote that closes the one at i,
// or len(s) when it is not closed.
func skipQuoted(s string, i int) int {
	q := s[i]
	i++
	for i < len(s) && s[i] != q {
		i++
	}
	if i < len(s) {
		i++
	}
	return i
}

func quotedSubstitution(part string) bool {
	return strings.Contains(part, "$(") || strings.Contains(part, "`")
}

func consumeHeaderTail(s string, i int) int {
	for {
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j >= len(s) {
			return j
		}
		if s[j] == '-' || curlURLToken(s[j:]) {
			return i
		}
		for k := j; k < len(s) && s[k] != ' ' && s[k] != '\t'; k++ {
			if !headerStop(s, k) {
				continue
			}
			if k == j {
				return i
			}
			return k
		}
		i = j
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
	}
}

// headerStop reports a shell control operator or a following command
// substitution at k. '&' covers a background command and '&&'. '|'
// covers '|', '||', and '|&'. '<' covers an input redirect (< .env,
// 0<.env, <<<) and '<('. '$(' ends the value only when it starts a
// substitution, so '$TOKEN' stays inside it. '.' ends a clause only when
// a space or the end of the string follows it, so '.env' is not a stop.
func headerStop(s string, k int) bool {
	switch s[k] {
	case '&', '|', ';', '\n', '`', '<':
		return true
	case '$':
		return k+1 < len(s) && s[k+1] == '('
	case '.':
		return k+1 == len(s) || s[k+1] == ' ' || s[k+1] == '\t' || s[k+1] == '\n'
	default:
		return false
	}
}

// instructionSubstReadsPath reports a $(...) or backtick substitution in a
// curl or wget span that names a local path, such as $(cat ./token.txt) or
// `cat ~/.config/gh/hosts.yml`. The output of that command is sent, so the
// path counts even when it is not one of the named secret targets.
func instructionSubstReadsPath(span string) bool {
	for i := 0; i < len(span); i++ {
		var inner string
		switch {
		case span[i] == '$' && i+1 < len(span) && span[i+1] == '(':
			end := closingParen(span, i+2)
			inner, i = span[i+2:end], end
		case span[i] == '`':
			end := strings.IndexByte(span[i+1:], '`')
			if end < 0 {
				end = len(span) - i - 1
			}
			inner, i = span[i+1:i+1+end], i+1+end
		default:
			continue
		}
		for _, word := range strings.Fields(inner) {
			word = strings.TrimLeft(word, "`'\"<(")
			word = strings.TrimRight(word, "`'\");")
			if !strings.Contains(word, "://") && instructionLocalPath.MatchString(word) {
				return true
			}
		}
	}
	return false
}

// closingParen returns the index of the ')' that closes a '(' opened just
// before i, or len(s) when it is not closed.
func closingParen(s string, i int) int {
	depth := 1
	for ; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

func curlURLToken(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://")
}

func headerFlag(flag string) bool {
	return strings.EqualFold(flag, "-H") || strings.EqualFold(flag, "--header")
}

// instructionHasLocalPath reports a storage path anywhere in tail, so an
// origin sentence that also names .env or ~/.aws stays a read.
func instructionHasLocalPath(tail string) bool {
	rest := tail
	for {
		word, next := firstInstructionWord(rest)
		if word == "" {
			return false
		}
		// Trim quotes, opening parentheses and brackets, and trailing
		// punctuation. strings.Trim would also drop the leading dot,
		// and .env would no longer match.
		word = strings.TrimLeft(word, "`'\"([{")
		word = strings.TrimRight(word, "`'\".,);]}")
		if instructionLocalPath.MatchString(word) {
			return true
		}
		rest = next
	}
}

func instructionClause(text string) string {
	if end := instructionClauseEnd.FindStringIndex(text); end != nil {
		return text[:end[0]]
	}
	return text
}

// instructionCurlSpan is the curl or wget command plus the commands a
// shell separator attaches to it. A prose period still ends it.
func instructionCurlSpan(text string) string {
	if end := instructionProseEnd.FindStringIndex(text); end != nil {
		return text[:end[0]]
	}
	return text
}

func instructionMatchLineItem(item model.ConfigItem) int {
	raw := item.Payload
	if len(item.Detect) > 0 {
		raw = []byte(item.Detect)
	}
	var payload map[string]any
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&payload); err != nil {
		return 0
	}
	content, _ := payload["content"].(string)
	return instructionMatchLine(content)
}
