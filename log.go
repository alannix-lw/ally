package main

import (
	"os"

	"github.com/lacework/go-sdk/lwlogger"
)

var logger = lwlogger.New("INFO").Sugar()

func init() {
	if debug() {
		logger = lwlogger.New("DEBUG").Sugar()
	}
}

func debug() bool {
	if os.Getenv("DEBUG") != "" {
		return true
	}
	return false
}
