# thefuckplus — AI-powered command fix / generation for ZSH
# https://github.com/kasuganosoras/thefuckplus

typeset -g _THEFUCKPLUS_PLUGIN_DIR="${${(%):-%x}:A:h}"
typeset -g _THEFUCKPLUS_LAST_CMD=""
typeset -g _THEFUCKPLUS_LAST_EXIT=0
typeset -ga _thefuckplus_candidates
typeset -ga _thefuckplus_destructive
typeset -ga _thefuckplus_warnings
typeset -g _thefuckplus_index=1
typeset -g _thefuckplus_execute_mode="confirm"
typeset -g _thefuckplus_selecting=0
typeset -g _thefuckplus_show_list=0
typeset -g _thefuckplus_saved_buffer=""
typeset -g _thefuckplus_saved_cursor=0
typeset -g _thefuckplus_saved_postdisplay=""
typeset -g _thefuckplus_saved_keymap=""
typeset -g _thefuckplus_saved_keytimeout=""
typeset -ga _thefuckplus_saved_region_highlight
typeset -ga _THEFUCKPLUS_SPINNER_FRAMES
_THEFUCKPLUS_SPINNER_FRAMES=(⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏)
# region_highlight styles (ZLE escapes raw ANSI in POSTDISPLAY)
# Selected fallback when zsh-syntax-highlighting is absent: white
typeset -g _THEFUCKPLUS_HL_SELECTED="fg=15,bold"
typeset -g _THEFUCKPLUS_HL_MUTED="fg=245"
# Destructive: selected = bright yellow; unselected = dim yellow
typeset -g _THEFUCKPLUS_HL_WARN="fg=226,bold"
typeset -g _THEFUCKPLUS_HL_WARN_MUTED="fg=178"
# WARNING SIGN without VS16 (avoids <fe0f> rendering issues)
typeset -g _THEFUCKPLUS_WARN_ICON="⚠"

: ${THEFUCKPLUS_BIN:=thefuckplus}
: ${THEFUCKPLUS_ESC_ESC:=1}

_thefuckplus_preexec() {
  _THEFUCKPLUS_LAST_CMD="$1"
}

_thefuckplus_precmd() {
  _THEFUCKPLUS_LAST_EXIT=$?
}

autoload -Uz add-zsh-hook 2>/dev/null
if (( $+functions[add-zsh-hook] )); then
  add-zsh-hook preexec _thefuckplus_preexec
  add-zsh-hook precmd _thefuckplus_precmd
else
  preexec_functions+=(_thefuckplus_preexec)
  precmd_functions+=(_thefuckplus_precmd)
fi

_thefuckplus_bin() {
  if (( $+commands[$THEFUCKPLUS_BIN] )); then
    command "$THEFUCKPLUS_BIN" "$@"
    return $?
  fi
  local candidates=(
    "${_THEFUCKPLUS_PLUGIN_DIR}/../bin/thefuckplus"
    "${_THEFUCKPLUS_PLUGIN_DIR}/../thefuckplus"
    "${HOME}/.local/bin/thefuckplus"
  )
  local p
  for p in $candidates; do
    if [[ -x "$p" ]]; then
      "$p" "$@"
      return $?
    fi
  done
  print -u2 "thefuckplus: binary not found in PATH (install or set THEFUCKPLUS_BIN)"
  return 127
}

