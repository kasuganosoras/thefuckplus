package fsguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kasuganosoras/thefuckplus/internal/config"
)

func TestReadPackageJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"build:h5":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root, config.DefaultAgent())
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile("package.json")
	if strings.Contains(out, `"ok":false`) {
		t.Fatalf("expected success, got %s", out)
	}
	if !strings.Contains(out, "build:h5") {
		t.Fatalf("missing content: %s", out)
	}
	if !strings.Contains(out, "<untrusted_file") {
		t.Fatalf("missing wrapper: %s", out)
	}
}

func TestDenyEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := New(root, config.DefaultAgent())
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile(".env")
	if !strings.Contains(out, "sensitive_path") {
		t.Fatalf("expected deny, got %s", out)
	}
	if strings.Contains(out, "SECRET") {
		t.Fatal("must not leak file content")
	}
}

func TestExtraAllowEnvExample(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("KEY=placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := config.DefaultAgent()
	agent.ExtraAllow = []string{".env.example"}
	g, err := New(root, agent)
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile(".env.example")
	if strings.Contains(out, `"ok":false`) {
		t.Fatalf("expected allow, got %s", out)
	}
	if !strings.Contains(out, "placeholder") {
		t.Fatalf("missing content: %s", out)
	}
}

func TestPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := New(root, config.DefaultAgent())
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile(filepath.Join("..", filepath.Base(outside), "secret.txt"))
	// Rel from root/../outside may still escape
	out2 := g.ReadFile(secret)
	for _, o := range []string{out, out2} {
		if !strings.Contains(o, "path_outside_cwd") && !strings.Contains(o, "denied") {
			// absolute outside path should be denied
			if strings.Contains(o, "nope") {
				t.Fatalf("escaped sandbox: %s", o)
			}
		}
	}
	if strings.Contains(out2, "nope") {
		t.Fatal("absolute path outside cwd leaked")
	}
}

func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink not supported")
	}
	g, err := New(root, config.DefaultAgent())
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile("link.txt")
	if strings.Contains(out, "leak") {
		t.Fatalf("symlink escape: %s", out)
	}
	if !strings.Contains(out, "denied") && !strings.Contains(out, "path_outside_cwd") {
		t.Fatalf("expected deny, got %s", out)
	}
}

func TestAllowSensitive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent := config.DefaultAgent()
	agent.AllowSensitive = true
	g, err := New(root, agent)
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile(".env")
	if strings.Contains(out, `"ok":false`) {
		t.Fatalf("expected allow when allow_sensitive, got %s", out)
	}
}

func TestExtraDenyWins(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := config.DefaultAgent()
	agent.AllowSensitive = true
	agent.ExtraDeny = []string{"package.json"}
	g, err := New(root, agent)
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile("package.json")
	if !strings.Contains(out, "sensitive_path") {
		t.Fatalf("extra_deny should win, got %s", out)
	}
}

func TestListDirSkipsDenied(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".env"), []byte("x"), 0o600)
	g, err := New(root, config.DefaultAgent())
	if err != nil {
		t.Fatal(err)
	}
	out := g.ListDir(".")
	if !strings.Contains(out, "ok.txt") {
		t.Fatalf("missing ok.txt: %s", out)
	}
	if strings.Contains(out, ".env") {
		t.Fatalf("should omit .env: %s", out)
	}
}

func TestTruncate(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("a", 1000)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := config.DefaultAgent()
	agent.MaxFileBytes = 100
	g, err := New(root, agent)
	if err != nil {
		t.Fatal(err)
	}
	out := g.ReadFile("big.txt")
	if !strings.Contains(out, "[truncated]") {
		t.Fatalf("expected truncation: %s", out)
	}
}
