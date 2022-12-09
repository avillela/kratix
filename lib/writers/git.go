package writers

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-logr/logr"
)

type GitWriter struct {
	Log       logr.Logger
	gitServer gitServer
	author    gitAuthor
}

type gitServer struct {
	URL  string
	Auth *http.BasicAuth
}

type gitAuthor struct {
	Name  string
	Email string
}

const (
	Add    string = "Add"
	Delete string = "Delete"
)

func newGitBucketWriter(logger logr.Logger) (BucketWriter, error) {
	return &GitWriter{
		gitServer: gitServer{
			URL: "https://gitea-http.gitea.svc.cluster.local/gitea_admin/",
			Auth: &http.BasicAuth{
				Username: "gitea_admin",
				Password: "r8sA8CPHD9!bt6d",
			},
		},
		author: gitAuthor{
			Name:  "Kratix",
			Email: "kratix@syntasso.io",
		},
		Log: logger,
	}, nil
}

func (g *GitWriter) WriteObject(bucketName string, objectName string, toWrite []byte) error {
	if len(toWrite) == 0 {
		g.Log.Info("Empty byte[]. Nothing to write to Git", "objectName", objectName)
		return nil
	}

	repoPath, err := createLocalDirectory(bucketName)
	if err != nil {
		g.Log.Error(err, "could not create temporary repository directory")
		return err
	}
	defer os.RemoveAll(filepath.Dir(repoPath))

	repo, err := g.cloneRepo(bucketName, repoPath)
	if err != nil {
		switch err.Error() {
		case "repository not found":
			if repo, err = g.initRepo(bucketName, repoPath); err != nil {
				g.Log.Error(err, "could not initialise repository")
				return err
			}
		default:
			g.Log.Error(err, "could not clone repository")
			return err
		}
	}

	objectFileName := filepath.Join(repoPath, objectName)
	if err := ioutil.WriteFile(objectFileName, toWrite, 0644); err != nil {
		g.Log.Error(err, "could not write to file")
		return err
	}

	if err := g.commit(repo, Add, objectName); err != nil {
		return err
	}

	if err := g.push(repo); err != nil {
		return err
	}

	return nil
}

func (g *GitWriter) RemoveObject(bucketName string, objectName string) error {
	repoPath, err := createLocalDirectory(bucketName)
	if err != nil {
		g.Log.Error(err, "could not create temporary repository directory")
		return err
	}
	defer os.RemoveAll(filepath.Dir(repoPath))

	repo, err := g.cloneRepo(bucketName, repoPath)
	if err != nil {
		g.Log.Error(err, "could not clone repository")
		return err
	}

	filename := filepath.Join(repoPath, bucketName, objectName)
	if err := os.Remove(filename); err != nil {
		g.Log.Error(err, "could not delete file")
		return err
	}

	if err := g.commit(repo, Delete, objectName); err != nil {
		return err
	}

	if err := g.push(repo); err != nil {
		return err
	}

	return nil
}

func (g *GitWriter) push(repo *git.Repository) error {
	err := repo.Push(&git.PushOptions{
		RemoteName:      "origin",
		Auth:            g.gitServer.Auth,
		InsecureSkipTLS: true,
	})
	if err != nil {
		g.Log.Error(err, "could not push to remote")
		return err
	}
	return nil
}

func (g *GitWriter) cloneRepo(bucketName, repoPath string) (*git.Repository, error) {
	return git.PlainClone(repoPath, false, &git.CloneOptions{
		Auth:            g.gitServer.Auth,
		URL:             g.gitServer.URL + bucketName,
		SingleBranch:    true,
		Depth:           1,
		NoCheckout:      true,
		InsecureSkipTLS: true,
	})
}

func (g *GitWriter) initRepo(bucketName, repoPath string) (*git.Repository, error) {
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		g.Log.Error(err, "could not initialise repository")
		return nil, err
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{g.gitServer.URL + bucketName + ".git"},
	})
	if err != nil {
		g.Log.Error(err, "could not create remote")
		return nil, err
	}

	return repo, nil
}

func (g *GitWriter) commit(repo *git.Repository, action, fileToAdd string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		g.Log.Error(err, "could not access repo worktree")
		return err
	}

	if _, err := worktree.Add(fileToAdd); err != nil {
		g.Log.Error(err, "could not stage file to worktree", "file", fileToAdd)
		return err
	}

	_, err = worktree.Commit(fmt.Sprintf("%s: %s", action, fileToAdd), &git.CommitOptions{
		Author: &object.Signature{
			Name:  g.author.Name,
			Email: g.author.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		g.Log.Error(err, "could not commit file to worktree")
		return err
	}
	return nil
}

func createLocalDirectory(repositoryName string) (string, error) {
	dir, err := ioutil.TempDir("", "kratix-repo")
	if err != nil {
		return "", err
	}

	repoPath := filepath.Join(dir, repositoryName)
	os.MkdirAll(repoPath, 0700) // TODO: Should this be a single repo with different paths?

	return repoPath, nil
}