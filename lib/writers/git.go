package writers

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-logr/logr"
	gitk "github.com/syntasso/kratix/lib/git"
)

type GitWriter struct {
	Log    logr.Logger
	author gitAuthor

	gitClient gitk.Provider
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
	gitClient, err := gitk.NewProvider(logger, gitk.Options{Type: gitk.Gitea})
	if err != nil {
		logger.Error(err, "Error creating git client")
		return nil, err
	}
	return &GitWriter{
		Log:       logger,
		gitClient: gitClient,
	}, nil
}

func (g *GitWriter) WriteObject(bucketName string, objectName string, toWrite []byte) error {
	log := g.Log.WithValues("bucketName", bucketName, "objectName", objectName)
	if len(toWrite) == 0 {
		log.Info("Empty byte[]. Nothing to write to Git")
		return nil
	}

	repoPath, err := createLocalDirectory(bucketName)
	if err != nil {
		log.Error(err, "could not create temporary repository directory")
		return err
	}
	defer os.RemoveAll(filepath.Dir(repoPath))

	repo, err := g.gitClient.CloneOrCreate(bucketName, repoPath)
	if err != nil {
		log.Error(err, "could not initialise repository")
		return err
	}

	return g.gitClient.AddFile(repo, repoPath, objectName, toWrite, log)
}

func (g *GitWriter) RemoveObject(bucketName string, objectName string) error {
	log := g.Log.WithValues("bucketName", bucketName, "objectName", objectName)

	repoPath, err := createLocalDirectory(bucketName)
	if err != nil {
		log.Error(err, "could not create temporary repository directory")
		return err
	}
	defer os.RemoveAll(filepath.Dir(repoPath))

	repo, err := g.gitClient.CloneOrCreate(bucketName, repoPath)
	if err != nil {
		log.Error(err, "could not clone repository")
		return err
	}

	return g.gitClient.RemoveFile(repo, objectName, log)
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
