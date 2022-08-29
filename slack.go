package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	// Codefresh pipeline to trigger a new release
	SlackTriggerTechAllyProject  = "trigger_tech_ally_project"
	SlackSelectedTechAllyProject = "selected_tech_ally_project"

	// Github action to sign the Lacework CLI
	SlackSignLaceworkCLIGithubAction = "sign_cli_via_gh_action"
	SlackMfaTokenForGithubAction     = "mfa_token_for_gh_action"

	// The length of the message when signing the Lacework CLI
	AppMentionMessageToSignCLILength = 4
)

func connectToSlackViaSocketmode() (*socketmode.Client, *slack.Client, error) {
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		return nil, nil, errors.New("SLACK_APP_TOKEN must be set")
	}

	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, nil, errors.New("SLACK_APP_TOKEN must have the prefix \"xapp-\".")
	}

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if botToken == "" {
		return nil, nil, errors.New("SLACK_BOT_TOKEN must be set.")
	}

	if !strings.HasPrefix(botToken, "xoxb-") {
		return nil, nil, errors.New("SLACK_BOT_TOKEN must have the prefix \"xoxb-\".")
	}

	api := slack.New(
		botToken,
		slack.OptionDebug(debug()),
		slack.OptionAppLevelToken(appToken),
		slack.OptionLog(log.New(os.Stdout, "api: ", log.Lshortfile|log.LstdFlags)),
	)

	client := socketmode.New(
		api,
		socketmode.OptionDebug(debug()),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)
	return client, api, nil
}

func listenToSlackEvents(client *socketmode.Client, api *slack.Client, config *c) {
	for evt := range client.Events {
		logger.Debugw("raw received", "type", evt.Type, "raw", evt)

		switch evt.Type {

		case socketmode.EventTypeEventsAPI:
			apiEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				logger.Errorw("could not type cast the event to the EventsAPIEvent", "event", evt)
				continue
			}

			client.Ack(*evt.Request)

			err := handleEventMessage(api, config, apiEvent)
			if err != nil {
				logger.Errorw("unable to handle EventsAPI event", "event", evt, "error", err)
			}

		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				logger.Errorw("could not type cast the event to the SlashCommand", "event", evt)
				continue
			}

			logger.Infow("event received",
				"type", evt.Type, "username", cmd.UserName,
				"command", cmd.Command, "channel_name", cmd.ChannelName)

			notifySlackChannel(api,
				config.NotifySlackChannel,
				fmt.Sprintf("User %s is preparing a release via `/release`", cmd.UserName),
			)

			client.Ack(*evt.Request, renderSlackCommandPayload(config))

		case socketmode.EventTypeInteractive:
			callback, ok := evt.Data.(slack.InteractionCallback)
			if !ok {
				logger.Infow("event ignored", "type", evt.Type)
				continue
			}

			logger.Infow("event received",
				"type", evt.Type, "response_url", callback.ResponseURL,
				"value", callback.Value, "channel_name", callback.Channel.Name)

			var payload interface{}
			client.Ack(*evt.Request, payload)

			err := handleInteractiveEvent(api, config, callback)
			if err != nil {
				logger.Errorw("unable to handle Interactive event", "event", evt, "error", err)
			}

		default:
			logger.Warnw("unexpected event type received", "type", evt.Type, "raw", evt)
		}

	}
}

// handleEventMessage will take an event and handle it properly based on the type of event
func handleEventMessage(api *slack.Client, config *c, event slackevents.EventsAPIEvent) error {
	switch event.Type {

	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:

			logger.Infow("event received",
				"type", event.Type, "inner_type", innerEvent.Type,
				"user", ev.User, "channel", ev.Channel, "text", ev.Text)

			if err := handleAppMentionEvent(api, config, ev); err != nil {
				return err
			}
		}

	default:
		return errors.New("unsupported event type")
	}

	return nil
}

// handleAppMentionEvent is used to take care of the AppMentionEvent when the bot is mentioned
//
// if we want to mention the Release Ally App from Codefresh use:
// ```yaml
//   version: "1.0"
//
//   stages:
//     - "test"
//
//   steps:
//     AppMention:
//       type: slack-message-sender
//       arguments:
//         WEBHOOK_URL: ${{SLACK_WEBHOOK_URL}}
//         MESSAGE: "<@U0279A42HV0> hello"
// ```
func handleAppMentionEvent(api *slack.Client, config *c, event *slackevents.AppMentionEvent) error {
	notifySlackChannel(api, config.NotifySlackChannel, formatAppMentionMsg(event.User, event.Channel, event.Text))

	if strings.Contains(event.Text, "sign_cli") {
		actionArgs := strings.Split(event.Text, " ")
		if len(actionArgs) != AppMentionMessageToSignCLILength {
			// Malformed message
			msg := "I was expecting a message with the following format:\n\n" +
				"> @release_ally sign_cli VERSION BUILD_LINK"
			notifySlackChannel(api, event.Channel, msg)
			return nil
		}

		// coming from message:
		//
		// @release_ally sign_cli v0.55.0 https://g.codefresh.io/build/abc123
		tag := actionArgs[2]
		pipeline := actionArgs[3]

		postSlackMessage(api,
			event.Channel,
			slack.MsgOptionBlocks(renderPayloadToSignCLI(tag, pipeline)...),
			slack.MsgOptionMetadata(
				slack.SlackMetadata{
					EventType:    "sign_cli_metadata",
					EventPayload: map[string]interface{}{"tag": tag},
				}),
		)
		return nil
	}

	if strings.Contains(event.Text, "trigger_action") {

		// Trigger generic Github Action, validate message format
		actionArgs := strings.Split(event.Text, ":")
		if len(actionArgs) != 2 {
			// Malformed message
			msg := "I was expecting a message with the following format:\n\n" +
				"> @release_ally trigger_action:WORKFLOW_ID --repo [HOST/]OWNER/REPO"
			notifySlackChannel(api, event.Channel, msg)
			return nil
		}

		return runGithubAction(api, event.Channel, actionArgs[1])
	}

	// Unknown message, print help
	helpText := slack.NewTextBlockObject(
		slack.MarkdownType,
		":waving: Hi there!\n\n"+
			"There are three things I can help you with:\n\n"+
			"*1. To trigger releases from the following <https://lacework.atlassian.net/l/cp/J73uu2wh|list of projects>*\nType: `/release`\n\n"+
			"*2. To sign the Lacework CLI artifacts*\nType: `@release_ally sign_cli VERSION BUILD_LINK`\n\n"+
			"*3. To trigger Github Workflows*\nType: `@release_ally trigger_action:WORKFLOW_ID --repo [HOST/]OWNER/REPO`\n\n"+
			"",
		false, false)
	// TODO maybe add an accesory to make it nicer
	helpSection := slack.NewSectionBlock(helpText, nil, nil)
	postSlackMessage(api,
		event.Channel,
		slack.MsgOptionBlocks(helpSection))
	return nil

}

