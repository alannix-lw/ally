package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/slack-go/slack"
)

const codeFreshConfigFile = ".cfconfig"

func defaultCodefreshConfig() string {
	home, err := homedir.Dir()
	if err != nil {
		logger.Fatal("unable to find home directory",
			"error", err,
		)
	}

	return path.Join(home, codeFreshConfigFile)
}

func configureCodefreshCLI(cfConfig string) error {
	cfApiKey := os.Getenv("CODEFRESH_API_KEY")
	if cfApiKey == "" {
		return errors.New("CODEFRESH_API_KEY must be set")
	}

	logger.Info("configuring the codefresh CLI")
	out, err := exec.Command(
		"codefresh", "auth",
		"create-context", "--api-key", cfApiKey,
		"--cfconfig", cfConfig,
	).Output()

	logger.Debugw("command output",
		"cmd", "codefresh auth create-context",
		"output", string(out),
	)

	return err
}

func (config *c) verifyCodefreshConfig() error {
	// if the config does not have a codefresh config, set the default
	if config.CodefreshCfg == "" {
		config.CodefreshCfg = defaultCodefreshConfig()
	}

	// verify the config exist
	if fileExists(config.CodefreshCfg) {
		return nil
	}

	// if the config does not exist, create one
	return configureCodefreshCLI(config.CodefreshCfg)
}

func codefreshCLIExists() bool {
	_, err := exec.LookPath("codefresh")
	return err == nil
}

func fileExists(f string) bool {
	info, err := os.Stat(f)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func runCodefreshPipeline(api *slack.Client, config *c, callback slack.InteractionCallback) error {
	var repo string
	if callback.BlockActionState != nil {
		actions := callback.BlockActionState.Values
		for _, action := range actions {
			repo = action[SlackTriggerTechAllyProject].SelectedOption.Value
		}
	}

	if repo == "" {
		return errors.New("callback event had no repository")
	}

	notifySlackChannel(api,
		config.NotifySlackChannel,
		fmt.Sprintf("A release has been triggered for the *%s* project. :megamix:", repo),
	)

	postSlackMessage(api, callback.Channel.ID,
		slack.MsgOptionText("Roger that! :rockon:", false),
		slack.MsgOptionReplaceOriginal(callback.ResponseURL),
	)

	timestamp := postSlackMessage(api, callback.Channel.ID,
		slack.MsgOptionText(":waiting: Triggering the release PR of the *"+repo+"* project :rocket:", false),
	)

	var success bool
	defer func() {
		if success {
			updateSlackMessage(api, callback.Channel.ID, timestamp,
				slack.MsgOptionText(":white_check_mark: Triggered! (project: *"+repo+"*)", false),
			)
			postSlackMessage(api, callback.Channel.ID,
				slack.MsgOptionText(
					"_:eyes: Look at <#"+config.NotifySlackChannel+"> for the release PR._", false),
			)
			return
		}
		updateSlackMessage(api, callback.Channel.ID, timestamp,
			slack.MsgOptionText(
				":x: Something went wrong while triggering the release! (project: *"+repo+"*)", false),
		)
	}()

	cmd := config.GenerateCodefreshCommandFor(repo)
	logger.Infow("running codefresh pipeline", "command", cmd.String())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "unable to create StdoutPipe")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "unable to create StderrPipe")
	}

	merged := io.MultiReader(stderr, stdout)
	go readCommandBuffer(bufio.NewScanner(merged))

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "unable to start command, buffer error")
	}

	err = cmd.Wait()
	if err == nil {
		success = true
	}
	return err
}

func readCommandBuffer(scanner *bufio.Scanner) {
	for scanner.Scan() {
		logger.Info(scanner.Text())
	}
}

func (config *c) GenerateCodefreshCommandFor(repo string) *exec.Cmd {
	for _, p := range config.Projects {
		if repo == p.Repository {
			args := []string{"run", p.Pipeline, "--cfconfig", config.CodefreshCfg}

			for _, v := range p.Variables {
				args = append(args, "-v", v)
			}

			return exec.Command("codefresh", args...)
		}
	}
	return exec.Command("")
}
