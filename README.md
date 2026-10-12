# thefuckplus

AI-powered command correction and generation for ZSH — inspired by [thefuck](https://github.com/nvbn/thefuck), driven by LLMs (OpenAI, DeepSeek, or any OpenAI-compatible API).

Type `fuck` to fix the last command, or `fuck <natural language>` to generate one. Cycle candidates with ↑ / ↓, edit with Tab, and get warned when a suggestion looks destructive.

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

**Safety:** candidates are scored for destructive risk (model + local heuristics). Risky ones show a bright-yellow `⚠` hint (dim yellow when unselected).

**First run:** if AI is not configured, `thefuckplus configure` launches automatically.

## Demo

```text
$ wg show
Unable to access interface wg0: Operation not permitted
$ fuck
# → sudo wg show   (↑↓ for more)

$ fuck find files larger than 100mb
# → find ~ -type f -size +100M
```

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
```

| `execute_mode` | Enter |
|----------------|--------|
| `confirm` (default) | Leave the command in the buffer so you can review/edit |
| `execute` | Run the selected candidate immediately; **Tab** still only inserts |

Presets:

- **OpenAI** — `https://api.openai.com/v1`
- **DeepSeek** — `https://api.deepseek.com/v1` + `deepseek-chat`
- **Custom** — any OpenAI-compatible endpoint (Ollama, local gateways, …); API key may be empty

## CLI

```bash
thefuckplus fix --command "git push" --exit 1
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
│   ├── cli/             # subcommands
│   ├── config/          # YAML + interactive setup
│   ├── context/         # cwd / OS / command context
│   ├── llm/             # OpenAI-compatible client
│   ├── prompt/          # fix / generate prompts
│   └── safety/          # destructive-command heuristics
├── shell/               # ZSH plugin
├── scripts/install.sh
└── thefuckplus.plugin.zsh
```

## License

MIT
