# thefuckplus

AI-powered command correction and generation for ZSH — inspired by [thefuck](https://github.com/nvbn/thefuck), driven by LLMs (OpenAI, DeepSeek, or any OpenAI-compatible API).

Type `fuck` to fix the last command, or `fuck <natural language>` to generate one. Cycle candidates with ↑ / ↓, edit with Tab, and get warned when a suggestion looks destructive.

![Demo](https://i.imgur.com/dGkK5Bm.gif)

## Features

| Input / key | Behavior |
|-------------|----------|
| `fuck` | Suggest fixes for the previous command (uses exit code + context) |
| `fuck <intent>` | Generate commands from natural language, e.g. `fuck find files larger than 100mb` |
| `↑` / `↓` | Cycle candidates; expands the full list on first arrow press |
| `Enter` | Keep in the buffer (`confirm`) or run immediately (`execute`) — configurable |
| `Tab` | Always insert into the buffer for manual editing (even in `execute` mode) |
| `Esc` / `Ctrl+C` / `Ctrl+G` | Cancel selection |
| `Esc Esc` | Quick-fix the previous command (disable with `THEFUCKPLUS_ESC_ESC=0`) |

**Agent (file reading):** when enabled (default), the model may call `read_file` / `list_dir` under your current working directory — e.g. read `package.json` after `npm run build` fails because the script is actually `build:h5`.

**Safety:** candidates are scored for destructive risk (model + local heuristics). Risky ones show a bright-yellow `⚠` hint (dim yellow when unselected). Sensitive files (`.env`, keys, credentials, …) are blocked unless you opt in.

**First run:** if AI is not configured, `thefuckplus configure` launches automatically.

## Install

### Quick install (clone + script)

```bash
git clone https://github.com/kasuganosoras/thefuckplus.git
cd thefuckplus
./scripts/install.sh
source ~/.zshrc
thefuckplus configure
```

The installer will:

1. Build the binary into `~/.local/bin`
2. Copy the ZSH plugin to `~/.local/share/thefuckplus`
3. Idempotently update `~/.zshrc` (`PATH` + `source`)

### oh-my-zsh

```bash
go install github.com/kasuganosoras/thefuckplus/cmd/thefuckplus@latest

git clone https://github.com/kasuganosoras/thefuckplus.git \
  ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/thefuckplus
```

Enable in `~/.zshrc`:

```zsh
plugins=(... thefuckplus)
```

Ensure `~/.local/bin` (or your `GOBIN`) is on `PATH`.

### Manual

```bash
git clone https://github.com/kasuganosoras/thefuckplus.git
cd thefuckplus
go build -o ~/.local/bin/thefuckplus ./cmd/thefuckplus
echo 'source ~/path/to/thefuckplus/thefuckplus.plugin.zsh' >> ~/.zshrc
```

## Configuration

```bash
thefuckplus configure
```

Config file: `~/.config/thefuckplus/config.yaml`

```yaml
provider: openai          # openai | deepseek | custom
api_key: "sk-..."
base_url: "https://api.openai.com/v1"
model: "gpt-4o-mini"
execute_mode: confirm     # confirm | execute
max_candidates: 5
timeout_sec: 30
agent:
  enabled: true
  max_rounds: 4
  max_file_bytes: 32768
  max_list_entries: 100
  allow_sensitive: false  # set true to allow .env / keys (still sandboxed to CWD)
  extra_allow: []         # e.g. [".env.example"]
  extra_deny: []          # extra deny globs (always applied)
```

| `execute_mode` | Enter |
|----------------|--------|
| `confirm` (default) | Leave the command in the buffer so you can review/edit |
| `execute` | Run the selected candidate immediately; **Tab** still only inserts |

### Agent security

- **Sandbox:** tools only access paths under the process CWD (symlink escapes denied).
- **Default deny:** `.env` / `.env.*`, `*.pem` / `*.key`, SSH key names, `.npmrc`, `.pypirc`, `credentials.json`, `*secret*`, `.aws/**`, `.gnupg/**`, `.ssh/**`, `config.yaml`, and similar.
- **`extra_allow`:** override default deny for specific relative globs (e.g. `.env.example`).
- **`extra_deny`:** always wins over allow / `allow_sensitive`.
- **Prompt injection:** file and command output are wrapped as untrusted data; the system prompt forbids following instructions inside them and forbids putting secrets into candidate commands.

Requires an OpenAI-compatible endpoint that supports `tools` / `tool_calls`. If the endpoint rejects tools, thefuckplus falls back to single-shot completion (no file reads).

Presets:

- **OpenAI** — `https://api.openai.com/v1`
- **DeepSeek** — `https://api.deepseek.com/v1` + `deepseek-chat`
- **Custom** — any OpenAI-compatible endpoint (Ollama, local gateways, …); API key may be empty

## CLI

```bash
thefuckplus fix --command "npm run build" --exit 1
thefuckplus fix --command "npm run build" --exit 1 --output 'Missing script: "build"'
thefuckplus fix --command "npm run build" --exit 1 --output-file /tmp/err.txt
thefuckplus generate --query "find files larger than 100mb"
thefuckplus configure
thefuckplus version
```

Successful stdout (JSON):

```json
{
  "candidates": [
    {
      "command": "find ~ -type f -size +100M",
      "destructive": false,
      "warning": ""
    }
  ],
  "mode": "generate",
  "execute_mode": "confirm"
}
```

If unconfigured: exit code `3`, stderr contains `need_config`.

## Environment variables

| Variable | Description |
|----------|-------------|
| `THEFUCKPLUS_BIN` | Binary name/path (default: `thefuckplus`) |
| `THEFUCKPLUS_ESC_ESC` | Set to `0` to disable Esc Esc quick-fix |

### Optional command output

Interactive ZSH cannot safely tee every command without breaking the TTY. The plugin exposes `_THEFUCKPLUS_LAST_OUTPUT`; if you set it (e.g. from a wrapper) before invoking `fuck`, it is passed as `--output`. File reading usually works without it.

## Requirements

- ZSH
- Go 1.22+ (to build)
- Optional: `jq` or `python3` for JSON parsing in the plugin (python3 is usually enough)
- Optional: [zsh-syntax-highlighting](https://github.com/zsh-users/zsh-syntax-highlighting) — used to color the selected command when available

## Development

```bash
go test ./...
go build -o /tmp/thefuckplus ./cmd/thefuckplus
```

```text
thefuckplus/
├── cmd/thefuckplus/     # entrypoint
├── internal/
│   ├── agent/           # multi-turn tool loop
│   ├── cli/             # subcommands
│   ├── config/          # YAML + interactive setup
│   ├── context/         # cwd / OS / command context
│   ├── fsguard/         # path sandbox + sensitive deny
│   ├── llm/             # OpenAI-compatible client
│   ├── prompt/          # fix / generate prompts
│   └── safety/          # destructive-command heuristics
├── shell/               # ZSH plugin
├── scripts/install.sh
└── thefuckplus.plugin.zsh
```

## License

MIT
