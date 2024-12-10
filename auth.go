package main

import (
	"context"
	"errors"
	"os"
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
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return nil, errors.New("could not find GITHUB_TOKEN environment variable")
		}
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
