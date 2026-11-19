package kit

import "testing"

func TestJSONLines(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
  "hooks": {
    "A": [
      {
        "hooks": [
          {"type": "command", "command": "one"}
        ]
      }
    ],
    "B/C": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "two"
          }
        ]
      }
    ],
    "A": [
      {"hooks": [{"url": "https://example.invalid/"}]}
    ]
  }
}`)
	got := JSONLines(raw)
	want := map[string]int{
		"":       1,
		"/hooks": 2,
		JSONPointer("hooks", "B/C", "0", "hooks", "0"):            13,
		JSONPointer("hooks", "B/C", "0", "hooks", "0", "command"): 15,
		"/hooks/B~1C/0/hooks/0/type":                              14,
		"/hooks/A":                                                20,
		"/hooks/A/0/hooks/0/url":                                  21,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("line of %q = %d, want %d", k, got[k], w)
		}
	}
	// The earlier duplicate "A" is replaced whole: its command is gone.
	if l, ok := got["/hooks/A/0/hooks/0/command"]; ok {
		t.Errorf("stale duplicate entry kept at line %d", l)
	}
	if got := JSONLines([]byte(`{"a": [1,`)); got["/a/0"] != 1 {
		t.Errorf("truncated document: /a/0 = %d, want 1", got["/a/0"])
	}
}
