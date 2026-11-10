package shellcmd

import (
	"slices"
	"testing"
)

func TestCommands(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		want [][]string
	}{
		{"npx -y 'pkg@1'", [][]string{{"npx", "-y", "pkg@1"}}},
		{"cd srv && npx pkg@1; uvx b==2\nexec c", [][]string{{"cd", "srv"}, {"npx", "pkg@1"}, {"uvx", "b==2"}, {"exec", "c"}}},
		{"a | b || c & d", [][]string{{"a"}, {"b"}, {"c"}, {"d"}}},
		{"npx pkg@1 < in.txt", [][]string{{"npx", "pkg@1"}}},
		{"x=$(curl -s u); npx pkg@1", [][]string{{"npx", "pkg@1"}}},
		{"npx pkg@$(cat v)", nil},
		{"", nil},
	}
	for _, tc := range cases {
		got := Commands(tc.line)
		if !slices.EqualFunc(got, tc.want, slices.Equal[[]string]) {
			t.Errorf("Commands(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}
