package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

const SlackTriggerTechAllyProject = "trigger_tech_ally_project"

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

		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				logger.Infow("event ignored", "type", evt.Type)
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

			switch callback.Type {
			case slack.InteractionTypeBlockActions:
				go func() {
					if err := runCodefreshPipeline(api, config, callback); err != nil {
						logger.Errorw("unable to run codefresh pipeline",
							"error", err.Error(), "raw", callback)
					}
				}()

			default:
				notifySlackChannel(api,
					config.NotifySlackChannel,
					fmt.Sprintf("Some weird type just showed up: *%s*", callback.Type),
				)
			}

			var payload interface{}
			client.Ack(*evt.Request, payload)

		default:
			logger.Warnw("unexpected event type received", "type", evt.Type, "raw", evt)
		}

	}
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
			"error", err.Error(),
		)
	}
}

// Post message to Slack wrapper that log errors
func postSlackMessage(api *slack.Client, channel string, options ...slack.MsgOption) string {
	_, timestamp, err := api.PostMessage(channel, options...)
	if err != nil {
		logger.Errorw("unable to post message to slack channel",
			"channel", channel,
			"error", err.Error(),
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
			"error", err.Error(),
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
						SlackTriggerTechAllyProject,
						createOptionBlockObjects(config.ListProjects())...,
					),
				),
			),
		}}
}
