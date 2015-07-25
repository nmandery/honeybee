package main

import (
	"errors"
	"fmt"
	"github.com/google/go-github/github"
	"strconv"
)

const GithubUserReposSourceType = "github-user-repos"

type GithubUserReposSource struct {
	userName     string
	includeForks bool
}

func NewGithubUserReposSource(params SourceParams) (gs *GithubUserReposSource, err error) {
	userName := ""
	includeForks := false

	for k, v := range params {
		switch k {
		case "includeForks":
			includeForks, err = strconv.ParseBool(v)
			if err != nil {
				return
			}
		case "user":
			userName = v
		default:
			err = errors.New(fmt.Sprintf("Unknown parameter for %v: %v", GithubUserReposSourceType, k))
			return
		}
	}
	if userName == "" {
		err = errors.New("'user' parameter is not set")
		return
	}

	gs = &GithubUserReposSource{
		userName:     userName,
		includeForks: includeForks,
	}
	return gs, nil
}

func (gs *GithubUserReposSource) Type() string {
	return GithubUserReposSourceType
}

func (gs *GithubUserReposSource) Id() string {
	return IdEncodeStrings(gs.Type(), gs.userName)
}

func (gs *GithubUserReposSource) GetBlocks() (blocks []*Block, err error) {

	client := github.NewClient(nil)
	opt := &github.RepositoryListOptions{Type: "owner", Sort: "updated", Direction: "desc"}
	repos, _, err := client.Repositories.List(gs.userName, opt)
	if err != nil {
		return
	}

	for _, repo := range repos {
		if !gs.includeForks && *repo.Fork {
			continue
		}
		if repo.Name == nil || repo.HTMLURL == nil {
			continue
		}
		block := NewBlock(gs)
		if repo.Description != nil {
			block.Content = *repo.Description
		}
		block.Title = *repo.Name
		block.Link = *repo.HTMLURL

		/*
		   From http://stackoverflow.com/questions/15918588/github-api-v3-what-is-the-difference-between-pushed-at-and-updated-at

		   pushed_at will be updated any time a commit is pushed to any of the repository's
		   branches. updated_at will be updated any time the repository object is updated,
		   e.g. when the description or the primary language of the repository is updated. It's
		   not necessary that a push will update the updated_at attribute -- that will only
		   happen if a push triggers an update to the repository object. For example, if the
		   primary language of the repository was Python, and then you pushed lots of
		   JavaScript code -- that might change the primary language to JavaScript, which
		   updates the repository object's language attribute and in turn updates the
		   updated_at attribute. Previously, the primary language was getting updated after
		   every push, even if it didn't change (which wasn't intended), so it triggered
		   an update to updated_at.
		*/
		switch {
		case repo.PushedAt != nil:
			block.TimeStamp = repo.PushedAt.Time.UTC()
		case repo.UpdatedAt != nil:
			block.TimeStamp = repo.UpdatedAt.Time.UTC()
		case repo.CreatedAt != nil:
			block.TimeStamp = repo.CreatedAt.Time.UTC()
		}
		blocks = append(blocks, block)
	}
	return
}
