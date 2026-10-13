package prompt

import (
	"strings"
	"testing"

	"github.com/kasuganosoras/thefuckplus/internal/context"
)

func TestFixMessagesWrapsOutput(t *testing.T) {
	sys, user := FixMessages(context.RequestContext{
		Command:  "npm run build",
		ExitCode: 1,
		CWD:      "/tmp/proj",
		OS:       "linux/amd64",
		Shell:    "zsh",
		Output:   "npm ERR! Missing script: \"build\"",
	}, 3, true)
	if !strings.Contains(sys, "UNTRUSTED DATA") {
		t.Fatalf("missing security boundary: %s", sys)
	}
	if !strings.Contains(sys, "read_file") {
		t.Fatalf("missing agent tools hint: %s", sys)
	}
	if !strings.Contains(user, "<untrusted_command_output>") {
		t.Fatalf("missing output wrapper: %s", user)
	}
	if !strings.Contains(user, "Missing script") {
		t.Fatalf("missing output body: %s", user)
	}
}

func TestGenerateMessagesNoAgent(t *testing.T) {
	sys, _ := GenerateMessages(context.RequestContext{
		Query: "list files",
		CWD:   "/tmp",
		OS:    "linux/amd64",
		Shell: "zsh",
	}, 3, false)
	if strings.Contains(sys, "read_file and list_dir") {
		t.Fatalf("agent hint should be off: %s", sys)
	}
	if !strings.Contains(sys, "SECURITY BOUNDARY") {
		t.Fatalf("security boundary should remain: %s", sys)
	}
}
