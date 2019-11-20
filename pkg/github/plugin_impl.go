// Copyright (c) 2019, the Drone Plugins project authors.
// Please see the AUTHORS file for details. All rights reserved.
// Use of this source code is governed by an Apache 2.0 license that can be
// found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v28/github"
	"github.com/mitchellh/ioprogress"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// Settings for the Plugin.
type Settings struct {
	GitHubURL string
	APIKey    string
	Owner     string
	Name      string
	Tag       string
	Path      string
	Files     []string

	githubURL *url.URL
}

const latestTag = "latest"
const prereleaseTag = "prerelease"

func (p *pluginImpl) Validate() error {
	// Validate the config
	if p.settings.APIKey == "" {
		return errors.New("no api key provided")
	}

	if p.settings.Owner == "" {
		return errors.New("no repository owner provided")
	}

	if p.settings.Name == "" {
		return errors.New("no repository name provided")
	}

	uri, err := url.Parse(p.settings.GitHubURL)
	if err != nil {
		return errors.New("could not parse GitHub link")
	}
	// Remove the path in the case that DRONE_REPO_LINK was passed in
	uri.Path = ""
	p.settings.githubURL = uri

	if len(p.settings.Files) == 0 {
		return errors.New("no files specified")
	}

	// Set defaults
	if strings.TrimSpace(p.settings.Tag) == "" {
		p.settings.Tag = latestTag
	}

	return nil
}

func (p *pluginImpl) Exec() error {
	// Create the client
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.settings.APIKey})
	tc := oauth2.NewClient(
		context.WithValue(oauth2.NoContext, oauth2.HTTPClient, p.network.Client),
		ts,
	)

	client := github.NewClient(tc)

	if p.settings.githubURL.Hostname() != "github.com" {
		relBaseURL, _ := url.Parse("./api/v3/")
		relUploadURL, _ := url.Parse("./api/v3/upload/")

		client.BaseURL = p.settings.githubURL.ResolveReference(relBaseURL)
		client.UploadURL = p.settings.githubURL.ResolveReference(relUploadURL)
	}

	logrus.WithFields(logrus.Fields{
		"github-url": p.settings.githubURL.String(),
		"base-url":   client.BaseURL.String(),
		"upload-url": client.BaseURL.String(),
	}).Debug("Connecting to GitHub instance")

	// Get the repository
	repo, _, err := client.Repositories.Get(p.network.Context, p.settings.Owner, p.settings.Name)
	if err != nil {
		return errors.Wrapf(err, "Error getting repository %s/%s", p.settings.Owner, p.settings.Name)
	}

	logrus.WithFields(logrus.Fields{
		"fullname": repo.GetFullName(),
		"html-url": repo.GetHTMLURL(),
	}).Info("found repository")

	// Get the release
	release, err := p.getRelease(client)
	if err != nil {
		return errors.Wrapf(err, "Error getting release %s", p.settings.Tag)
	}

	logrus.WithFields(logrus.Fields{
		"tag":          release.GetTagName(),
		"name":         release.GetName(),
		"published-at": release.GetPublishedAt(),
	}).Info("found release")

	// Get the assets
	assets, err := p.getReleaseAssets(client, release)
	if err != nil {
		return errors.Wrapf(err, "Error getting release %s assets", p.settings.Tag)
	}

	// Get the path to download to
	downloadPath, err := filepath.Abs(p.settings.Path)
	if err != nil {
		return errors.Wrapf(err, "Could not create download path %s", p.settings.Path)
	}

	err = os.MkdirAll(downloadPath, os.ModeDir)
	if err != nil {
		return errors.Wrapf(err, "Could not create directory %s", p.settings.Path)
	}

	logrus.WithField("path", downloadPath).Info("downloading assets to")

	// Start downloading the files
	for _, asset := range assets {
		name := asset.GetName()
		logrus.WithFields(logrus.Fields{
			"name":         name,
			"content-type": asset.GetContentType(),
			"created-at":   asset.GetCreatedAt(),
		}).Info("Downloading asset")

		var rc io.ReadCloser
		rc, redirectURL, err := client.Repositories.DownloadReleaseAsset(
			p.network.Context,
			p.settings.Owner,
			p.settings.Name,
			asset.GetID(),
		)
		if err != nil {
			return errors.Wrapf(err, "Error while downloading %s", name)
		}

		// DownloadReleaseAsset either returns a io.ReadCloser or a redirect URL
		// if there is no error so it can be assumed that we have a URL
		if rc == nil {
			resp, err := p.network.Client.Get(redirectURL)

			if err != nil {
				return errors.Wrapf(err, "Error while downloading %s from %s", name, redirectURL)
			}

			rc = resp.Body
		}
		defer rc.Close()

		assetPath := filepath.Join(downloadPath, asset.GetName())
		out, err := os.Create(assetPath)
		if err != nil {
			return errors.Wrapf(err, "Error creating file at %s", assetPath)
		}
		defer out.Close()

		bar := ioprogress.DrawTextFormatBar(20)
		drawFunc := ioprogress.DrawTerminalf(os.Stdout, func(progress, total int64) string {
			return fmt.Sprintf("%s %s %20s", name, bar(progress, total), ioprogress.DrawTextFormatBytes(progress, total))
		})
		rp := &ioprogress.Reader{
			Reader:       rc,
			DrawInterval: 5 * time.Second,
			Size:         int64(asset.GetSize()),
			DrawFunc:     drawFunc,
		}

		_, err = io.Copy(out, rp)
		if err != nil {
			return errors.Wrapf(err, "Error while downloading file %s", name)
		}

		fileInfo, err := out.Stat()
		if err != nil {
			return errors.Wrapf(err, "Error when getting file information for %s", assetPath)
		}

		logrus.WithFields(logrus.Fields{
			"name":  name,
			"path":  assetPath,
			"bytes": fileInfo.Size(),
		}).Info("Downloaded asset")
	}

	return nil
}

