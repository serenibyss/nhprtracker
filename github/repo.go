package github

import (
	"errors"
	"slices"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"

	"github.com/serenibyss/nhprtracker/auth"
	"github.com/serenibyss/nhprtracker/internal"
)

// GatherRepositories gathers all repositories on the specified organization
func GatherRepositories(client *auth.GithubClient) ([]*github.Repository, error) {
	if client.Repos != nil {
		return gatherSpecificRepositories(client)
	}

	var cleansedRepos []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(client.Ctx, client.Org, opts)
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
			if repo.GetPushedAt().Before(client.Date) {
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

func gatherSpecificRepositories(client *auth.GithubClient) ([]*github.Repository, error) {
	var repositories []*github.Repository
	var hadError bool

	for _, repo := range client.Repos {
		repository, _, err := client.Repositories.Get(client.Ctx, client.Org, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to get repo with name %s/%s: %v", client.Org, repo, err)
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
func GatherReleaseRepositories(client *auth.GithubClient, repos []*github.Repository) (map[string]*github.Repository, error) {
	patchRepos := map[string]*github.Repository{}
	var hadError bool

	for _, repo := range repos {
		hasBranch, err := checkForReleaseBranch(client, repo)
		if err != nil {
			zap.S().Named("github").Errorf("failed to list branches for repo %s/%s: %v", client.Org, repo.GetName(), err)
			hadError = true
		}

		if hasBranch {
			zap.S().Named("github").Debugf("found patch repo %s", repo.GetName())
			patchRepos[client.Org+"/"+repo.GetName()] = repo
		}
	}
	if hadError {
		return patchRepos, errors.New("some repo branches could not be checked, see logs above")
	}
	return patchRepos, nil
}