// createOptionBlockObjects - utility function for generating option block objects
func createOptionBlockObjects(options []string) []*slack.OptionBlockObject {
	optionBlockObjects := make([]*slack.OptionBlockObject, 0, len(options))
	for _, str := range options {
		optionText := slack.NewTextBlockObject(slack.PlainTextType, str, false, false)
		optionBlockObjects = append(optionBlockObjects, slack.NewOptionBlockObject(str, optionText, nil))
	}
	return optionBlockObjects
}

// Update message to Slack wrapper that log errors
func updateSlackMessage(api *slack.Client, channel string, timestamp string, options ...slack.MsgOption) {
	_, _, _, err := api.UpdateMessage(channel, timestamp, options...)
	if err != nil {
		logger.Errorw("unable to update message to slack channel",
			"channel", channel,
			"error", err,
		)
	}
}

// Post message to Slack wrapper that log errors
func postSlackMessage(api *slack.Client, channel string, options ...slack.MsgOption) string {
	_, timestamp, err := api.PostMessage(channel, options...)
	if err != nil {
		logger.Errorw("unable to post message to slack channel",
			"channel", channel,
			"error", err,
		)
	}
	return timestamp
}

// Notify To Slack
func notifySlackChannel(api *slack.Client, channel, msg string) {
	_, _, err := api.PostMessage(channel, slack.MsgOptionText(msg, false))
	if err != nil {
		logger.Errorw("unable to post message to slack channel",
			"channel", channel,
			"error", err,
		)
	}
}

func renderSlackCommandPayload(config *c) map[string]interface{} {
	return map[string]interface{}{
		"blocks": []slack.Block{
			slack.NewSectionBlock(
				&slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: ":waving: Select the project to release",
				},
				nil,
				slack.NewAccessory(
					slack.NewOptionsSelectBlockElement(
						slack.OptTypeStatic,
						&slack.TextBlockObject{
							Type: slack.PlainTextType,
							Text: "tech-ally projects",
						},
						SlackSelectedTechAllyProject,
						createOptionBlockObjects(config.ListProjects())...,
					),
				),
				slack.SectionBlockOptionBlockID(SlackTriggerTechAllyProject),
			),
		}}
}

// handleInteractiveEvent will take an Interactive Event and handle it properly
func handleInteractiveEvent(api *slack.Client, config *c, callback slack.InteractionCallback) error {

	switch callback.Type {
	case slack.InteractionTypeBlockActions:

		if callback.BlockActionState == nil {
			// we need the state of the action to know what to do
			// with it, else, we drop the message
			return errors.New("no block_action state field")
		}

		// from here, it is safe to call BlockActionState
		actions := callback.BlockActionState.Values
		for id, action := range actions {
			switch id {

			case SlackTriggerTechAllyProject:
				repo := action[SlackSelectedTechAllyProject].SelectedOption.Value
				go func() {
					if err := runCodefreshPipeline(api, config, callback, repo); err != nil {
						logger.Errorw("unable to run Codefresh pipeline",
							"error", err, "raw", callback)
					}
				}()

			case SlackSignLaceworkCLIGithubAction:
				mfaToken := action[SlackMfaTokenForGithubAction].Value
				tag, ok := callback.Message.Metadata.EventPayload["tag"]
				if !ok {
					logger.Errorw("unable to sign the Lacework CLI since 'tag' field was missing",
						"block_id", id, "raw", action)
					continue
				}

				go func() {
					if err := runGithubActionWithCallback(api, config, callback, mfaToken, tag.(string)); err != nil {
						logger.Errorw("unable to run Github workflow",
							"error", err, "raw", callback)
					}
				}()

			default:
				logger.Errorw("unknown or not yet implemented interactive block_id",
					"block_id", id, "raw", action)
			}
		}

	default:
		notifySlackChannel(api,
			config.NotifySlackChannel,
			fmt.Sprintf("Some weird type just showed up: *%s*", callback.Type),
		)
	}

	return nil
}

func formatAppMentionMsg(user, channel, text string) (msg string) {
	// Who
	if len(user) == 0 {
		msg = "Incoming webhook interacting with the release ally app! :woohoo:"
	} else {
		msg = fmt.Sprintf("User <@%s> is interacting with the release ally app! :woohoo:", user)
	}

	// What
	msg = fmt.Sprintf("%s\n\n*Message:*\n> %s", msg, text)

	// Where
	if len(channel) != 0 {
		msg = fmt.Sprintf("%s\n\n*Channel:* <#%s>", msg, channel)
	}

	return
}
