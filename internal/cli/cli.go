package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kasuganosoras/thefuckplus/internal/config"
	appctx "github.com/kasuganosoras/thefuckplus/internal/context"
	"github.com/kasuganosoras/thefuckplus/internal/llm"
	"github.com/kasuganosoras/thefuckplus/internal/prompt"
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
  thefuckplus fix [--command CMD] [--exit N]
  thefuckplus generate --query "natural language"
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
	cmd, exitCode, err := parseFixFlags(args)
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

	ctxInfo := appctx.Collect(cmd, exitCode, "")
	system, user := prompt.FixMessages(ctxInfo, cfg.MaxCandidates)
	return completeAndPrint(cfg, "fix", system, user)
}

func runGenerate(args []string) int {
	query, err := parseGenerateFlags(args)
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

	ctxInfo := appctx.Collect("", 0, query)
	system, user := prompt.GenerateMessages(ctxInfo, cfg.MaxCandidates)
	return completeAndPrint(cfg, "generate", system, user)
}

func completeAndPrint(cfg config.Config, mode, system, user string) int {
	client := llm.New(cfg)
	candidates, err := client.Complete(context.Background(), system, user)
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

func parseFixFlags(args []string) (command string, exitCode int, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--command", "-c":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("--command requires a value")
			}
			i++
			command = args[i]
		case "--exit", "-e":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("--exit requires a value")
			}
			i++
			n, e := strconv.Atoi(args[i])
			if e != nil {
				return "", 0, fmt.Errorf("invalid --exit: %w", e)
			}
			exitCode = n
		default:
			return "", 0, fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	return command, exitCode, nil
}

func parseGenerateFlags(args []string) (string, error) {
	var query string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--query", "-q":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--query requires a value")
			}
			i++
			query = args[i]
		default:
			// Allow trailing words as query for convenience.
			if query == "" {
				query = strings.Join(args[i:], " ")
				return query, nil
			}
			return "", fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	return query, nil
}
