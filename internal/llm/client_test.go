package llm

import "testing"

func TestParseCandidatesJSONStrings(t *testing.T) {
	in := `["find ~ -type f -size +100M", "fd --type f --size +100m"]`
	got := ParseCandidates(in, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
	if got[0].Command != "find ~ -type f -size +100M" {
		t.Fatalf("unexpected first: %q", got[0].Command)
	}
}

func TestParseCandidatesObjects(t *testing.T) {
	in := `[{"command":"rm -rf /tmp/x","destructive":true,"warning":"recursive delete"},{"command":"ls","destructive":false}]`
	got := ParseCandidates(in, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %#v", got)
	}
	if !got[0].Destructive || got[0].Command != "rm -rf /tmp/x" {
		t.Fatalf("unexpected first: %#v", got[0])
	}
	if got[1].Destructive {
		t.Fatalf("ls should not be marked destructive by parse alone: %#v", got[1])
	}
}

func TestParseCandidatesFenced(t *testing.T) {
	in := "```json\n[\"ls -la\"]\n```"
	got := ParseCandidates(in, 5)
	if len(got) != 1 || got[0].Command != "ls -la" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestParseCandidatesLines(t *testing.T) {
	in := "git status\ngit status -sb"
	got := ParseCandidates(in, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %#v", got)
	}
}

func TestEnrichSafety(t *testing.T) {
	got := enrichSafety([]Candidate{{Command: "rm -rf /tmp/a", Destructive: false}})
	if len(got) != 1 || !got[0].Destructive {
		t.Fatalf("local heuristic should mark destructive: %#v", got)
	}
}

func TestUnwrapNestedCommandJSON(t *testing.T) {
	in := `[{"command":"{\"command\":\"npm run build:h5\",\"destructive\":false,\"warning\":\"\"}","destructive":false}]`
	got := ParseCandidates(in, 5)
	if len(got) != 1 || got[0].Command != "npm run build:h5" {
		t.Fatalf("unexpected unwrap: %#v", got)
	}
}
