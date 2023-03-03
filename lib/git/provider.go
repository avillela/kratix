package provider

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-logr/logr"
)

type Options struct {
	Type string
}

type Provider interface {
	CloneOrCreate(repoName, path string) (*git.Repository, error)
	RemoveFile(repo *git.Repository, objectName string, log logr.Logger) error
	AddFile(repo *git.Repository, repoPath, objectName string, toWrite []byte, log logr.Logger) error
}

const (
	Gitea string = "gitea"
)

func NewProvider(opt Options) (Provider, error) {
	switch opt.Type {
	case "gitea":
		return NewGiteaProvider()
	default:
		panic("not implemented")
	}
}
