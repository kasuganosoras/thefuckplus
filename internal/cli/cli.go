package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kasuganosoras/thefuckplus/internal/agent"
	"github.com/kasuganosoras/thefuckplus/internal/config"
	appctx "github.com/kasuganosoras/thefuckplus/internal/context"
	"github.com/kasuganosoras/thefuckplus/internal/llm"
)

const Version = "0.1.0"

type Result struct {
	Candidates  []llm.Candidate `json:"candidates"`
	Mode        string          `json:"mode"`
	ExecuteMode string          `json:"execute_mode"`
}

func Run(args []string) int {
	if len(args) < 1 {
		printUsage()
		return 2
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println(Version)
		return 0
	case "configure", "config":
		_, err := config.ConfigureInteractive()
		if err != nil {
			fmt.Fprintf(os.Stderr, "configure failed: %v\n", err)
			return 1
		}
		return 0
	case "fix":
		return runFix(args[1:])
	case "generate":
		return runGenerate(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  thefuckplus fix [--command CMD] [--exit N] [--output TEXT|--output-file PATH]
  thefuckplus generate --query "natural language" [--output TEXT|--output-file PATH]
  thefuckplus configure
  thefuckplus version`)
}

func loadConfigOrHint() (config.Config, int) {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			fmt.Fprintln(os.Stderr, "need_config: run `thefuckplus configure` first")
			return config.Config{}, 3
		}
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return config.Config{}, 1
	}
	return cfg, 0
}

func runFix(args []string) int {
	cmd, exitCode, output, err := parseFixFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	cfg, code := loadConfigOrHint()
	if code != 0 {
		return code
	}
	if strings.TrimSpace(cmd) == "" {
		fmt.Fprintln(os.Stderr, "no previous command provided")
		return 1
	}

	ctxInfo := appctx.CollectWithOutput(cmd, exitCode, "", output)
	return completeAndPrint(cfg, "fix", ctxInfo)
}

func runGenerate(args []string) int {
	query, output, err := parseGenerateFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	cfg, code := loadConfigOrHint()
	if code != 0 {
		return code
	}
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "query is required")
		return 2
	}

	ctxInfo := appctx.CollectWithOutput("", 0, query, output)
	return completeAndPrint(cfg, "generate", ctxInfo)
}

func completeAndPrint(cfg config.Config, mode string, info appctx.RequestContext) int {
	candidates, err := agent.Run(context.Background(), cfg, mode, info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm: %v\n", err)
		return 1
	}
	out := Result{
		Candidates:  candidates,
		Mode:        mode,
		ExecuteMode: cfg.ExecuteMode,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}

func parseFixFlags(args []string) (command string, exitCode int, output string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--command", "-c":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--command requires a value")
			}
			i++
			command = args[i]
		case "--exit", "-e":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--exit requires a value")
			}
			i++
			n, e := strconv.Atoi(args[i])
			if e != nil {
				return "", 0, "", fmt.Errorf("invalid --exit: %w", e)
			}
			exitCode = n
		case "--output", "-o":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--output requires a value")
			}
			i++
			output = args[i]
		case "--output-file":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--output-file requires a value")
			}
			i++
			data, e := os.ReadFile(args[i])
			if e != nil {
				return "", 0, "", fmt.Errorf("read --output-file: %w", e)
			}
			output = string(data)
		default:
			return "", 0, "", fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	return command, exitCode, output, nil
}

func parseGenerateFlags(args []string) (query string, output string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--query", "-q":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--query requires a value")
			}
			i++
			query = args[i]
		case "--output", "-o":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--output requires a value")
			}
			i++
			output = args[i]
		case "--output-file":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--output-file requires a value")
			}
			i++
			data, e := os.ReadFile(args[i])
			if e != nil {
				return "", "", fmt.Errorf("read --output-file: %w", e)
			}
			output = string(data)
		default:
			// Allow trailing words as query for convenience.
			if query == "" {
				query = strings.Join(args[i:], " ")
				return query, output, nil
			}
			return "", "", fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	return query, output, nil
}
