// Author: zhangweiwei
package opener

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestValidCwd(t *testing.T) {
	if !validCwd(t.TempDir()) {
		t.Error("temp dir should be valid")
	}
	if validCwd(`C:\definitely\not\exists\path`) {
		t.Error("missing path accepted")
	}
	if validCwd(`relative\path`) {
		t.Error("relative path accepted")
	}
	if validCwd("C:\\bad\"path") {
		t.Error("path with quote accepted")
	}
}

func TestPlanWithWindowsTerminal(t *testing.T) {
	o := &Opener{wt: `C:\wt.exe`, pwsh: `C:\pwsh.exe`}
	terminal, shell, args, err := o.plan(`D:\work\proj`, "3509b3c8-7e44-4373-b56c-6090484dc360")
	if err != nil {
		t.Fatal(err)
	}
	if terminal != "windows-terminal" || shell != "pwsh" {
		t.Errorf("terminal/shell = %q/%q", terminal, shell)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-w", "0", "nt", "-d", `D:\work\proj`, "-NoExit", "-EncodedCommand"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
	script := decodePowerShell(t, args[len(args)-1])
	for _, want := range []string{
		"CLAUDE_CODE_CHILD_SESSION=$null",
		"CLAUDE_CODE_FORCE_SESSION_PERSISTENCE='1'",
		"claude --resume 3509b3c8-7e44-4373-b56c-6090484dc360",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q: %s", want, script)
		}
	}
}

func decodePowerShell(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	return string(utf16.Decode(units))
}

func TestSanitizedEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "session-id")
	t.Setenv("CLAUDE_CODE_KEEP_ME", "1")

	joined := strings.Join(sanitizedEnv(), "\n")
	if strings.Contains(joined, "CLAUDE_CODE_CHILD_SESSION=") {
		t.Error("child session marker was not removed")
	}
	if strings.Contains(joined, "CLAUDE_CODE_SESSION_ID=") {
		t.Error("session id was not removed")
	}
	if !strings.Contains(joined, "CLAUDE_CODE_KEEP_ME=1") {
		t.Error("unrelated env var was removed")
	}
	if !strings.Contains(joined, "CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1") {
		t.Error("force persistence flag missing")
	}
}

func TestPlanFallbackToPowerShell(t *testing.T) {
	o := &Opener{wt: `C:\wt.exe`, powershell: `C:\powershell.exe`}
	_, shell, _, err := o.plan(`D:\work\proj`, "id")
	if err != nil {
		t.Fatal(err)
	}
	if shell != "powershell" {
		t.Errorf("shell = %q, want powershell", shell)
	}
}

func TestUUIDValidation(t *testing.T) {
	o := &Opener{claude: `C:\claude.cmd`}
	if _, err := o.Open(t.TempDir(), "not-a-uuid"); err != ErrInvalidID {
		t.Errorf("err = %v, want ErrInvalidID", err)
	}
}
