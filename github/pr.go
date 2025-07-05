package github

import (
	"errors"
	"strings"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"

	"github.com/serenibyss/nhprtracker/auth"
	"github.com/serenibyss/nhprtracker/internal"
)

// GatherMergedPRs returns a map of all pull requests merged to specific repos after a specified date.
func GatherMergedPRs(client *auth.GithubClient, repos []*github.Repository) (map[string][]*github.PullRequest, error) {
	prMap := make(map[string][]*github.PullRequest)
	var hadError bool

	for _, repo := range repos {
		prs, err := gatherMergedPRsForRepo(client, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to list pull requests for repo %s/%s: %v", client.Org, repo.GetName(), err)
			hadError = true
		}
		if prs != nil {
			zap.S().Named("github").Debugf("found %d PRs for repo %s", len(prs), repo.GetName())
			prMap[client.Org+"/"+repo.GetName()] = prs
		}
	}

	if hadError {
		return prMap, errors.New("some repo PR lists could not be checked, see logs above")
	}
	return prMap, nil
}

// gatherMergedPRsForRepo gathers all PRs merged before a specified date for a specified repository.
func gatherMergedPRsForRepo(client *auth.GithubClient, repo *github.Repository) ([]*github.PullRequest, error) {
	var prList []*github.PullRequest
	repoName := repo.GetName()
	opts := &github.PullRequestListOptions{
		State:     "closed",
		Sort:      "updated",
		Direction: "desc",
	}

	for {
		prs, resp, err := client.PullRequests.List(client.Ctx, client.Org, repoName, opts)
		if err != nil {
			return prList, err
		}

		zap.S().Named("github").Debugf("found page with %d PRs for repo %s", len(prs), repoName)

		for _, pr := range prs {
			if pr.GetMergedAt().Equal(github.Timestamp{}) {
				continue
			}

			if pr.GetMergedAt().Before(client.Date) {
				return prList, nil
			}

			if !prTitleCheck(pr) {
				continue
			}

			zap.S().Debugf("found pr #%d (%s) for repo %s", pr.GetNumber(), pr.GetTitle(), repoName)
			prList = append(prList, pr)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return prList, nil
}

func prTitleCheck(pr *github.PullRequest) bool {
	for _, excl := range internal.ExcludedPRTitles {
		if strings.Contains(pr.GetTitle(), excl) {
			return false
		}
	}
	return true
}
