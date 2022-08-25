package main

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func githubCLIExists() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func (config *c) verifyGithubCLIConfig() error {
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		return errors.New("GH_TOKEN must be set")
	}
	return nil
}