# Parse JSON result into globals. Prefers jq, then python3, then a minimal extractor.
# Supports candidates as strings or {command,destructive,warning} objects.
_thefuckplus_parse_result() {
  local json="$1"
  _thefuckplus_candidates=()
  _thefuckplus_destructive=()
  _thefuckplus_warnings=()
  _thefuckplus_execute_mode="confirm"
  _thefuckplus_index=1

  _thefuckplus_ingest_rows() {
    # $1 = execute_mode line + TSV rows: command<TAB>0|1<TAB>warning
    local -a lines
    local row cmd dest warn rest
    lines=("${(@f)1}")
    (( ${#lines} < 2 )) && return 1
    _thefuckplus_execute_mode="${lines[1]}"
    for row in "${lines[@]:1}"; do
      cmd="${row%%$'\t'*}"
      rest="${row#*$'\t'}"
      dest="${rest%%$'\t'*}"
      warn="${rest#*$'\t'}"
      [[ "$warn" == "$rest" ]] && warn=""
      [[ -n "$cmd" ]] || continue
      _thefuckplus_candidates+=("$cmd")
      _thefuckplus_destructive+=("${dest:-0}")
      _thefuckplus_warnings+=("$warn")
    done
    (( ${#_thefuckplus_candidates} > 0 ))
  }

  if (( $+commands[jq] )); then
    local parsed
    parsed=$(print -r -- "$json" | jq -r '
      (.execute_mode // "confirm"),
      (.candidates[]? |
        if type=="object" then
          ((.command // "") | gsub("\n";" ") | gsub("\t";" "))
          + "\t" + (if .destructive then "1" else "0" end)
          + "\t" + ((.warning // "") | gsub("\n";" ") | gsub("\t";" "))
        else
          (tostring | gsub("\n";" ") | gsub("\t";" ")) + "\t0\t"
        end
      )' 2>/dev/null) || return 1
    _thefuckplus_ingest_rows "$parsed" && return 0
  fi

  if (( $+commands[python3] )); then
    local parsed
    parsed=$(THEFUCKPLUS_JSON="$json" python3 -c 'import json,os
data=json.loads(os.environ["THEFUCKPLUS_JSON"])
print(data.get("execute_mode") or "confirm")
for c in data.get("candidates") or []:
    if isinstance(c, dict):
        cmd=str(c.get("command") or "").replace("\n"," ").replace("\t"," ")
        dest="1" if c.get("destructive") else "0"
        warn=str(c.get("warning") or "").replace("\n"," ").replace("\t"," ")
    else:
        cmd=str(c).replace("\n"," ").replace("\t"," ")
        dest="0"
        warn=""
    if not cmd:
        continue
    print(cmd + "\t" + dest + "\t" + warn)' 2>/dev/null) || return 1
    _thefuckplus_ingest_rows "$parsed" && return 0
  fi

  # Minimal fallback: collect JSON string values, skip known keys/enums.
  local rest="$json" item
  while [[ "$rest" =~ \"([^\"]+)\" ]]; do
    item="${match[1]}"
    rest="${rest#*\"${item}\"}"
    case "$item" in
      candidates|execute_mode|mode|confirm|execute|fix|generate|command|destructive|warning|true|false) ;;
      *)
        _thefuckplus_candidates+=("$item")
        _thefuckplus_destructive+=("0")
        _thefuckplus_warnings+=("")
        ;;
    esac
  done
  if [[ "$json" == *'"execute_mode":"execute"'* || "$json" == *'"execute_mode": "execute"'* ]]; then
    _thefuckplus_execute_mode="execute"
  fi
  (( ${#_thefuckplus_candidates} > 0 ))
}

_thefuckplus_is_destructive() {
  local i="${1:-$_thefuckplus_index}"
  [[ "${_thefuckplus_destructive[i]}" == "1" ]]
}

# Dedicated keymap so Ctrl+C / Esc never leave half-overridden main bindings
# (that caused "ghost" select UI and stray "OB"/"0B" in BUFFER).
_thefuckplus_ensure_select_keymap() {
  bindkey -N thefuckplus-select main 2>/dev/null || bindkey -N thefuckplus-select emacs
  bindkey -M thefuckplus-select '^[[A' _thefuckplus_up
  bindkey -M thefuckplus-select '^[OA' _thefuckplus_up
  bindkey -M thefuckplus-select '^[[B' _thefuckplus_down
  bindkey -M thefuckplus-select '^[OB' _thefuckplus_down
  bindkey -M thefuckplus-select '^I' _thefuckplus_tab
  bindkey -M thefuckplus-select '^M' _thefuckplus_enter
  bindkey -M thefuckplus-select '^J' _thefuckplus_enter
  bindkey -M thefuckplus-select '^G' _thefuckplus_cancel
  bindkey -M thefuckplus-select '^C' _thefuckplus_cancel
  # Esc alone cancels; arrows still win as longer sequences within KEYTIMEOUT.
  bindkey -M thefuckplus-select '^[' _thefuckplus_cancel
  bindkey -M thefuckplus-select '\e\e' _thefuckplus_cancel
}

_thefuckplus_enter_select_keymap() {
  _thefuckplus_saved_keymap="${KEYMAP:-main}"
  _thefuckplus_saved_keytimeout="$KEYTIMEOUT"
  # Shorter timeout so Esc cancels quickly while ↑↓ sequences still resolve.
  KEYTIMEOUT=10
  _thefuckplus_ensure_select_keymap
  zle -K thefuckplus-select
}

_thefuckplus_leave_select_keymap() {
  local km="${_thefuckplus_saved_keymap:-main}"
  [[ -n "$_thefuckplus_saved_keytimeout" ]] && KEYTIMEOUT=$_thefuckplus_saved_keytimeout
  _thefuckplus_saved_keytimeout=""
  zle -K "$km" 2>/dev/null || zle -K main
  _thefuckplus_saved_keymap=""
}

# True when zsh-syntax-highlighting (or a compatible _zsh_highlight) is available.
_thefuckplus_has_syntax_highlight() {
  (( $+functions[_zsh_highlight] ))
}

# Build plain POSTDISPLAY + region_highlight (BUFFER+POSTDISPLAY coords).
# BUFFER: prefer zsh-syntax-highlighting; else white. Never put raw ANSI in POSTDISPLAY.
_thefuckplus_apply_highlights() {
  (( _thefuckplus_selecting )) || return

  local n=${#_thefuckplus_candidates}
  local -a lines
  local i icon="$_THEFUCKPLUS_WARN_ICON"
  local hint="Use ↑ and ↓ to switch command (${_thefuckplus_index}/${n})"
  local cur_warn=""
  if _thefuckplus_is_destructive; then
    cur_warn="${_thefuckplus_warnings[$_thefuckplus_index]}"
    [[ -n "$cur_warn" ]] || cur_warn="destructive"
    hint="${icon} ${hint} — ${cur_warn}"
  fi
  lines+=("$hint")

  if (( _thefuckplus_show_list )); then
    for i in {1..$n}; do
      if (( i == _thefuckplus_index )); then
        if _thefuckplus_is_destructive "$i"; then
          lines+=("${icon} ❯ ${_thefuckplus_candidates[i]}")
        else
          lines+=("❯ ${_thefuckplus_candidates[i]}")
        fi
      else
        if _thefuckplus_is_destructive "$i"; then
          lines+=("${icon}   ${_thefuckplus_candidates[i]}")
        else
          lines+=("  ${_thefuckplus_candidates[i]}")
        fi
      fi
    done
  fi
  POSTDISPLAY=$'\n'"${(F)lines}"

  local -a hl
  local buf_len=${#BUFFER}

  if _thefuckplus_has_syntax_highlight; then
    # Let syntax-highlighting paint BUFFER (commands, paths, args, etc.)
    _zsh_highlight
    hl=("${region_highlight[@]}")
  else
    hl+=("0 ${buf_len} ${_THEFUCKPLUS_HL_SELECTED}")
  fi

  # Walk POSTDISPLAY: combined index starts at buf_len
  local pos=$buf_len
  local line start end cand_i=0
  for line in "${lines[@]}"; do
    (( pos++ )) # leading newline before each line
    start=$pos
    end=$(( start + ${#line} ))
    if (( cand_i == 0 )); then
      # Entire status/hint line in bright yellow when current candidate is destructive.
      if _thefuckplus_is_destructive; then
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_WARN}")
      else
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_MUTED}")
      fi
    elif (( cand_i == _thefuckplus_index )); then
      if _thefuckplus_is_destructive "$cand_i"; then
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_WARN}")
      else
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_SELECTED}")
      fi
    else
      if _thefuckplus_is_destructive "$cand_i"; then
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_WARN_MUTED}")
      else
        hl+=("${start} ${end} ${_THEFUCKPLUS_HL_MUTED}")
      fi
    fi
    pos=$end
    (( cand_i++ ))
  done

  region_highlight=("${hl[@]}")
}

_thefuckplus_show_status() {
  _thefuckplus_apply_highlights
  zle -R
}

# Full reset of select-mode state (menu, flags, keymap). Safe to call repeatedly.
_thefuckplus_reset_state() {
  _thefuckplus_selecting=0
  _thefuckplus_show_list=0
  _thefuckplus_index=1
  _thefuckplus_candidates=()
  _thefuckplus_destructive=()
  _thefuckplus_warnings=()
  POSTDISPLAY=""
  _thefuckplus_saved_postdisplay=""
  region_highlight=()
  _thefuckplus_saved_region_highlight=()
  if [[ -n "$_thefuckplus_saved_keytimeout" ]]; then
    KEYTIMEOUT=$_thefuckplus_saved_keytimeout
    _thefuckplus_saved_keytimeout=""
  fi
  if [[ "$KEYMAP" == "thefuckplus-select" ]]; then
    zle -K "${_thefuckplus_saved_keymap:-main}" 2>/dev/null || zle -K main
  fi
  _thefuckplus_saved_keymap=""
}

# Re-apply after other plugins touch region_highlight; keep our hook last.
_thefuckplus_line_pre_redraw() {
  # Orphaned select: flag still set but keymap already left (e.g. Ctrl+C/send-break).
  # Must reset — otherwise every redraw rebuilds the candidate menu.
  if (( _thefuckplus_selecting )) && [[ "$KEYMAP" != "thefuckplus-select" ]]; then
    _thefuckplus_reset_state
    return
  fi
  (( _thefuckplus_selecting )) || return
  _thefuckplus_apply_highlights
}

_thefuckplus_register_redraw_hook() {
  (( $+functions[add-zle-hook-widget] )) || {
    autoload -Uz add-zle-hook-widget 2>/dev/null
    (( $+functions[add-zle-hook-widget] )) || return
  }
  # Remove + re-add so we run after zsh-syntax-highlighting's pre-redraw hook.
  add-zle-hook-widget -d line-pre-redraw _thefuckplus_line_pre_redraw 2>/dev/null
  add-zle-hook-widget -Uz line-pre-redraw _thefuckplus_line_pre_redraw
}

if (( ! ${+_THEFUCKPLUS_REDRAW_HOOKED} )); then
  typeset -g _THEFUCKPLUS_REDRAW_HOOKED=1
  _thefuckplus_register_redraw_hook
fi

_thefuckplus_apply_candidate() {
  BUFFER="${_thefuckplus_candidates[$_thefuckplus_index]}"
  CURSOR=${#BUFFER}
  _thefuckplus_show_status
}

_thefuckplus_exit_select() {
  local saved_post="$_thefuckplus_saved_postdisplay"
  local -a saved_hl
  saved_hl=("${_thefuckplus_saved_region_highlight[@]}")
  _thefuckplus_leave_select_keymap
  _thefuckplus_reset_state
  # Restore pre-select POSTDISPLAY only if it was not our menu (usually empty / autosuggest).
  if [[ -n "$saved_post" && "$saved_post" != *$'\nUse ↑ and ↓'* && "$saved_post" != *$'\n⚠'* ]]; then
    POSTDISPLAY="$saved_post"
  fi
  region_highlight=("${saved_hl[@]}")
  zle -R
}

_thefuckplus_up() {
  (( _thefuckplus_selecting )) || { zle up-line-or-history; return }
  _thefuckplus_show_list=1
  (( _thefuckplus_index > 1 )) && (( _thefuckplus_index-- ))
  _thefuckplus_apply_candidate
}

_thefuckplus_down() {
  (( _thefuckplus_selecting )) || { zle down-line-or-history; return }
  _thefuckplus_show_list=1
  (( _thefuckplus_index < ${#_thefuckplus_candidates} )) && (( _thefuckplus_index++ ))
  _thefuckplus_apply_candidate
}

_thefuckplus_tab() {
  (( _thefuckplus_selecting )) || { zle expand-or-complete; return }
  BUFFER="${_thefuckplus_candidates[$_thefuckplus_index]}"
  CURSOR=${#BUFFER}
  _thefuckplus_exit_select
}

_thefuckplus_enter() {
  (( _thefuckplus_selecting )) || { zle .accept-line; return }
  BUFFER="${_thefuckplus_candidates[$_thefuckplus_index]}"
  CURSOR=${#BUFFER}
  local mode="$_thefuckplus_execute_mode"
  _thefuckplus_exit_select
  if [[ "$mode" == "execute" ]]; then
    zle .accept-line
  fi
}

_thefuckplus_cancel() {
  if (( ! _thefuckplus_selecting )); then
    # Still scrub leftovers if Ctrl+C raced with state flags.
    if (( ${#_thefuckplus_candidates} > 0 )) || [[ -n "$POSTDISPLAY" && "$POSTDISPLAY" == *$'\nUse ↑ and ↓'* ]]; then
      BUFFER=""
      CURSOR=0
      _thefuckplus_reset_state
      zle -R
    fi
    return
  fi
  BUFFER="$_thefuckplus_saved_buffer"
  CURSOR=$_thefuckplus_saved_cursor
  _thefuckplus_exit_select
}

# If Ctrl+C / send-break aborted ZLE without running cancel, scrub leftover state.
_thefuckplus_line_init() {
  if (( _thefuckplus_selecting )) || [[ "$KEYMAP" == "thefuckplus-select" ]] || (( ${#_thefuckplus_candidates} > 0 )); then
    _thefuckplus_reset_state
  fi
  # Drop a lingering menu POSTDISPLAY even if flags were already cleared.
  if [[ -n "$POSTDISPLAY" && ( "$POSTDISPLAY" == *$'\nUse ↑ and ↓'* || "$POSTDISPLAY" == *$'\n⚠'* || "$POSTDISPLAY" == *$'\n❯'* ) ]]; then
    POSTDISPLAY=""
  fi
}

zle -N _thefuckplus_up
zle -N _thefuckplus_down
zle -N _thefuckplus_tab
zle -N _thefuckplus_enter
zle -N _thefuckplus_cancel
zle -N _thefuckplus_line_init

_thefuckplus_start_select() {
  if (( ${#_thefuckplus_candidates} == 0 )); then
    zle -M "thefuckplus: no candidates"
    return 1
  fi
  _thefuckplus_saved_buffer="$BUFFER"
  _thefuckplus_saved_cursor=$CURSOR
  _thefuckplus_saved_postdisplay="$POSTDISPLAY"
  _thefuckplus_saved_region_highlight=("${region_highlight[@]}")
  _thefuckplus_index=1
  _thefuckplus_selecting=1
  _thefuckplus_show_list=0
  _thefuckplus_register_redraw_hook
  _thefuckplus_enter_select_keymap
  _thefuckplus_apply_candidate
}

# line-init hook (and legacy widget name for older zsh)
if (( ! ${+_THEFUCKPLUS_LINEINIT_HOOKED} )); then
  typeset -g _THEFUCKPLUS_LINEINIT_HOOKED=1
  autoload -Uz add-zle-hook-widget 2>/dev/null
  if (( $+functions[add-zle-hook-widget] )); then
    add-zle-hook-widget -Uz line-init _thefuckplus_line_init
  else
    # Fallback: chain zle-line-init if present
    if (( $+functions[zle-line-init] )); then
      functions[_thefuckplus_orig_zle_line_init]=${functions[zle-line-init]}
      zle-line-init() {
        _thefuckplus_orig_zle_line_init "$@"
        _thefuckplus_line_init
      }
    else
      zle-line-init() { _thefuckplus_line_init; }
    fi
    zle -N zle-line-init
  fi
fi

# Resolve previous command for fix mode, skipping a just-recorded fuck line.
_thefuckplus_prev_command() {
  local cmd="${_THEFUCKPLUS_LAST_CMD}"
  if [[ -z "$cmd" ]]; then
    cmd="$(fc -ln -1 2>/dev/null | tail -n1)"
  fi
  if [[ "$cmd" == "fuck" || "$cmd" == fuck\ * ]]; then
    cmd="$(fc -ln -2 2>/dev/null | head -n1)"
  fi
  print -r -- "$cmd"
}

# Run binary in background; animate spinner via zle -R.
# Suppress job-control notices like: [1] 12345 done ( ... )
_thefuckplus_call_with_spinner() {
  setopt LOCAL_OPTIONS NO_MONITOR NO_NOTIFY
  local outfile="$1" errfile="$2"
  shift 2
  local pid frame_i n rcfile
  : >"$outfile"
  : >"$errfile"
  rcfile="$(mktemp "${TMPDIR:-/tmp}/thefuckplus.rc.XXXXXX")"
  (
    _thefuckplus_bin "$@" >"$outfile" 2>"$errfile"
    print -r -- $? >"$rcfile"
  ) &!
  pid=$!
  frame_i=1
  n=${#_THEFUCKPLUS_SPINNER_FRAMES}
  while kill -0 "$pid" 2>/dev/null; do
    zle -R "${_THEFUCKPLUS_SPINNER_FRAMES[frame_i]} AI is fucking it..."
    (( frame_i = frame_i % n + 1 ))
    sleep 0.08
  done
  # disowned job: read exit code from rcfile instead of wait
  local waits=0
  while [[ ! -s "$rcfile" ]] && (( waits < 100 )); do
    sleep 0.01
    (( waits++ ))
  done
  _thefuckplus_spinner_rc="$(<"$rcfile")"
  [[ -n "$_thefuckplus_spinner_rc" ]] || _thefuckplus_spinner_rc=1
  rm -f "$rcfile"
  _thefuckplus_spinner_out="$(<"$outfile")"
}

_thefuckplus_invoke() {
  local mode="$1"
  shift
  local out errfile rc outfile
  typeset -g _thefuckplus_spinner_rc=1
  typeset -g _thefuckplus_spinner_out=""
  errfile="$(mktemp "${TMPDIR:-/tmp}/thefuckplus.XXXXXX")"
  outfile="$(mktemp "${TMPDIR:-/tmp}/thefuckplus.out.XXXXXX")"

  if [[ "$mode" == "generate" ]]; then
    _thefuckplus_call_with_spinner "$outfile" "$errfile" generate --query "$*"
  else
    _thefuckplus_call_with_spinner "$outfile" "$errfile" fix \
      --command "$(_thefuckplus_prev_command)" \
      --exit "${_THEFUCKPLUS_LAST_EXIT:-0}"
  fi
  rc=$_thefuckplus_spinner_rc
  out="$_thefuckplus_spinner_out"

  if (( rc == 3 )) || grep -q 'need_config' "$errfile" 2>/dev/null; then
    zle -R ""
    print -u2 "thefuckplus: AI not configured — launching setup..."
    _thefuckplus_bin configure </dev/tty >/dev/tty 2>/dev/tty
    rc=$?
    if (( rc == 0 )); then
      if [[ "$mode" == "generate" ]]; then
        _thefuckplus_call_with_spinner "$outfile" "$errfile" generate --query "$*"
      else
        _thefuckplus_call_with_spinner "$outfile" "$errfile" fix \
          --command "$(_thefuckplus_prev_command)" \
          --exit "${_THEFUCKPLUS_LAST_EXIT:-0}"
      fi
      rc=$_thefuckplus_spinner_rc
      out="$_thefuckplus_spinner_out"
    fi
  fi

  rm -f "$outfile"
  unset _thefuckplus_spinner_rc _thefuckplus_spinner_out

  if (( rc != 0 )); then
    zle -R ""
    local errmsg
    errmsg="$(cat "$errfile" 2>/dev/null)"
    rm -f "$errfile"
    zle -M "thefuckplus: ${errmsg:-failed}"
    return 1
  fi
  rm -f "$errfile"

  if ! _thefuckplus_parse_result "$out"; then
    zle -R ""
    zle -M "thefuckplus: could not parse response"
    return 1
  fi
  _thefuckplus_start_select
}

# Intercept Enter when the line is `fuck` or `fuck <natural language>`.
_thefuckplus_accept_line() {
  if (( _thefuckplus_selecting )); then
    _thefuckplus_enter
    return
  fi

  local -a words
  words=("${(z)BUFFER}")
  if (( ${#words} >= 1 )) && [[ "${words[1]}" == "fuck" ]]; then
    # Record the fuck line in history so ↑ can recall / edit the prompt.
    print -s -- "${BUFFER%%$'\n'*}"
    BUFFER=""
    CURSOR=0
    if (( ${#words} == 1 )); then
      _thefuckplus_invoke fix
    else
      _thefuckplus_invoke generate "${words[@]:1}"
    fi
    return
  fi

  zle .accept-line
}
zle -N accept-line _thefuckplus_accept_line

# Esc Esc → fix previous command (disabled during select; select rebinds \e\e to cancel)
_thefuckplus_esc_esc() {
  if (( _thefuckplus_selecting )); then
    _thefuckplus_cancel
    return
  fi
  _thefuckplus_invoke fix
}
zle -N _thefuckplus_esc_esc
if [[ "$THEFUCKPLUS_ESC_ESC" != "0" ]]; then
  bindkey -M emacs '\e\e' _thefuckplus_esc_esc
  bindkey -M vicmd '\e\e' _thefuckplus_esc_esc
  bindkey -M viins '\e\e' _thefuckplus_esc_esc
fi

# Non-interactive helper: print JSON to stdout.
fuck() {
  if [[ $# -eq 0 ]]; then
    local cmd="${_THEFUCKPLUS_LAST_CMD}"
    [[ -z "$cmd" ]] && cmd="$(fc -ln -1 2>/dev/null)"
    _thefuckplus_bin fix --command "$cmd" --exit "${_THEFUCKPLUS_LAST_EXIT:-0}"
  else
    _thefuckplus_bin generate --query "$*"
  fi
}
