package safety

import (
	"regexp"
	"strings"
)

// Assessment is a local heuristic for destructive shell commands.
type Assessment struct {
	Destructive bool
	Warning     string
}

var (
	reRmRecursive = regexp.MustCompile(`(^|[;&|]\s*)(sudo\s+)?rm\s+(-[a-zA-Z]*r[a-zA-Z]*|--recursive\b)`)
	reRmForceDeep = regexp.MustCompile(`(^|[;&|]\s*)(sudo\s+)?rm\s+.*\s+(/|/\*|~|\$HOME|\$\{HOME\})(\s|$|/)`)
	reDdDisk      = regexp.MustCompile(`\bdd\b.*\bof=/dev/`)
	reMkfs        = regexp.MustCompile(`(^|[;&|]\s*)(sudo\s+)?mkfs(\.|[.\s]|$)`)
	reWipefs      = regexp.MustCompile(`\bwipefs\b`)
	reShred       = regexp.MustCompile(`\bshred\b`)
	reChmodRoot   = regexp.MustCompile(`\bchmod\s+(-R\s+)?([0-7]{3,4}\s+)?/(\s|$)`)
	reChownRoot   = regexp.MustCompile(`\bchown\s+(-R\s+)?.*/(\s|$)`)
	rePipeShell   = regexp.MustCompile(`(curl|wget)\b.*\|\s*(sudo\s+)?(ba)?sh\b`)
	reGitForce    = regexp.MustCompile(`\bgit\s+push\b.*(--force|-f)\b`)
	reGitHard     = regexp.MustCompile(`\bgit\s+reset\s+--hard\b`)
	reDropDB      = regexp.MustCompile(`(?i)\b(drop\s+(database|table|schema)|truncate\s+table)\b`)
	reIptablesF   = regexp.MustCompile(`\b(iptables|nft)\b.*\b(-F|--flush)\b`)
	reMkfsAlt     = regexp.MustCompile(`\b(pvcreate|vgremove|lvremove)\b`)
	reDiskPart    = regexp.MustCompile(`\b(fdisk|parted|sgdisk)\b`)
	reRedirectDev = regexp.MustCompile(`>\s*/dev/[sn]d[a-z]`)
)

type rule struct {
	re      *regexp.Regexp
	warning string
}

var rules = []rule{
	{reRmRecursive, "recursive delete"},
	{reRmForceDeep, "delete at filesystem root/home"},
	{reDdDisk, "raw disk write (dd)"},
	{reMkfs, "format filesystem"},
	{reWipefs, "wipe filesystem signatures"},
	{reShred, "secure file destroy"},
	{reChmodRoot, "chmod on /"},
	{reChownRoot, "chown on /"},
	{rePipeShell, "pipe remote script to shell"},
	{reGitForce, "force push"},
	{reGitHard, "hard reset"},
	{reDropDB, "drop/truncate database objects"},
	{reIptablesF, "flush firewall rules"},
	{reMkfsAlt, "LVM destructive operation"},
	{reDiskPart, "partition editor"},
	{reRedirectDev, "redirect to block device"},
}

// Assess returns whether a command looks destructive using local heuristics.
func Assess(command string) Assessment {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return Assessment{}
	}
	for _, r := range rules {
		if r.re.MatchString(cmd) {
			return Assessment{Destructive: true, Warning: r.warning}
		}
	}
	// Broad rm of large trees without -i
	fields := strings.Fields(cmd)
	if len(fields) >= 2 {
		base := fields[0]
		if base == "sudo" && len(fields) >= 3 {
			base = fields[1]
		}
		if base == "rm" {
			joined := strings.Join(fields, " ")
			if strings.Contains(joined, "-rf") || strings.Contains(joined, "-fr") ||
				(strings.Contains(joined, "-r") && strings.Contains(joined, "-f")) {
				return Assessment{Destructive: true, Warning: "force recursive delete"}
			}
		}
	}
	return Assessment{}
}

// Merge combines model judgment with local heuristics (OR).
func Merge(command string, modelDestructive bool, modelWarning string) Assessment {
	local := Assess(command)
	if modelDestructive || local.Destructive {
		warn := modelWarning
		if warn == "" {
			warn = local.Warning
		}
		if warn == "" {
			warn = "potentially destructive"
		}
		return Assessment{Destructive: true, Warning: warn}
	}
	return Assessment{}
}
