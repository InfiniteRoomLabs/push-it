// Package installer wires push-it into git's global pre-push hook reversibly.
package installer

import (
	"errors"
	"os/exec"
	"strings"
)

// Git is the subset of `git config --global` the installer needs.
type Git interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Unset(key string) error
}

// CLIGit shells out to git.
type CLIGit struct{}

func (CLIGit) Get(key string) (string, error) {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", nil // unset
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (CLIGit) Set(key, value string) error {
	return exec.Command("git", "config", "--global", key, value).Run()
}

func (CLIGit) Unset(key string) error {
	err := exec.Command("git", "config", "--global", "--unset", key).Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 5 {
		return nil // already unset
	}
	return err
}
