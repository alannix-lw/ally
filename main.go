package main

func main() {
	// load config file ally.toml
	config, err := LoadConfig(*cFlag)
	if err != nil {
		logger.Fatalw("unable to load config", "error", err.Error())
	}

	// validate environment
	validateEnvironment(config)

	// connec to to Slack
	client, api, err := connectToSlackViaSocketmode()
	if err != nil {
		logger.Fatalw("unable to connect to slack", "error", err.Error())
	}

	// goroutine to listen to Slack events
	go listenToSlackEvents(client, api, config)

	if err := client.Run(); err != nil {
		logger.Fatalw("unable to run ally Slack app", "error", err.Error())
	}
}

func validateEnvironment(config *c) {
	// verify if the codefresh CLI is installed
	if !codefreshCLIExists() {
		logger.Fatalw("missing dependency", "bin", "codefresh")
	}

	// verify if the Github CLI is installed
	if !githubCLIExists() {
		logger.Fatalw("missing dependency", "bin", "gh")
	}

	// verify that there is a codefresh config on disk
	// if there is not one, try to configure it
	if err := config.verifyCodefreshConfig(); err != nil {
		logger.Fatalw("unable to configure the Codefresh CLI",
			"error", err.Error(),
		)
	}

	// verify that the Github CLI is configured via environment variable
	if err := config.verifyGithubCLIConfig(); err != nil {
		logger.Fatalw("Github CLI not configured", "error", err.Error())
	}
}
