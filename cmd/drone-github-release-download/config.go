// Copyright (c) 2019, the Drone Plugins project authors.
// Please see the AUTHORS file for details. All rights reserved.
// Use of this source code is governed by an Apache 2.0 license that can be
// found in the LICENSE file.

package main

import (
	"github.com/urfave/cli/v2"

	"github.com/drone-plugins/drone-github-release-download/pkg/github"
)

const (
	githubURLFlag = "github-url"
	apiKeyFlag    = "api-key"
	ownerFlag     = "owner"
	nameFlag      = "name"
	tagFlag       = "tag"
	pathFlag      = "path"
	filesFlag     = "files"
)

// settingsFlags has the cli.Flags for the plugin.Settings.
func settingsFlags() []cli.Flag {
	// Replace below with all the flags required for the plugin's specific
	// settings.
	return []cli.Flag{
		&cli.StringFlag{
			Name:    githubURLFlag,
			Usage:   "github url, defaults to current scm",
			EnvVars: []string{"PLUGIN_GITHUB_URL", "DRONE_REPO_LINK"},
		},
		&cli.StringFlag{
			Name:    apiKeyFlag,
			Usage:   "api key to access github api",
			EnvVars: []string{"PLUGIN_API_KEY,GITHUB_RELEASE_DOWNLOAD_API_KEY,GITHUB_TOKEN"},
		},
		&cli.StringFlag{
			Name:    ownerFlag,
			Usage:   "repository owner",
			EnvVars: []string{"PLUGIN_OWNER"},
		},
		&cli.StringFlag{
			Name:    nameFlag,
			Usage:   "repository name",
			EnvVars: []string{"PLUGIN_NAME"},
		},
		&cli.StringFlag{
			Name:    tagFlag,
			Usage:   "release tag",
			EnvVars: []string{"PLUGIN_TAG"},
		},
		&cli.StringFlag{
			Name:    pathFlag,
			Usage:   "path to place downloaded files",
			EnvVars: []string{"PLUGIN_PATH"},
		},
		&cli.StringSliceFlag{
			Name:    filesFlag,
			Usage:   "list of files to download",
			EnvVars: []string{"PLUGIN_FILES"},
		},
	}
}

// settingsFromContext creates a plugin.Settings from the cli.Context.
func settingsFromContext(ctx *cli.Context) github.Settings {
	return github.Settings{
		GitHubURL: ctx.String(githubURLFlag),
		APIKey:    ctx.String(apiKeyFlag),
		Owner:     ctx.String(ownerFlag),
		Name:      ctx.String(nameFlag),
		Tag:       ctx.String(tagFlag),
		Path:      ctx.String(pathFlag),
		Files:     ctx.StringSlice(filesFlag),
	}
}
