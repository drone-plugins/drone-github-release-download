package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	version = "unknown"
)

func main() {
	app := cli.NewApp()
	app.Name = "github-release-download plugin"
	app.Usage = "github-release-download plugin"
	app.Action = run
	app.Version = version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "api-key",
			Usage:  "api key to access github api",
			EnvVar: "PLUGIN_API_KEY,GITHUB_RELEASE_DOWNLOAD_API_KEY,GITHUB_TOKEN",
		},
		cli.StringSliceFlag{
			Name:   "files",
			Usage:  "list of files to download",
			EnvVar: "PLUGIN_FILES",
		},
		cli.StringFlag{
			Name:   "path",
			Usage:  "path to place downloaded files",
			EnvVar: "PLUGIN_PATH",
		},
		cli.StringFlag{
			Name:   "github-url",
			Usage:  "github url, defaults to current scm",
			EnvVar: "PLUGIN_GITHUB_URL,DRONE_REPO_LINK",
		},
		cli.StringFlag{
			Name:   "owner",
			Usage:  "repository owner",
			EnvVar: "PLUGIN_OWNER",
		},
		cli.StringFlag{
			Name:   "name",
			Usage:  "repository name",
			EnvVar: "PLUGIN_NAME",
		},
		cli.StringFlag{
			Name:   "tag",
			Usage:  "release tag",
			EnvVar: "PLUGIN_TAG",
		},
		cli.StringFlag{
			Name:   "log-level",
			Value:  "info",
			Usage:  "plugin logging level",
			EnvVar: "PLUGIN_LOG_LEVEL",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	// Parse into structs
	plugin := Plugin{
		Repo: Repo{
			Owner: c.String("owner"),
			Name:  c.String("name"),
			Tag:   c.String("tag"),
		},
		Config: Config{
			APIKey:    c.String("api-key"),
			Files:     c.StringSlice("files"),
			Path:      c.String("path"),
			GitHubURL: c.String("github-url"),
		},
	}

	// Get log level
	lvl, err := logrus.ParseLevel(c.String("log-level"))

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"log-level": c.String("log-level"),
		}).Info("Invalid log level defaulting to 'info'")

		lvl = logrus.InfoLevel
	}

	logrus.SetLevel(lvl)
	logrus.WithFields(logrus.Fields{
		"log-level": lvl,
	}).Info("Logging level set to")

	// Execute plugin
	return plugin.Exec()
}
