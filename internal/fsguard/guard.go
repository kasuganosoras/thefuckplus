package fsguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kasuganosoras/thefuckplus/internal/config"
)

// DefaultDenyGlobs are matched against slash-normalized paths relative to CWD
// and against the basename. Used when AllowSensitive is false.
var DefaultDenyGlobs = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"id_rsa",
	"id_rsa.*",
	"id_ed25519",
	"id_ed25519.*",
	"id_ecdsa",
	"id_ecdsa.*",
	"id_dsa",
	"id_dsa.*",
	".npmrc",
	".pypirc",
	"credentials",
	"credentials.json",
	"*.secret",
	"*secret*",
	"*.keystore",
	".aws",
	".aws/**",
	".gnupg",
	".gnupg/**",
	".ssh",
	".ssh/**",
	"config.yaml", // thefuckplus / similar configs often hold API keys
}

// Guard enforces CWD sandbox and sensitive-path policy.
type Guard struct {
	Root           string
	MaxFileBytes   int
	MaxListEntries int
	AllowSensitive bool
	ExtraAllow     []string
	ExtraDeny      []string
}

func New(root string, agent config.AgentConfig) (*Guard, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		// CWD may not exist yet; keep Abs path.
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	maxBytes := agent.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = 32768
	}
	maxList := agent.MaxListEntries
	if maxList <= 0 {
		maxList = 100
	}
	return &Guard{
		Root:           abs,
		MaxFileBytes:   maxBytes,
		MaxListEntries: maxList,
		AllowSensitive: agent.AllowSensitive,
		ExtraAllow:     append([]string(nil), agent.ExtraAllow...),
		ExtraDeny:      append([]string(nil), agent.ExtraDeny...),
	}, nil
}

type toolError struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

func errJSON(code, reason string) string {
	b, _ := json.Marshal(toolError{OK: false, Error: code, Reason: reason})
	return string(b)
}

// Resolve checks path is under Root after symlink resolution.
// Returns absolute cleaned path and slash-relative path from Root.
func (g *Guard) Resolve(path string) (absPath, relSlash string, errCode, reason string) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	var candidate string
	if filepath.IsAbs(path) {
		candidate = filepath.Clean(path)
	} else {
		candidate = filepath.Clean(filepath.Join(g.Root, path))
	}

	resolved := candidate
	if fi, err := os.Lstat(candidate); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return "", "", "not_found", err.Error()
			}
			resolved = target
		} else if fi.IsDir() {
			// EvalSymlinks on dirs resolves intermediate links.
			if t, err := filepath.EvalSymlinks(candidate); err == nil {
				resolved = t
			}
		} else {
			// Resolve parent for non-symlink files so parent escapes are caught.
			dir := filepath.Dir(candidate)
			if t, err := filepath.EvalSymlinks(dir); err == nil {
				resolved = filepath.Join(t, filepath.Base(candidate))
			}
		}
	} else if os.IsNotExist(err) {
		// Still ensure the cleaned path stays under root (no need for file to exist for policy).
		resolved = candidate
	} else {
		return "", "", "io_error", err.Error()
	}

	rel, err := filepath.Rel(g.Root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", "denied", "path_outside_cwd"
	}
	relSlash = filepath.ToSlash(rel)
	if !g.allowRel(relSlash) {
		return "", "", "denied", "sensitive_path"
	}
	return resolved, relSlash, "", ""
}

func (g *Guard) allowRel(relSlash string) bool {
	base := filepath.Base(relSlash)
	// extra_deny always applies
	if matchAny(relSlash, base, g.ExtraDeny) {
		return false
	}
	denied := false
	if !g.AllowSensitive {
		if matchAny(relSlash, base, DefaultDenyGlobs) {
			denied = true
		}
	}
	if !denied {
		return true
	}
	// extra_allow can override default deny (not extra_deny)
	if matchAny(relSlash, base, g.ExtraAllow) {
		return true
	}
	return false
}

func matchAny(relSlash, base string, globs []string) bool {
	for _, g := range globs {
		g = filepath.ToSlash(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
		if ok, _ := filepath.Match(g, relSlash); ok {
			return true
		}
		// prefix /** patterns: ".aws/**"
		if strings.HasSuffix(g, "/**") {
			prefix := strings.TrimSuffix(g, "/**")
			if relSlash == prefix || strings.HasPrefix(relSlash, prefix+"/") {
				return true
			}
		}
		// substring-style *secret* already covered by Match on base/rel
	}
	return false
}

// ReadFile returns wrapped content or a JSON error string for the model.
func (g *Guard) ReadFile(path string) string {
	abs, rel, code, reason := g.Resolve(path)
	if code != "" {
		return errJSON(code, reason)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return errJSON("not_found", "file_not_found")
		}
		return errJSON("io_error", err.Error())
	}
	if fi.IsDir() {
		return errJSON("denied", "is_directory")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return errJSON("io_error", err.Error())
	}
	truncated := false
	if len(data) > g.MaxFileBytes {
		data = data[:g.MaxFileBytes]
		truncated = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<untrusted_file path=%q>\n", rel)
	b.Write(data)
	if truncated {
		b.WriteString("\n[truncated]")
	}
	b.WriteString("\n</untrusted_file>")
	return b.String()
}

// ListDir returns a text listing or JSON error.
func (g *Guard) ListDir(path string) string {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	abs, rel, code, reason := g.Resolve(path)
	if code != "" {
		return errJSON(code, reason)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return errJSON("not_found", "dir_not_found")
		}
		return errJSON("io_error", err.Error())
	}
	if !fi.IsDir() {
		return errJSON("denied", "not_a_directory")
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return errJSON("io_error", err.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<untrusted_dir path=%q>\n", rel)
	n := 0
	for _, e := range entries {
		name := e.Name()
		childRel := name
		if rel != "." {
			childRel = rel + "/" + name
		}
		// Skip listing denied names (do not reveal content; omit from list)
		if !g.allowRel(filepath.ToSlash(childRel)) {
			continue
		}
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", name, suffix)
		n++
		if n >= g.MaxListEntries {
			b.WriteString("[truncated]\n")
			break
		}
	}
	b.WriteString("</untrusted_dir>")
	return b.String()
}
