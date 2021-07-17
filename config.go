package main

import (
	"flag"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

var cFlag *string

func init() {
	cFlag = flag.String("c", "ally.toml", "path to TOML config file")
	flag.Parse()
}

type c struct {
	NotifySlackChannel string `toml:"notify_slack_channel"`
	CodefreshCfg       string `toml:"codefresh_config,omitempty"`
	Projects           []struct {
		Repository string   `toml:"repository"`
		Pipeline   string   `toml:"pipeline"`
		Variables  []string `toml:"variables,omitempty"`
	} `toml:"project"`
}

//
// Example config
//
// ```toml
// notify_slack_channel = "C011B98EA5U"
// codefresh_config = "/foo/bar/.cfconfig"
//
// [[project]]
// repository = "go-sdk"
// pipeline = "go-sdk/prepare-release"
//
// [[project]]
// repository = "terraform-provider-lacework"
// pipeline = "terraform-provider-lacework/prepare-release"
//
// [[project]]
// repository = "terraform-gcp-config"
// pipeline  = "terraform-modules/prepare-release-for"
// variables = ["TF_MODULE=terraform-gcp-config"]
//
// [[project]]
// repository = "terraform-aws-ecr"
// pipeline  = "terraform-modules/prepare-release-for"
// variables = ["TF_MODULE=terraform-aws-ecr"]
// ```

func LoadConfig(f string) (*c, error) {
	logger.Infow("loading config", "path", f)

	var config c
	if _, err := toml.DecodeFile(f, &config); err != nil {
		return nil, errors.Wrapf(err, "unable to decode config %s", f)
	}

	for _, p := range config.Projects {
		logger.Debugw("project loaded",
			"repository", p.Repository,
			"pipeline", p.Pipeline,
			"variables", p.Variables,
		)
	}
	return &config, nil
}

func (config *c) ListProjects() []string {
	out := []string{}
	for _, p := range config.Projects {
		out = append(out, p.Repository)
	}
	return out
}
