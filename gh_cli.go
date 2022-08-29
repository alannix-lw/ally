package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/slack-go/slack"
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

func runGithubAction(api *slack.Client, channel, args string) error {
	timestamp := postSlackMessage(api, channel,
		slack.MsgOptionText(
			fmt.Sprintf(":waiting: Running Github Action with args: '%s' :rocket:", args),
			false,
		))

	var success bool
	defer func() {
		if success {
			updateSlackMessage(api, channel, timestamp,
				slack.MsgOptionText(":white_check_mark: That was a success!", false),
			)
			return
		}
		updateSlackMessage(api, channel, timestamp,
			slack.MsgOptionText(
				":x: Something went wrong while running the Github Action!", false),
		)
	}()

	cmd := GenerateGithubCommandWithArgs(args)
	logger.Infow("running github workflow", "command", cmd.String())

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

func GenerateGithubCommandWithArgs(args string) *exec.Cmd {
	cmd := []string{"workflow", "run"}
	return exec.Command("gh", append(cmd, strings.Split(args, " ")...)...)
}

func runGithubActionWithCallback(api *slack.Client, config *c,
	callback slack.InteractionCallback, mfaToken, tag string) error {
	if mfaToken == "" {
		return errors.New("unable to process callback event, missing MFA token")
	}
	if tag == "" {
		return errors.New("unable to process callback event, missing Github tag")
	}

	notifySlackChannel(api,
		config.NotifySlackChannel,
		fmt.Sprintf(
			"User %s approved signing of the *Lacework CLI %s* :chewbacca:",
			callback.User.Name, tag,
		),
	)

	// Update the same message which is built by renderPayloadToSignCLI()
	// and remove the last two blocks that asks for the MFA token and has
	// the button to approve.
	//
	// Instead, tell everyone who approved it!
	approverText := slack.NewTextBlockObject(
		slack.MarkdownType,
		fmt.Sprintf("\n:white_check_mark: *Approved by %s*", callback.User.Name),
		false, false)
	approverSection := slack.NewSectionBlock(approverText, nil, nil)

	updatedMessage := []slack.Block{
		callback.Message.Blocks.BlockSet[0],
		callback.Message.Blocks.BlockSet[1],
		approverSection,
	}

	postSlackMessage(api, callback.Channel.ID,
		slack.MsgOptionBlocks(updatedMessage...),
		slack.MsgOptionReplaceOriginal(callback.ResponseURL),
	)

	timestamp := postSlackMessage(api, callback.Channel.ID,
		slack.MsgOptionText(
			fmt.Sprintf(
				":waiting: Running Github Action to sign the *Lacework CLI %s* :rocket:", tag,
			),
			false,
		),
	)

	var success bool
	defer func() {
		if success {
			updateSlackMessage(api, callback.Channel.ID, timestamp,
				slack.MsgOptionText("That was a success! :megamix:", false),
			)
			return
		}
		updateSlackMessage(api, callback.Channel.ID, timestamp,
			slack.MsgOptionText(
				":x: Something went wrong while running the Github Action!", false),
		)
	}()

	cmd := GenerateGithubCommandWithArgs(
		fmt.Sprintf(
			"32728677 -R lacework-dev/lacework-cli-signing --field mfa_token=%s --field branch_or_tag=%s",
			mfaToken, tag,
		),
	)
	logger.Infow("running github workflow", "command", cmd.String())

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

func renderPayloadToSignCLI(tag, pipeline string) []slack.Block {
	headerText := slack.NewTextBlockObject(
		slack.MarkdownType,
		"*A new release of the Lacework CLI is ready to be signed.*\n\n"+
			"Only authorized users with Okta Verify configured can approve this action.",
		false, false)
	headerSection := slack.NewSectionBlock(headerText, nil, nil)

	detailsText := slack.NewTextBlockObject(slack.MarkdownType,
		"*:1234: Version:* "+tag+
			"\n*:win-as-a-team: Approver:* <!subteam^S01JP5A3ACQ|@allies>\n"+
			"\n*:codefresh: Triggered by pipeline:*\n"+pipeline+"\n"+
			"\n*:gear: Approve to run Github Action:*\n"+
			"https://github.com/lacework-dev/lacework-cli-signing/actions\n", false, false)
	detailsImage := slack.NewImageBlockElement(
		"https://upload.wikimedia.org/wikipedia/commons/c/c7/Windows_logo_-_2012.png",
		"windows logo",
	)

	detailsSection := slack.NewSectionBlock(detailsText, nil, slack.NewAccessory(detailsImage))

	inputTxt := slack.PlainTextInputBlockElement{
		Type:      "plain_text_input",
		ActionID:  SlackMfaTokenForGithubAction,
		Multiline: false,
		MaxLength: 6, // Tokens are always 6 numbers
	}

	inputBlock := slack.NewInputBlock(
		SlackSignLaceworkCLIGithubAction,
		slack.NewTextBlockObject(
			slack.PlainTextType,
			":key: MFA Token",
			false,
			false,
		),
		nil,
		inputTxt,
	)

	// Approve Button
	approveBtnTxt := slack.NewTextBlockObject(slack.PlainTextType, "Approve", false, false)
	approveBtn := slack.NewButtonBlockElement("", "click_me", approveBtnTxt)
	actionBlock := slack.NewActionBlock("", approveBtn)

	return []slack.Block{
		headerSection,
		detailsSection,
		inputBlock,
		actionBlock,
	}
}
