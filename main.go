package main

func main() {
	// load config file ally.toml
	config, err := LoadConfig(*cFlag)
	if err != nil {
		logger.Fatalw("unable to load config", "error", err.Error())
	}

	// verify if the codefresh CLI is installed
	if !codefreshCLIExists() {
		logger.Fatalw("missing dependency", "bin", "codefresh")
	}

	// verify that there is a codefresh config on disk
	// if there is not one, try to configure it
	if err := config.verifyCodefreshConfig(); err != nil {
		logger.Fatalw("unable to configure the codefresh CLI",
			"error", err.Error(),
		)
	}

	// connecto to Slack
	client, api, err := connectToSlackViaSocketmode()
	if err != nil {
		logger.Fatalw("unable to connect to slack", "error", err.Error())
	}

	// goroutine to listen to Slack events
	go listenToSlackEvents(client, api, config)

	client.Run()
}
