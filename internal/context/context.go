package context

import (
	"os"
	"runtime"
	"strings"
)

type RequestContext struct {
	Command  string
	ExitCode int
	CWD      string
	OS       string
	Shell    string
	Query    string
	Output   string // truncated stdout/stderr of the previous command, if available
}

func Collect(command string, exitCode int, query string) RequestContext {
	return CollectWithOutput(command, exitCode, query, "")
}

func CollectWithOutput(command string, exitCode int, query, output string) RequestContext {
	cwd, _ := os.Getwd()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "zsh"
	}
	return RequestContext{
		Command:  strings.TrimSpace(command),
		ExitCode: exitCode,
		CWD:      cwd,
		OS:       runtime.GOOS + "/" + runtime.GOARCH,
		Shell:    shell,
		Query:    strings.TrimSpace(query),
		Output:   strings.TrimSpace(output),
	}
}