func (p *pluginImpl) getReleaseAssets(client *github.Client, release *github.RepositoryRelease) ([]*github.ReleaseAsset, error) {
	// Iterate over release assets
	var assetsToDownload []*github.ReleaseAsset
	opt := &github.ListOptions{PerPage: 10}
	fileCount := len(p.settings.Files)

	for {
		logrus.WithFields(logrus.Fields{
			"page":     opt.Page,
			"per-page": opt.PerPage,
		}).Debug("getting release asset page")

		assets, resp, err := client.Repositories.ListReleaseAssets(
			p.network.Context,
			p.settings.Owner,
			p.settings.Name,
			release.GetID(),
			opt,
		)

		if err != nil {
			return nil, errors.Wrap(err, "Error getting release asssets")
		}

		for _, asset := range assets {
			assetName := asset.GetName()

			logrus.WithFields(logrus.Fields{
				"name":         assetName,
				"content-type": asset.GetContentType(),
				"created-at":   asset.GetCreatedAt(),
			}).Debug("Found asset")

			for _, file := range p.settings.Files {
				if file == assetName {
					assetsToDownload = append(assetsToDownload, asset)

					if len(assetsToDownload) == fileCount {
						logrus.Debug("all assets found")
						break
					}
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	// Error checking for assets
	if len(assetsToDownload) != fileCount {
		var missing []string

		for _, file := range p.settings.Files {
			found := false

			for _, asset := range assetsToDownload {
				if file == asset.GetName() {
					found = true
					break
				}
			}

			if !found {
				missing = append(missing, file)
			}
		}

		return nil, errors.Errorf("Missing files in download %s", missing)
	}

	return assetsToDownload, nil
}

func (p *pluginImpl) getRelease(client *github.Client) (*github.RepositoryRelease, error) {
	logrus.WithField("tag", p.settings.Tag).Info("retrieving release")

	var release *github.RepositoryRelease
	var err error

	tag := p.settings.Tag

	if tag == latestTag {
		logrus.Debug("getting latest release")
		release, _, err = client.Repositories.GetLatestRelease(p.network.Context, p.settings.Owner, p.settings.Name)
	} else if tag == prereleaseTag {
		logrus.Debug("getting latest prerelease")
		release, err = p.getLatestPrerelease(client)
	} else {
		logrus.Debug("getting release by tag")
		release, _, err = client.Repositories.GetReleaseByTag(p.network.Context, p.settings.Owner, p.settings.Name, p.settings.Tag)
	}

	return release, err
}

func (p *pluginImpl) getLatestPrerelease(client *github.Client) (*github.RepositoryRelease, error) {
	var release *github.RepositoryRelease
	opt := &github.ListOptions{PerPage: 10}

	for {
		logrus.WithFields(logrus.Fields{
			"page":     opt.Page,
			"per-page": opt.PerPage,
		}).Debug("getting release listing page")

		releases, resp, err := client.Repositories.ListReleases(
			p.network.Context,
			p.settings.Owner,
			p.settings.Name,
			opt,
		)

		if err != nil {
			return nil, errors.Wrap(err, "Error getting release asssets")
		}

		for _, r := range releases {
			if r.GetPrerelease() {
				if release == nil {
					release = r
				} else if r.GetPublishedAt().After(release.GetPublishedAt().Time) {
					release = r
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	if release == nil {
		return nil, errors.Errorf("Could not find latest prerelease")
	}

	return release, nil
}
