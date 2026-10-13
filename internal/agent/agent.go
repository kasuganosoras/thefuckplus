package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kasuganosoras/thefuckplus/internal/config"
	appctx "github.com/kasuganosoras/thefuckplus/internal/context"
	"github.com/kasuganosoras/thefuckplus/internal/fsguard"
	"github.com/kasuganosoras/thefuckplus/internal/llm"
	"github.com/kasuganosoras/thefuckplus/internal/prompt"
)

var tools = []llm.Tool{
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "read_file",
			Description: "Read a text file under the current working directory. Use for project config (e.g. package.json) needed to fix or generate commands. Sensitive files may be denied.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative path under CWD"}},"required":["path"]}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "list_dir",
			Description: "List entries in a directory under the current working directory.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative directory path under CWD (default \".\")"}},"required":[]}`),
		},
	},
}

// Run executes fix/generate with optional multi-turn file tools.
func Run(ctx context.Context, cfg config.Config, mode string, info appctx.RequestContext) ([]llm.Candidate, error) {
	client := llm.New(cfg)

	var system, user string
	switch mode {
	case "generate":
		system, user = prompt.GenerateMessages(info, cfg.MaxCandidates, cfg.Agent.Enabled)
	default:
		system, user = prompt.FixMessages(info, cfg.MaxCandidates, cfg.Agent.Enabled)
	}

	if !cfg.Agent.Enabled {
		return client.Complete(ctx, system, user)
	}

	guard, err := fsguard.New(info.CWD, cfg.Agent)
	if err != nil {
		return nil, fmt.Errorf("fsguard: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}

	maxRounds := cfg.Agent.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 4
	}

	toolsUnsupported := false
	for round := 0; round < maxRounds; round++ {
		var res *llm.ChatResult
		var chatErr error
		if toolsUnsupported {
			res, chatErr = client.Chat(ctx, messages, nil)
		} else {
			res, chatErr = client.Chat(ctx, messages, tools)
			if chatErr != nil && isToolsUnsupported(chatErr, res) {
				toolsUnsupported = true
				// Fall back to single-shot without tools.
				return client.Complete(ctx, system, user)
			}
		}
		if chatErr != nil {
			return nil, chatErr
		}

		msg := res.Message
		if len(msg.ToolCalls) > 0 {
			// Append assistant message with tool_calls (content may be empty).
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   msg.Content,
				ToolCalls: msg.ToolCalls,
			})
			for _, tc := range msg.ToolCalls {
				result := executeTool(guard, tc)
				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Content:    result,
				})
			}
			continue
		}

		content := strings.TrimSpace(msg.Content)
		candidates := llm.ParseCandidates(content, cfg.MaxCandidates)
		if len(candidates) > 0 {
			return llm.EnrichSafety(candidates), nil
		}

		// No tool calls and no parseable candidates: ask once more to emit JSON,
		// or fail on last round.
		if round+1 >= maxRounds {
			return nil, fmt.Errorf("could not parse candidates from llm output: %q", truncate(content, 200))
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: msg.Content})
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: "Return ONLY the JSON array of candidate command objects now. Do not call tools unless strictly necessary.",
		})
	}

	return nil, fmt.Errorf("agent exceeded max rounds without candidates")
}

func executeTool(guard *fsguard.Guard, tc llm.ToolCall) string {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

	switch tc.Function.Name {
	case "read_file":
		if strings.TrimSpace(args.Path) == "" {
			return `{"ok":false,"error":"invalid_args","reason":"path_required"}`
		}
		return guard.ReadFile(args.Path)
	case "list_dir":
		return guard.ListDir(args.Path)
	default:
		return `{"ok":false,"error":"unknown_tool","reason":"` + tc.Function.Name + `"}`
	}
}

func isToolsUnsupported(err error, res *llm.ChatResult) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "tool") && (strings.Contains(msg, "not support") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "does not support") ||
		strings.Contains(msg, "unknown field")) {
		return true
	}
	if res != nil && (res.StatusCode == 400 || res.StatusCode == 422) {
		body := strings.ToLower(res.RawBody)
		if strings.Contains(body, "tools") || strings.Contains(body, "tool_choice") ||
			strings.Contains(body, "tool call") {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
