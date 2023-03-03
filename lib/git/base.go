package provider

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

type gitAuthor struct {
	Name  string
	Email string
}

type baseClient struct {
	auth   *http.BasicAuth
	author gitAuthor
}

const (
	Add    string = "Add"
	Delete string = "Delete"
)

func NewBaseClient(auth *http.BasicAuth) *baseClient {
	return &baseClient{
		auth: auth,
		author: gitAuthor{
			Name:  "Kratix",
			Email: "kratix@syntasso.io",
		},
	}
}

func (b *baseClient) Init(repositoryURL, path string) (*git.Repository, error) {
	repo, err := git.PlainInit(path, false)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialise repository")
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repositoryURL},
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create remote")
	}

	return repo, nil
}

func (b *baseClient) Clone(repositoryURL, path string) (*git.Repository, error) {
	return git.PlainClone(path, false, &git.CloneOptions{
		Auth:            b.auth,
		URL:             repositoryURL,
		ReferenceName:   plumbing.NewBranchReferenceName("main"),
		SingleBranch:    true,
		Depth:           1,
		NoCheckout:      false,
		InsecureSkipTLS: true,
	})
}

func (b *baseClient) RemoveFile(repo *git.Repository, objectName string, log logr.Logger) error {
	worktree, err := repo.Worktree()
	if err != nil {
		log.Error(err, "could not access repo worktree")
		return err
	}

	if _, err := worktree.Filesystem.Lstat(objectName); err == nil {
		if _, err := worktree.Remove(objectName); err != nil {
			log.Error(err, "could not remove file from worktree")
			return err
		}
		log.Info("successfully deleted file from worktree")
	} else {
		log.Info("file does not exist on worktree, nothing to delete")
		return nil
	}

	return b.commitAndPush(repo, worktree, Delete, objectName, log)
}

func (b *baseClient) AddFile(repo *git.Repository, repoPath, objectName string, toWrite []byte, log logr.Logger) error {
	objectFileName := filepath.Join(repoPath, objectName)
	if err := ioutil.WriteFile(objectFileName, toWrite, 0644); err != nil {
		log.Error(err, "could not write to file")
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		log.Error(err, "could not access repo worktree")
		return err
	}

	if _, err := worktree.Add(objectName); err != nil {
		log.Error(err, "could not add file to worktree")
		return err
	}

	return b.commitAndPush(repo, worktree, Add, objectName, log)
}

func (b *baseClient) commitAndPush(repo *git.Repository, worktree *git.Worktree, action, fileToAdd string, log logr.Logger) error {
	status, err := worktree.Status()
	if err != nil {
		log.Error(err, "could not get worktree status")
		return err
	}

	if status.IsClean() {
		log.Info("no changes to be committed")
		return nil
	}

	_, err = worktree.Commit(fmt.Sprintf("%s: %s", action, fileToAdd), &git.CommitOptions{
		Author: &object.Signature{
			Name:  b.author.Name,
			Email: b.author.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Error(err, "could not commit file to worktree")
		return err
	}

	return b.push(repo, log)
}

func (b *baseClient) push(repo *git.Repository, log logr.Logger) error {
	err := repo.Push(&git.PushOptions{
		RemoteName:      "origin",
		Auth:            b.auth,
		InsecureSkipTLS: true,
	})
	if err != nil {
		log.Error(err, "could not push to remote")
		return err
	}
	return nil
}
