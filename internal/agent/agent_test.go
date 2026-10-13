package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kasuganosoras/thefuckplus/internal/config"
	appctx "github.com/kasuganosoras/thefuckplus/internal/context"
)

func TestRunToolRoundTrip(t *testing.T) {
	root := t.TempDir()
	pkg := `{"scripts":{"build:h5":"vite build"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_1",
									"type": "function",
									"function": map[string]string{
										"name":      "read_file",
										"arguments": `{"path":"package.json"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": `[{"command":"npm run build:h5","destructive":false,"warning":""}]`,
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.BaseURL = srv.URL + "/v1"
	cfg.APIKey = "test"
	cfg.Provider = config.ProviderCustom
	cfg.Agent.Enabled = true
	cfg.Agent.MaxRounds = 4

	info := appctx.RequestContext{
		Command:  "npm run build",
		ExitCode: 1,
		CWD:      root,
		OS:       "linux/amd64",
		Shell:    "zsh",
	}

	cands, err := Run(context.Background(), cfg, "fix", info)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].Command != "npm run build:h5" {
		t.Fatalf("unexpected candidates: %#v", cands)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected tool round-trip, calls=%d", calls.Load())
	}
}

func TestRunDisabledFallsBack(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			t.Errorf("tools should not be sent when agent disabled")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": `[{"command":"ls","destructive":false,"warning":""}]`,
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.BaseURL = srv.URL + "/v1"
	cfg.APIKey = "test"
	cfg.Provider = config.ProviderCustom
	cfg.Agent.Enabled = false

	info := appctx.RequestContext{
		Command:  "lss",
		ExitCode: 127,
		CWD:      t.TempDir(),
		OS:       "linux/amd64",
		Shell:    "zsh",
	}
	cands, err := Run(context.Background(), cfg, "fix", info)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].Command != "ls" {
		t.Fatalf("unexpected: %#v", cands)
	}
}

func TestDenyEnvInAgentTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}

	var sawToolResult string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_env",
									"type": "function",
									"function": map[string]string{
										"name":      "read_file",
										"arguments": `{"path":".env"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "tool" {
				sawToolResult = m.Content
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": `[{"command":"echo ok","destructive":false,"warning":""}]`,
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.BaseURL = srv.URL + "/v1"
	cfg.APIKey = "test"
	cfg.Provider = config.ProviderCustom

	info := appctx.RequestContext{Command: "npm start", ExitCode: 1, CWD: root, OS: "linux/amd64", Shell: "zsh"}
	if _, err := Run(context.Background(), cfg, "fix", info); err != nil {
		t.Fatal(err)
	}
	if sawToolResult == "" {
		t.Fatal("expected tool result in second request")
	}
	if !strings.Contains(sawToolResult, "sensitive_path") {
		t.Fatalf("expected deny json, got %s", sawToolResult)
	}
	if strings.Contains(sawToolResult, "SECRET") {
		t.Fatal("must not leak .env content")
	}
}
