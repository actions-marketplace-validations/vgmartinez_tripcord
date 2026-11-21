package rules

import (
	"regexp"
	"testing"
)

func TestFixturePath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"tests/fixtures/bad-plugin/.mcp.json", true},
		{"internal/agentconfig/testdata/repo/.claude/settings.json", true},
		{"test/.claude/settings.json", true},
		{"tests/e2e/.codex/config.toml", true},
		{"src/__fixtures__/.cursor/mcp.json", true},
		{"a/test-fixtures/.mcp.json", true},
		{"a/test_fixtures/.mcp.json", true},
		{".claude/settings.json", false},
		{".mcp.json", false},
		{"testing-guide/.claude/settings.json", false},
		{"contests/.mcp.json", false},
		{"latest/.mcp.json", false},
		{"Fixtures/.mcp.json", false},
		{"mytests/.mcp.json", false},
		{"docs/test", false},
		{"tests", false},
	} {
		if got := FixturePath(tc.path); got != tc.want {
			t.Errorf("FixturePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// FixturePath must agree with the directory clause TC-CFG-006 and the
// first TC-CFG-010 low clause write as a regular expression.
func TestFixturePathMatchesRuleClause(t *testing.T) {
	re := regexp.MustCompile(`(^|/)(fixtures|__fixtures__|testdata|test-fixtures|test_fixtures|tests|test)/`)
	for _, p := range []string{
		"tests/x.json", "a/test/b/c.json", "fixtures/.mcp.json", "a/testdata/.mcp.json",
		"x.json", "testing/x.json", "a/tests", "a/__fixtures__/b", "a/b/test_fixtures/c/d.toml",
		"contests/x", "test-fixtures/x", "Tests/x",
	} {
		if got, want := FixturePath(p), re.MatchString(p); got != want {
			t.Errorf("FixturePath(%q) = %v, regexp %v", p, got, want)
		}
	}
}
