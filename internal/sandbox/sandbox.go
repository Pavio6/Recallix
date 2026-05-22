package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ErrDisabled = errors.New("skill sandbox is disabled")

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Killed   bool
}

func (r Result) Success() bool { return r.ExitCode == 0 && !r.Killed }

type Executor struct {
	mode    string
	timeout time.Duration
	image   string
}

func New(mode string, timeout time.Duration, image string) *Executor {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if image == "" {
		image = "wechatopenai/weknora-sandbox:latest"
	}
	return &Executor{mode: strings.TrimSpace(mode), timeout: timeout, image: image}
}

func (e *Executor) Enabled() bool { return e.mode == "local" || e.mode == "docker" }

func (e *Executor) Execute(ctx context.Context, scriptPath, workDir string, args []string, stdin string) (Result, error) {
	if !e.Enabled() {
		return Result{}, ErrDisabled
	}
	if err := validateScriptPath(scriptPath, workDir); err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return Result{}, err
	}
	if err := validateContent(string(content)); err != nil {
		return Result{}, err
	}
	for _, arg := range args {
		if dangerousArg(arg) {
			return Result{}, fmt.Errorf("unsafe script argument rejected")
		}
	}

	interpreter, err := interpreterFor(scriptPath)
	if err != nil {
		return Result{}, err
	}
	if e.mode == "docker" {
		return e.executeDocker(ctx, scriptPath, workDir, args, stdin)
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, interpreter, append([]string{scriptPath}, args...)...)
	cmd.Dir = workDir
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start)}
	if runErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.Killed = true
			result.ExitCode = -1
			return result, nil
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	return result, nil
}

func (e *Executor) executeDocker(ctx context.Context, scriptPath, workDir string, args []string, stdin string) (Result, error) {
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	relativeScript, err := filepath.Rel(workDir, scriptPath)
	if err != nil {
		return Result{}, err
	}
	commandArgs := []string{
		"run", "--rm",
		"--network", "none",
		"--memory", "256m",
		"--cpus", "1",
		"--pids-limit", "100",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"-v", fmt.Sprintf("%s:/workspace:ro", workDir),
		"-w", "/workspace",
		e.image,
	}
	interpreter, err := interpreterFor(relativeScript)
	if err != nil {
		return Result{}, err
	}
	commandArgs = append(commandArgs, interpreter, filepath.ToSlash(relativeScript))
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(execCtx, "docker", commandArgs...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start)}
	if runErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.Killed = true
			result.ExitCode = -1
			return result, nil
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	return result, nil
}

func validateScriptPath(scriptPath, workDir string) error {
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return err
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absScript, absWork+string(filepath.Separator)) {
		return fmt.Errorf("script path outside sandbox workdir")
	}
	return nil
}

func interpreterFor(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python3", nil
	case ".sh", ".bash":
		return "bash", nil
	case ".js":
		return "node", nil
	default:
		return "", fmt.Errorf("unsupported script type")
	}
}

var blockedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-rf\b`),
	regexp.MustCompile(`(?i)\bcurl\b|\bwget\b|\brequests\.`),
	regexp.MustCompile(`(?i)\bos\.system\b|\bsubprocess\.`),
	regexp.MustCompile(`(?i)\beval\s*\(`),
	regexp.MustCompile(`(?i)/etc/passwd|~/.ssh`),
}

func validateContent(content string) error {
	for _, pattern := range blockedPatterns {
		if pattern.MatchString(content) {
			return fmt.Errorf("unsafe script content rejected")
		}
	}
	return nil
}

func dangerousArg(arg string) bool {
	return strings.ContainsAny(arg, "\n\r;|><`") || strings.Contains(arg, "$(") || strings.Contains(arg, "..")
}
