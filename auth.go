package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type GithubClient struct {
	*github.Client

	ctx    context.Context
	org    string
	branch string
	repos  []string
	date   time.Time
}

func GetClient(org string, branch string, repos []string, timestamp time.Time, token string) (*GithubClient, error) {
	token = getToken(token)
	if token != "" {
		return nil, errors.New("could not find GITHUB_TOKEN environment variable")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	zap.S().Named("auth").Infof("Organization: %s", org)
	zap.S().Named("auth").Infof("Release Branch Name: %s", branch)
	if len(repos) > 0 {
		zap.S().Named("auth").Infof("Specific Repos: %v", repos)
	}
	zap.S().Named("auth").Infof("PRs After Date: %s", timestamp.Format(time.RFC3339))

	return &GithubClient{
		Client: client,
		ctx:    ctx,
		org:    org,
		branch: branch,
		repos:  repos,
		date:   timestamp,
	}, nil
}

func getToken(token string) string {
	if token != "" {
		return token
	}

	token = os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return token
	}

	switch runtime.GOOS {
	case "darwin":
		fallthrough
	case "windows":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		fi, err := os.Open(filepath.Join(homeDir, ".github_personal_token"))
		if err != nil {
			return ""
		}
		data, err := io.ReadAll(fi)
		if err != nil {
			return ""
		}
		return string(data)
	case "linux":
		fi, err := os.Open(filepath.Join("/", ".github_personal_token"))
		if err != nil {
			return ""
		}
		data, err := io.ReadAll(fi)
		if err != nil {
			return ""
		}
		return string(data)
	default:
		return ""
	}
}
