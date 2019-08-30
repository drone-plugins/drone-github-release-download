package main

import (
	"context"
	"fmt"
	"io"

	"path/filepath"

	"net/http"
	"net/url"

	"os"

	"github.com/google/go-github/v28/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

type (
	Repo struct {
		Owner string
		Name  string
		Tag   string
	}

	Config struct {
		APIKey    string
		Files     []string
		Path      string
		GitHubURL string
	}

	Plugin struct {
		Repo   Repo
		Config Config
	}
)

func (p Plugin) Exec() error {
	if p.Config.APIKey == "" {
		return fmt.Errorf("You must provide an API key")
	}

	if p.Repo.Owner == "" {
		return fmt.Errorf("You must provide a repository owner")
	}

	if p.Repo.Name == "" {
		return fmt.Errorf("You must provide a repository name")
	}

	fileCount := len(p.Config.Files)

	if fileCount == 0 {
		return fmt.Errorf("No files specified")
	}

	// Parse base URL
	githubURL, err := url.Parse(p.Config.GitHubURL)

	if err != nil {
		return fmt.Errorf("Failed to parse GitHub URL. %s", err)
	}

	// Print out proxy information
	logrus.WithFields(logrus.Fields{
		"http-proxy": os.Getenv("HTTP_PROXY"),
		"no-proxy":   os.Getenv("NO_PROXY"),
	}).Debug("Proxy information")

	// Remove the path in the case that DRONE_REPO_LINK was passed in
	githubURL.Path = ""

	// Create the client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.Config.APIKey})
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	if githubURL.Hostname() != "github.com" {
		relBaseURL, _ := url.Parse("./api/v3/")
		relUploadURL, _ := url.Parse("./api/v3/upload/")

		client.BaseURL = githubURL.ResolveReference(relBaseURL)
		client.UploadURL = githubURL.ResolveReference(relUploadURL)
	}

	logrus.WithFields(logrus.Fields{
		"github-url": githubURL.String(),
		"base-url":   client.BaseURL.String(),
		"upload-url": client.BaseURL.String(),
	}).Debug("Connecting to GitHub instance")

	// Get the repository
	repo, _, err := client.Repositories.Get(ctx, p.Repo.Owner, p.Repo.Name)

	if err != nil {
		return fmt.Errorf("Error getting repository %s", err)
	}

	logrus.WithFields(logrus.Fields{
		"fullname": repo.GetFullName(),
		"html-url": repo.GetHTMLURL(),
	}).Info("Found repository")

	var release *github.RepositoryRelease

	// Get the release
	if p.Repo.Tag == "" || p.Repo.Tag == "latest" {
		logrus.Info("No tag specified, defaulting to latest release")

		release, _, err = client.Repositories.GetLatestRelease(ctx, p.Repo.Owner, p.Repo.Name)

		if err != nil {
			return fmt.Errorf("Error getting release %s", err)
		}
	} else {
		release, _, err = client.Repositories.GetReleaseByTag(ctx, p.Repo.Owner, p.Repo.Name, p.Repo.Tag)

		if err != nil {
			return fmt.Errorf("Error getting release %s", err)
		}
	}

	logrus.WithFields(logrus.Fields{
		"tag":          release.GetTagName(),
		"name":         release.GetName(),
		"published-at": release.GetPublishedAt(),
	}).Info("Found release")

	// Iterate over release assets
	var assetsToDownload []*github.ReleaseAsset
	opt := &github.ListOptions{PerPage: 10}

	for {
		logrus.WithFields(logrus.Fields{
			"page":     opt.Page,
			"per-page": opt.PerPage,
		}).Debug("Getting release asset page")

		assets, resp, err := client.Repositories.ListReleaseAssets(ctx, p.Repo.Owner, p.Repo.Name, release.GetID(), opt)

		if err != nil {
			return fmt.Errorf("Error getting release asssets %s", err)
		}

		for _, asset := range assets {
			assetName := asset.GetName()

			logrus.WithFields(logrus.Fields{
				"name":         assetName,
				"content-type": asset.GetContentType(),
				"created-at":   asset.GetCreatedAt(),
			}).Debug("Found asset")

			for _, file := range p.Config.Files {
				if file == assetName {
					assetsToDownload = append(assetsToDownload, asset)

					if len(assetsToDownload) == fileCount {
						logrus.Debug("All assets found")
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

		for _, file := range p.Config.Files {
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

		return fmt.Errorf("Missing files in download %s", missing)
	}

	// Get the path to download to
	downloadPath, err := filepath.Abs(p.Config.Path)

	if err != nil {
		return fmt.Errorf("Could not create download path %s", err)
	}
	err = os.MkdirAll(downloadPath, os.ModeDir)

	if err != nil {
		return fmt.Errorf("Could not create directory to download to")
	}

	logrus.WithFields(logrus.Fields{
		"path": downloadPath,
	}).Info("Downloading assets to")

	// Start downloading the files
	for _, asset := range assetsToDownload {
		name := asset.GetName()
		logrus.WithFields(logrus.Fields{
			"name":         name,
			"content-type": asset.GetContentType(),
			"created-at":   asset.GetCreatedAt(),
		}).Info("Downloading asset")

		var rc io.ReadCloser
		rc, redirectURL, err := client.Repositories.DownloadReleaseAsset(ctx, p.Repo.Owner, p.Repo.Name, asset.GetID())
		if err != nil {
			return fmt.Errorf("Error while downloading %s", err)
		}

		// DownloadReleaseAsset either returns a io.ReadCloser or a redirect URL
		// if there is no error so it can be assumed that we have a URL
		if rc == nil {
			resp, err := http.Get(redirectURL)

			if err != nil {
				return fmt.Errorf("Error while downloading %s", err)
			}

			rc = resp.Body
		}
		defer rc.Close()

		assetPath := filepath.Join(downloadPath, asset.GetName())
		out, err := os.Create(assetPath)
		if err != nil {
			return fmt.Errorf("Error creating file %s", err)
		}
		defer out.Close()

		_, err = io.Copy(out, rc)
		if err != nil {
			return fmt.Errorf("Error while downloading file %s", err)
		}

		fileInfo, err := out.Stat()

		if err != nil {
			return fmt.Errorf("Error when getting file information %s", err)
		}

		logrus.WithFields(logrus.Fields{
			"name":  name,
			"path":  assetPath,
			"bytes": fileInfo.Size(),
		}).Info("Downloaded asset")
	}

	return nil
}
