package github

import (
	"errors"
	"fmt"
	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"
	"strings"

	"github.com/serenibyss/nhprtracker/auth"
)

// checkForReleaseBranch checks for if the specified repository has a branch matching the client option.
func checkForReleaseBranch(client *auth.GithubClient, repo *github.Repository) (bool, error) {
	opts := &github.BranchListOptions{}
	repoName := repo.GetName()

	for {
		branches, resp, err := client.Repositories.ListBranches(client.Ctx, client.Org, repoName, opts)
		if err != nil {
			return false, err
		}

		for _, branch := range branches {
			if branch.GetName() == client.Branch {
				zap.S().Named("github").Debugf("found repo with branch %s: %s", client.Branch, repoName)
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

func FilterMatchingCommitsOnBranch(client *auth.GithubClient, prMap map[string][]*github.PullRequest, releaseRepos map[string]*github.Repository) (map[string][]*github.PullRequest, error) {
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

func gatherCommitsToCheck(client *auth.GithubClient, repo *github.Repository) ([]*github.Commit, error) {
	var allCommits []*github.Commit
	opts := &github.CommitsListOptions{
		SHA:   client.Branch,
		Since: client.Date,
	}

	for {
		commits, resp, err := client.Repositories.ListCommits(client.Ctx, client.Org, repo.GetName(), opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list commits for repo %s: %w", repo.GetName(), err)
		}

		zap.S().Named("github").Debugf("found %d commits on branch %s for repo %s", len(commits), client.Branch, repo.GetName())
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

func UpdateBranchRules(client *auth.GithubClient) ([]*github.Repository, error) {
	var cleansedRepos []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(client.Ctx, client.Org, opts)
		if err != nil {
			zap.S().Named("rules").Errorf("failed to fetch some repositories: %v", err)
			return nil, err
		}

		for _, repo := range repos {
			// remove archived repos
			if repo.GetArchived() {
				continue
			}

			// Requires a paid plan that we do not have
			if repo.GetPrivate() {
				continue
			}

			hasBranch, err := checkForReleaseBranch(client, repo)
			if err != nil {
				zap.S().Named("rules").Errorf("error looking for release branch on repo %s", repo.GetName())
				continue
			}

			if !hasBranch {
				continue
			}

			zap.S().Named("rules").Debugf("found repo %s", repo.GetName())
			cleansedRepos = append(cleansedRepos, repo)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	var addedRepos []*github.Repository
	for _, repo := range cleansedRepos {
		repoName := repo.GetName()
		rule, _, err := client.Repositories.GetBranchProtection(client.Ctx, client.Org, repoName, client.Branch)
		if err != nil && rule != nil {
			zap.S().Named("rules").Errorf("failed to get rule for repo %s: %v", repoName, err)
			continue
		} else if rule != nil {
			zap.S().Named("rules").Debugf("found valid rule for repo %s, skipping", repoName)
			continue
		}

		requireConvRes := true
		req := &github.ProtectionRequest{
			RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
				RequiredApprovingReviewCount: 1,
				RequireCodeOwnerReviews:      true,
			},
			RequiredStatusChecks: &github.RequiredStatusChecks{
				Checks: &[]*github.RequiredStatusCheck{
					{
						Context: "build-and-test / build-and-test",
					},
				},
			},
			RequiredConversationResolution: &requireConvRes,
		}
		zap.S().Named("rules").Debugf("adding rule to repo %s", repoName)
		rule, _, err = client.Repositories.UpdateBranchProtection(client.Ctx, client.Org, repoName, client.Branch, req)
		if err != nil {
			zap.S().Named("rules").Errorf("failed to add release/2.7.x branch protection for repo %s: %v", repoName, err)
		}
		if rule != nil {
			addedRepos = append(addedRepos, repo)
		}
	}
	return addedRepos, nil
}
