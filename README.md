# drone-github-release-download

[![Build Status](http://harness.drone.io/api/badges/drone-plugins/drone-github-release-download/status.svg)](http://harness.drone.io/drone-plugins/drone-github-release-download)
[![Slack](https://img.shields.io/badge/slack-drone-orange.svg?logo=slack)](https://join.slack.com/t/harnesscommunity/shared_invite/zt-y4hdqh7p-RVuEQyIl5Hcx4Ck8VCvzBw)
[![Join the discussion at https://community.harness.io](https://img.shields.io/badge/discourse-forum-orange.svg)](https://community.harness.io)
[![Drone questions at https://stackoverflow.com](https://img.shields.io/badge/drone-stackoverflow-orange.svg)](https://stackoverflow.com/questions/tagged/drone.io)
[![Go Doc](https://godoc.org/github.com/drone-plugins/drone-github-release-download?status.svg)](http://godoc.org/github.com/drone-plugins/drone-github-release-download)
[![Go Report](https://goreportcard.com/badge/github.com/drone-plugins/drone-github-release-download)](https://goreportcard.com/report/github.com/drone-plugins/drone-github-release-download)

Drone plugin for downloading Github Releases.

## Build

Build the binary with the following command:

```bash
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
export GO111MODULE=on

go build -v -a -tags netgo -o release/linux/amd64/drone-github-release-download ./cmd/drone-github-release-download
```

## Docker

Build the Docker image with the following command:

```bash
docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/Dockerfile.linux.amd64 --tag plugins/github-release-download .
```

## Usage

```bash
docker run --rm \
  -e PLUGIN_API_KEY=${HOME}/.ssh/id_rsa \
  -e PLUGIN_OWNER=octocat \
  -e PLUGIN_NAME=foo \
  -e PLUGIN_FILES=foo.zip \
  -v $(pwd):$(pwd) \
  -w $(pwd) \
  plugins/github-release-download
```
