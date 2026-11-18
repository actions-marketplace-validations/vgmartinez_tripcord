package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vgmartinez/tripcord/internal/exitcode"
	"github.com/vgmartinez/tripcord/internal/fpkey"
)

// codexTwoHooks has one handler per event; each finding must point at
// its own handler's command line (9 and 19), not the first one's.
const codexTwoHooks = `{
  "description": "sample hooks",
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/opt/sample/hooks/session-start.sh"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/opt/sample/hooks/session-stop.sh"
          }
        ]
      }
    ]
  }
}
`

func TestScanCodexHooksLineGolden(t *testing.T) {
	t.Setenv(fpkey.EnvVar, noCINote)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "hooks.json"), []byte(codexTwoHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"scan", "-v", root}, &stdout, &stderr); code != int(exitcode.OK) {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	assertGolden(t, "scan-codex-hooks-line.golden", stdout.String())
}
