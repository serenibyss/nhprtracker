package github

import (
	"errors"

	"github.com/google/go-github/v67/github"
	"go.uber.org/zap"

	"github.com/serenibyss/nhprtracker/auth"
)

type LabelData struct {
	Name       string
	OldName    string
	Color      string
	Desc       string
	UpdateOnly bool
}

func CreateLabelOnRepositories(client *auth.GithubClient, repos []*github.Repository, data *LabelData) error {
	if data.UpdateOnly && data.OldName == "" {
		return errors.New("could not update labels as no old name was specified")
	}

	var hadError bool

	for _, repo := range repos {
		err := createLabelOnRepository(client, repo, data)
		if err != nil {
			hadError = true
		}
	}

	if hadError {
		return errors.New("some repos could not have the label added/updated, see logs above")
	}
	return nil
}

func createLabelOnRepository(client *auth.GithubClient, repo *github.Repository, data *LabelData) error {
	if data.OldName != "" {
		label, _, err := client.Issues.GetLabel(client.Ctx, client.Org, repo.GetName(), data.OldName)
		if err != nil {
			zap.S().Named("github").Errorf("failed to get label with name %s for %s/%s: %v", data.OldName, client.Org, repo.GetName(), err)
			return err
		}

		if label != nil {
			label.Name = &data.Name
			if data.Color != "" {
				label.Color = &data.Color
			}
			if data.Desc != "" {
				label.Description = &data.Desc
			}

			_, _, err = client.Issues.EditLabel(client.Ctx, client.Org, repo.GetName(), data.OldName, label)
			if err != nil {
				zap.S().Named("github").Errorf("failed to update label for %s/%s: %v", client.Org, repo.GetName(), err)
				return err
			}
			zap.S().Named("github").Infof("updated label with name %s to name %s on repo %s/%s", data.OldName, data.Name, client.Org, repo.GetName())
			return nil
		}
	}

	if data.UpdateOnly {
		return nil
	}

	label := &github.Label{
		Name: &data.Name,
	}
	if data.Color != "" {
		label.Color = &data.Color
	}
	if data.Desc != "" {
		label.Description = &data.Desc
	}

	_, _, err := client.Issues.CreateLabel(client.Ctx, client.Org, repo.GetName(), label)
	if err != nil {
		zap.S().Named("github").Errorf("failed to create label for %s/%s: %v", client.Org, repo.GetName(), err)
		return err
	}
	zap.S().Named("github").Infof("created label with name %s on repo %s/%s", data.Name, client.Org, repo.GetName())
	return nil
}
