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
}

func Collect(command string, exitCode int, query string) RequestContext {
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
	}
}
