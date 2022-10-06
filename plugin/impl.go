// Copyright (c) 2020, the Drone Plugins project authors.
// Please see the AUTHORS file for details. All rights reserved.
// Use of this source code is governed by an Apache 2.0 license that can be
// found in the LICENSE file.

package plugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v47/github"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
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
	Files     cli.StringSlice

	githubURL *url.URL
}

const latestTag = "latest"
const prereleaseTag = "prerelease"

// Validate handles the settings validation of the plugin.
func (p *Plugin) Validate() error {
	// Validate the config
	if p.settings.APIKey == "" {
		return fmt.Errorf("no api key provided")
	}

	if p.settings.Owner == "" {
		return fmt.Errorf("no repository owner provided")
	}

	if p.settings.Name == "" {
		return fmt.Errorf("no repository name provided")
	}

	uri, err := url.Parse(p.settings.GitHubURL)
	if err != nil {
		return fmt.Errorf("could not parse GitHub link")
	}
	// Remove the path in the case that DRONE_REPO_LINK was passed in
	uri.Path = ""
	p.settings.githubURL = uri

	if len(p.settings.Files.Value()) == 0 {
		return fmt.Errorf("no files specified")
	}

	// Set defaults
	if strings.TrimSpace(p.settings.Tag) == "" {
		p.settings.Tag = latestTag
	}

	return nil
}

// Execute provides the implementation of the plugin.
func (p *Plugin) Execute() error {
	// Create the client
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.settings.APIKey})
	tc := oauth2.NewClient(
		context.WithValue(context.Background(), oauth2.HTTPClient, p.network.Client),
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
	}).Debug("connecting to github instance")

	// Get the repository
	repo, _, err := client.Repositories.Get(p.network.Context, p.settings.Owner, p.settings.Name)
	if err != nil {
		return fmt.Errorf("error getting repository %s/%s: %w", p.settings.Owner, p.settings.Name, err)
	}

	logrus.WithFields(logrus.Fields{
		"fullname": repo.GetFullName(),
		"html-url": repo.GetHTMLURL(),
	}).Info("found repository")

	// Get the release
	release, err := p.getRelease(client)
	if err != nil {
		return fmt.Errorf("error getting release %s: %w", p.settings.Tag, err)
	}

	logrus.WithFields(logrus.Fields{
		"tag":          release.GetTagName(),
		"name":         release.GetName(),
		"published-at": release.GetPublishedAt(),
	}).Info("found release")

	// Determine if the source code tarball or zipball is being requested
	tarball := false
	zipball := false
	for _, file := range p.settings.Files.Value() {
		if strings.EqualFold("tarball", file) {
			tarball = true
		} else if strings.EqualFold("zipball", file) {
			zipball = true
		}
	}

	// Get the assets
	assets, err := p.getReleaseAssets(client, release)
	if err != nil && (!tarball && !zipball) {
		return fmt.Errorf("error getting release %s assets: %w", p.settings.Tag, err)
	}

	// Get the path to download to
	downloadPath, err := filepath.Abs(p.settings.Path)
	if err != nil {
		return fmt.Errorf("could not create download path %s: %w", p.settings.Path, err)
	}

	err = os.MkdirAll(downloadPath, os.ModeDir)
	if err != nil {
		return fmt.Errorf("could not create directory %s: %w", p.settings.Path, err)
	}

	logrus.WithField("path", downloadPath).Info("downloading assets to")

	// Start downloading the files
	for _, asset := range assets {
		name := asset.GetName()
		logrus.WithFields(logrus.Fields{
			"name":         name,
			"content-type": asset.GetContentType(),
			"created-at":   asset.GetCreatedAt(),
		}).Info("downloading asset")

		var rc io.ReadCloser
		rc, redirectURL, err := client.Repositories.DownloadReleaseAsset(
			p.network.Context,
			p.settings.Owner,
			p.settings.Name,
			asset.GetID(),
			p.network.Client,
		)
		if err != nil {
			return fmt.Errorf("error while downloading %s: %w", name, err)
		}

		// DownloadReleaseAsset either returns a io.ReadCloser or a redirect URL
		// if there is no error so it can be assumed that we have a URL
		if rc == nil {
			resp, err := p.network.Client.Get(redirectURL)
			if err != nil {
				return fmt.Errorf("error while downloading %s from %s: %w", name, redirectURL, err)
			}

			rc = resp.Body
		}
		defer rc.Close()

		err = writeAsset(rc, name, downloadPath)
		if err != nil {
			return err
		}
	}

	if tarball {
		p.downloadAssetFromURL(release.GetTarballURL(), "release.tar.gz", downloadPath)
		if err != nil {
			return fmt.Errorf("error while downloading tarball: %w", err)
		}
	}

	if zipball {
		p.downloadAssetFromURL(release.GetZipballURL(), "release.zip", downloadPath)
		if err != nil {
			return fmt.Errorf("error while downloading zipball: %w", err)
		}
	}

	return nil
}

func (p *Plugin) getReleaseAssets(client *github.Client, release *github.RepositoryRelease) ([]*github.ReleaseAsset, error) {
	// Iterate over release assets
	var assetsToDownload []*github.ReleaseAsset
	opt := &github.ListOptions{PerPage: 10}
	files := p.settings.Files.Value()
	fileCount := len(files)

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
			return nil, fmt.Errorf("error getting release asssets: %w", err)
		}

		for _, asset := range assets {
			assetName := asset.GetName()

			logrus.WithFields(logrus.Fields{
				"name":         assetName,
				"content-type": asset.GetContentType(),
				"created-at":   asset.GetCreatedAt(),
			}).Debug("found asset")

			for _, file := range files {
				match, err := filepath.Match(file, assetName)
				if err != nil {
					return nil, fmt.Errorf("error with file matching pattern %s: %w", file, err)
				}
				if match {
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

		for _, file := range files {
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

		return nil, fmt.Errorf("missing files in download %s", missing)
	}

	return assetsToDownload, nil
}

func (p *Plugin) getRelease(client *github.Client) (*github.RepositoryRelease, error) {
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

func (p *Plugin) getLatestPrerelease(client *github.Client) (*github.RepositoryRelease, error) {
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
			return nil, fmt.Errorf("error getting release asssets: %w", err)
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
		return nil, fmt.Errorf("could not find latest prerelease")
	}

	return release, nil
}

func (p *Plugin) downloadAssetFromURL(url, name, downloadPath string) error {
	logrus.WithFields(logrus.Fields{
		"name": name,
		"url":  url,
	}).Info("downloading asset")

	resp, err := p.network.Client.Get(url)
	if err != nil {
		return fmt.Errorf("error while downloading %s from %s: %w", name, url, err)
	}
	rc := resp.Body
	defer rc.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request for %s failed, status %s", url, http.StatusText(resp.StatusCode))
	}

	return writeAsset(rc, name, downloadPath)
}

func writeAsset(rc io.ReadCloser, name, downloadPath string) error {
	assetPath := filepath.Join(downloadPath, name)
	out, err := os.Create(assetPath)
	if err != nil {
		return fmt.Errorf("error creating file at %s: %w", assetPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	if err != nil {
		return fmt.Errorf("error while downloading file %s: %w", name, err)
	}

	fileInfo, err := out.Stat()
	if err != nil {
		return fmt.Errorf("error when getting file information for %s: %w", assetPath, err)
	}

	logrus.WithFields(logrus.Fields{
		"name":  name,
		"path":  assetPath,
		"bytes": fileInfo.Size(),
	}).Info("wrote asset")

	return nil
}
