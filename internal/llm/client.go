package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kasuganosoras/thefuckplus/internal/config"
	"github.com/kasuganosoras/thefuckplus/internal/safety"
)

type Client struct {
	cfg    config.Config
	client *http.Client
}

// Candidate is one suggested command with optional destruction warning.
type Candidate struct {
	Command     string `json:"command"`
	Destructive bool   `json:"destructive"`
	Warning     string `json:"warning,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
		Finish  string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ChatResult is one assistant turn (content and/or tool calls).
type ChatResult struct {
	Message      Message
	FinishReason string
	RawBody      string
	StatusCode   int
}

func New(cfg config.Config) *Client {
	timeout := time.Duration(cfg.HTTPTimeout()) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Complete(ctx context.Context, system, user string) ([]Candidate, error) {
	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	res, err := c.Chat(ctx, messages, nil)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(res.Message.Content)
	candidates := ParseCandidates(content, c.cfg.MaxCandidates)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("could not parse candidates from llm output: %q", truncate(content, 200))
	}
	return enrichSafety(candidates), nil
}

// Chat sends a multi-turn chat request, optionally with tools.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResult, error) {
	reqBody := chatRequest{
		Model:       c.cfg.Model,
		Messages:    messages,
		Temperature: 0.2,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
		reqBody.ToolChoice = "auto"
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	raw := string(body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ChatResult{RawBody: raw, StatusCode: resp.StatusCode},
			fmt.Errorf("llm http %d: %s", resp.StatusCode, strings.TrimSpace(raw))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("llm error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	return &ChatResult{
		Message:      parsed.Choices[0].Message,
		FinishReason: parsed.Choices[0].Finish,
		RawBody:      raw,
		StatusCode:   resp.StatusCode,
	}, nil
}

var codeFence = regexp.MustCompile("(?s)```(?:json|shell|bash|zsh)?\\s*(.*?)```")

func enrichSafety(in []Candidate) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		a := safety.Merge(c.Command, c.Destructive, c.Warning)
		c.Destructive = a.Destructive
		c.Warning = a.Warning
		out = append(out, c)
	}
	return out
}

// EnrichSafety applies local destructive heuristics (exported for agent package).
func EnrichSafety(in []Candidate) []Candidate {
	return enrichSafety(in)
}

func ParseCandidates(content string, max int) []Candidate {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if m := codeFence.FindStringSubmatch(content); len(m) == 2 {
		content = strings.TrimSpace(m[1])
	}

	// Prefer a JSON array substring.
	if start := strings.Index(content, "["); start >= 0 {
		if end := strings.LastIndex(content, "]"); end > start {
			raw := content[start : end+1]

			var objs []Candidate
			if err := json.Unmarshal([]byte(raw), &objs); err == nil && len(objs) > 0 && objs[0].Command != "" {
				return normalizeCandidates(objs, max)
			}

			// Older/simple format: array of strings
			var arr []string
			if err := json.Unmarshal([]byte(raw), &arr); err == nil {
				return normalizeCandidates(fromStrings(arr), max)
			}

			// Mixed/partial objects with "cmd" alias
			var generic []map[string]any
			if err := json.Unmarshal([]byte(raw), &generic); err == nil {
				var converted []Candidate
				for _, g := range generic {
					cmd, _ := g["command"].(string)
					if cmd == "" {
						cmd, _ = g["cmd"].(string)
					}
					if cmd == "" {
						continue
					}
					dest, _ := g["destructive"].(bool)
					warn, _ := g["warning"].(string)
					converted = append(converted, Candidate{Command: cmd, Destructive: dest, Warning: warn})
				}
				if len(converted) > 0 {
					return normalizeCandidates(converted, max)
				}
			}
		}
	}

	// Fallback: line-based extraction.
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.Trim(line, "`")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, " ") && !looksLikeCommand(line) {
			continue
		}
		if looksLikeCommand(line) {
			lines = append(lines, line)
		}
	}
	return normalizeCandidates(fromStrings(lines), max)
}

func fromStrings(in []string) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, s := range in {
		out = append(out, Candidate{Command: s})
	}
	return out
}

func normalizeCandidates(in []Candidate, max int) []Candidate {
	seen := make(map[string]struct{})
	var out []Candidate
	for _, c := range in {
		c = unwrapNestedCandidate(c)
		c.Command = strings.TrimSpace(c.Command)
		if c.Command == "" {
			continue
		}
		if _, ok := seen[c.Command]; ok {
			continue
		}
		seen[c.Command] = struct{}{}
		out = append(out, c)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// unwrapNestedCandidate fixes models that put a JSON object string into "command".
func unwrapNestedCandidate(c Candidate) Candidate {
	cmd := strings.TrimSpace(c.Command)
	if !strings.HasPrefix(cmd, "{") {
		return c
	}
	var inner Candidate
	if err := json.Unmarshal([]byte(cmd), &inner); err != nil || strings.TrimSpace(inner.Command) == "" {
		return c
	}
	if !c.Destructive && inner.Destructive {
		c.Destructive = true
	}
	if c.Warning == "" {
		c.Warning = inner.Warning
	}
	c.Command = inner.Command
	return c
}

func looksLikeCommand(s string) bool {
	if s == "" {
		return false
	}
	first := strings.Fields(s)[0]
	if strings.ContainsAny(first, "。？！，") {
		return false
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{"here ", "the ", "you ", "i ", "this ", "please ", "sorry "} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
