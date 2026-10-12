package prompt

import (
	"fmt"
	"strings"

	"github.com/kasuganosoras/thefuckplus/internal/context"
)

const responseFormat = `Return ONLY a JSON array of objects (no markdown, no code fences), each like:
{"command":"...","destructive":false,"warning":""}
Rules for each object:
- command: a single runnable shell command
- destructive: true if the command can delete/overwrite data, format disks, change critical system/security settings, force-push/reset git history, drop databases, flush firewalls, or pipe remote scripts to a shell
- warning: short reason when destructive is true; otherwise empty string
Order by likelihood. Return up to the requested number of candidates.`

const systemFix = `You are a shell command repair assistant for zsh on Linux/macOS.
Given a failed or incorrect command and context, suggest corrected shell commands.
` + responseFormat + `
Prefer non-destructive fixes. Only suggest destructive commands when clearly required to match the user's original intent.`

const systemGenerate = `You are a shell command generation assistant for zsh on Linux/macOS.
Convert the user's natural-language intent into runnable shell commands.
` + responseFormat + `
Prefer portable, widely available tools; modern alternatives may be extra candidates.
Do not invent interactive TUI workflows unless clearly requested.`

func FixMessages(ctx context.RequestContext, maxCandidates int) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "OS: %s\nShell: %s\nCWD: %s\n", ctx.OS, ctx.Shell, ctx.CWD)
	fmt.Fprintf(&b, "Previous command: %s\n", ctx.Command)
	if ctx.ExitCode != 0 {
		fmt.Fprintf(&b, "Exit code: %d\n", ctx.ExitCode)
	}
	fmt.Fprintf(&b, "Return up to %d candidates as a JSON array of objects.\n", maxCandidates)
	return systemFix, b.String()
}

func GenerateMessages(ctx context.RequestContext, maxCandidates int) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "OS: %s\nShell: %s\nCWD: %s\n", ctx.OS, ctx.Shell, ctx.CWD)
	fmt.Fprintf(&b, "Intent: %s\n", ctx.Query)
	fmt.Fprintf(&b, "Return up to %d candidates as a JSON array of objects.\n", maxCandidates)
	return systemGenerate, b.String()
}
