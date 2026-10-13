package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ConfigureInteractive() (Config, error) {
	in := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, "thefuckplus first-time setup")
	fmt.Fprintln(os.Stderr, "Configure your AI provider (OpenAI-compatible API).")
	fmt.Fprintln(os.Stderr)

	fmt.Fprintln(os.Stderr, "Provider:")
	fmt.Fprintln(os.Stderr, "  1) OpenAI")
	fmt.Fprintln(os.Stderr, "  2) DeepSeek")
	fmt.Fprintln(os.Stderr, "  3) Custom (OpenAI-compatible)")
	choice, err := prompt(in, "Choose [1-3]", "1")
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	switch strings.TrimSpace(choice) {
	case "2":
		cfg = Preset(ProviderDeepSeek)
	case "3":
		cfg = Preset(ProviderCustom)
	default:
		cfg = Preset(ProviderOpenAI)
	}

	baseURL, err := prompt(in, "Base URL", cfg.BaseURL)
	if err != nil {
		return Config{}, err
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	model, err := prompt(in, "Model", cfg.Model)
	if err != nil {
		return Config{}, err
	}
	cfg.Model = strings.TrimSpace(model)

	apiKeyDefault := ""
	if cfg.Provider == ProviderCustom {
		apiKeyDefault = "(leave empty if not required)"
	}
	apiKey, err := prompt(in, "API Key", apiKeyDefault)
	if err != nil {
		return Config{}, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "(leave empty if not required)" {
		apiKey = ""
	}
	cfg.APIKey = apiKey

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Execute mode:")
	fmt.Fprintln(os.Stderr, "  1) confirm — Enter keeps command in BUFFER for review (safer)")
	fmt.Fprintln(os.Stderr, "  2) execute — Enter runs the selected command immediately")
	fmt.Fprintln(os.Stderr, "     (Tab always inserts into BUFFER for editing)")
	mode, err := prompt(in, "Choose [1-2]", "1")
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(mode) == "2" {
		cfg.ExecuteMode = ExecuteExec
	} else {
		cfg.ExecuteMode = ExecuteConfirm
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Agent file reading: the AI may read files under your CWD")
	fmt.Fprintln(os.Stderr, "(e.g. package.json) to improve fixes. Sensitive paths (.env, keys)")
	fmt.Fprintln(os.Stderr, "are blocked by default.")
	sensitive, err := prompt(in, "Allow reading sensitive files? [y/N]", "N")
	if err != nil {
		return Config{}, err
	}
	cfg.Agent = DefaultAgent()
	switch strings.ToLower(strings.TrimSpace(sensitive)) {
	case "y", "yes":
		cfg.Agent.AllowSensitive = true
	default:
		cfg.Agent.AllowSensitive = false
	}

	if err := Save(cfg); err != nil {
		return Config{}, err
	}
	path, _ := Path()
	fmt.Fprintf(os.Stderr, "\nSaved config to %s\n", path)
	return cfg, nil
}

func prompt(in *bufio.Reader, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}
