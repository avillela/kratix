package provider

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"code.gitea.io/sdk/gitea"
	"github.com/go-git/go-git/v5"
	ghttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/pkg/errors"
)

type giteaProvider struct {
	*baseClient
	client    *gitea.Client
	org       string
	serverURL string
}

func NewGiteaProvider() (*giteaProvider, error) {
	giteaServer := "https://gitea-http.gitea.svc.cluster.local"
	auth := &ghttp.BasicAuth{
		Username: "gitea_admin",
		Password: "r8sA8CPHD9!bt6d",
	}

	// create a new http client without ssl validation
	httpClient := http.DefaultClient
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client, err := gitea.NewClient(
		giteaServer,
		gitea.SetBasicAuth(auth.Username, auth.Password),
		gitea.SetHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}
	return &giteaProvider{
		client:     client,
		serverURL:  giteaServer,
		baseClient: NewBaseClient(auth),
		org:        "gitea_admin",
	}, nil
}

func (g *giteaProvider) CloneOrCreate(repoName, path string) (*git.Repository, error) {
	repoURL := fmt.Sprintf("%s/%s/%s", g.serverURL, g.org, repoName)
	repo, err := g.Clone(repoURL, path)

	if err != nil {
		switch err.Error() {
		case "repository not found":
			_, _, err := g.client.CreateRepo(gitea.CreateRepoOption{
				Name:          repoName,
				Private:       false,
				DefaultBranch: "main",
				AutoInit:      true,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create repo")
			}

			return g.CloneOrCreate(repoName, path)
		case "remote repository is empty":
			return g.Init(repoURL, path)
		default:
			return nil, errors.Wrap(err, "failed to clone repo")
		}
	}

	return repo, nil
}
