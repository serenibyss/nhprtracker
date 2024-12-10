package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"

	"github.com/serenibyss/nhprtracker/internal"
)

// GatherRepositories gathers all repositories on the specified organization
func GatherRepositories(client *GithubClient) ([]*github.Repository, error) {
	if client.repos != nil {
		return gatherSpecificRepositories(client)
	}

	var cleansedRepos []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(client.ctx, client.org, opts)
		if err != nil {
			zap.S().Named("github").Errorf("failed to fetch some repositories: %v", err)
			return nil, err
		}

		for _, repo := range repos {
			// remove archived repos
			if repo.GetArchived() {
				continue
			}

			// remove untracked repositories
			name := repo.GetName()
			if name == "" || slices.Contains(internal.ExcludedRepositories, repo.GetName()) {
				continue
			}

			// remove repos with no updates after our specified date
			if repo.GetPushedAt().Before(client.date) {
				continue
			}

			zap.S().Named("github").Debugf("found repo %s", repo.GetName())
			cleansedRepos = append(cleansedRepos, repo)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return cleansedRepos, nil
}

func gatherSpecificRepositories(client *GithubClient) ([]*github.Repository, error) {
	var repositories []*github.Repository
	var hadError bool

	for _, repo := range client.repos {
		repository, _, err := client.Repositories.Get(client.ctx, client.org, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to get repo with name %s/%s: %v", client.org, repo, err)
			hadError = true
			continue
		}

		zap.S().Named("github").Debugf("found repo %s", repository.GetName())
		repositories = append(repositories, repository)
	}

	if hadError {
		return repositories, errors.New("some repos could not be found, see logs above")
	}
	return repositories, nil
}

// GatherReleaseRepositories gathers all repositories with a branch matching
// the specified release branch from a provided set of repositories.
func GatherReleaseRepositories(client *GithubClient, repos []*github.Repository) (map[string]*github.Repository, error) {
	patchRepos := map[string]*github.Repository{}
	var hadError bool

	for _, repo := range repos {
		hasBranch, err := checkForReleaseBranch(client, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to list branches for repo %s/%s: %v", client.org, repo.GetName(), err)
			hadError = true
		}

		if hasBranch {
			zap.S().Named("github").Debugf("found patch repo %s", repo.GetName())
			patchRepos[client.org+"/"+repo.GetName()] = repo
		}
	}
	if hadError {
		return patchRepos, errors.New("some repo branches could not be checked, see logs above")
	}
	return patchRepos, nil
}

// checkForReleaseBranch checks for if the specified repository has a branch matching the client option.
func checkForReleaseBranch(client *GithubClient, repo *github.Repository) (bool, error) {
	opts := &github.BranchListOptions{}
	repoName := repo.GetName()

	for {
		branches, resp, err := client.Repositories.ListBranches(client.ctx, client.org, repoName, opts)
		if err != nil {
			return false, err
		}

		for _, branch := range branches {
			if branch.GetName() == client.branch {
				zap.S().Named("github").Debugf("found repo with branch %s: %s", client.branch, repoName)
				return true, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return false, nil
}

// GatherMergedPRs returns a map of all pull requests merged to specific repos after a specified date.
func GatherMergedPRs(client *GithubClient, repos []*github.Repository) (map[string][]*github.PullRequest, error) {
	prMap := make(map[string][]*github.PullRequest)
	var hadError bool

	for _, repo := range repos {
		prs, err := gatherMergedPRsForRepo(client, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to list pull requests for repo %s/%s: %v", client.org, repo.GetName(), err)
			hadError = true
		}
		if prs != nil {
			zap.S().Named("github").Debugf("found %d PRs for repo %s", len(prs), repo.GetName())
			prMap[client.org+"/"+repo.GetName()] = prs
		}
	}

	if hadError {
		return prMap, errors.New("some repo PR lists could not be checked, see logs above")
	}
	return prMap, nil
}

// gatherMergedPRsForRepo gathers all PRs merged before a specified date for a specified repository.
func gatherMergedPRsForRepo(client *GithubClient, repo *github.Repository) ([]*github.PullRequest, error) {
	var prList []*github.PullRequest
	repoName := repo.GetName()
	opts := &github.PullRequestListOptions{
		State:     "closed",
		Sort:      "updated",
		Direction: "desc",
	}

	for {
		prs, resp, err := client.PullRequests.List(client.ctx, client.org, repoName, opts)
		if err != nil {
			return prList, err
		}

		zap.S().Named("github").Debugf("found page with %d PRs for repo %s", len(prs), repoName)

		for _, pr := range prs {
			if pr.GetMergedAt().Equal(github.Timestamp{}) {
				continue
			}

			if pr.GetMergedAt().Before(client.date) {
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

func FilterMatchingCommitsOnBranch(client *GithubClient, prMap map[string][]*github.PullRequest, releaseRepos map[string]*github.Repository) (map[string][]*github.PullRequest, error) {
	newPrMap := map[string][]*github.PullRequest{}
	var hadError bool
	for repoName, prs := range prMap {
		releaseRepo := releaseRepos[repoName]

		// Repository does not have a matching branch, so all PRs are valid to check
		if releaseRepo == nil {
			zap.S().Named("github").Debugf("no release branch for repo %s, all PRs valid", repoName)
			newPrMap[repoName] = prs
			continue
		}

		// Gather all commits on the release branch after a specified date
		commits, err := gatherCommitsToCheck(client, releaseRepo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to filter PRs: %v", err)
			hadError = true
			continue
		}

		var newPrs []*github.PullRequest
		for _, pr := range prs {
			var found bool
			for _, commit := range commits {
				message := commit.GetMessage()
				if strings.Contains(message, fmt.Sprintf("(#%d)", pr.GetNumber())) {
					zap.S().Named("github").Debugf("found matching release branch commit for PR #%d on repo %s", pr.GetNumber(), repoName)
					found = true
					break
				}
			}
			if !found {
				newPrs = append(newPrs, pr)
			}
		}
		if len(newPrs) != 0 {
			zap.S().Named("github").Debugf("found %d PRs on repo %s not included in release branch", len(newPrs), repoName)
			newPrMap[repoName] = newPrs
		} else {
			zap.S().Named("github").Debugf("all PRs on repo %s included in release branch, skipping", repoName)
		}
	}

	if hadError {
		return newPrMap, errors.New("failed to filter PRs for some repos, see log above")
	}
	return newPrMap, nil
}

func gatherCommitsToCheck(client *GithubClient, repo *github.Repository) ([]*github.Commit, error) {
	var allCommits []*github.Commit
	opts := &github.CommitsListOptions{
		SHA:   client.branch,
		Since: client.date,
	}

	for {
		commits, resp, err := client.Repositories.ListCommits(client.ctx, client.org, repo.GetName(), opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list commits for repo %s: %w", repo.GetName(), err)
		}

		zap.S().Named("github").Debugf("found %d commits on branch %s for repo %s", len(commits), client.branch, repo.GetName())
		for _, commit := range commits {
			allCommits = append(allCommits, commit.GetCommit())
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allCommits, nil
}
