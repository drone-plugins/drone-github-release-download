// Copyright (c) 2020, the Drone Plugins project authors.
// Please see the AUTHORS file for details. All rights reserved.
// Use of this source code is governed by an Apache 2.0 license that can be
// found in the LICENSE file.

package main

import (
	"github.com/urfave/cli/v2"

	"github.com/drone-plugins/drone-github-release-download/plugin"
)

// settingsFlags has the cli.Flags for the plugin.Settings.
func settingsFlags(settings *plugin.Settings) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "github-url",
			Usage:       "github url, defaults to current scm",
			EnvVars:     []string{"PLUGIN_GITHUB_URL", "DRONE_REPO_LINK"},
			Destination: &settings.GitHubURL,
		},
		&cli.StringFlag{
			Name:        "api-key",
			Usage:       "api key to access github api",
			EnvVars:     []string{"PLUGIN_API_KEY", "GITHUB_RELEASE_DOWNLOAD_API_KEY", "GITHUB_TOKEN"},
			Destination: &settings.APIKey,
		},
		&cli.StringFlag{
			Name:        "owner",
			Usage:       "repository owner",
			EnvVars:     []string{"PLUGIN_OWNER"},
			Destination: &settings.Owner,
		},
		&cli.StringFlag{
			Name:        "name",
			Usage:       "repository name",
			EnvVars:     []string{"PLUGIN_NAME"},
			Destination: &settings.Name,
		},
		&cli.StringFlag{
			Name:        "tag",
			Usage:       "release tag",
			EnvVars:     []string{"PLUGIN_TAG"},
			Destination: &settings.Tag,
		},
		&cli.StringFlag{
			Name:        "path",
			Usage:       "path to place downloaded files",
			EnvVars:     []string{"PLUGIN_PATH"},
			Destination: &settings.Path,
		},
		&cli.StringSliceFlag{
			Name:        "files",
			Usage:       "list of files to download",
			EnvVars:     []string{"PLUGIN_FILES"},
			Destination: &settings.Files,
		},
	}
}
