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
	serverURL string
}

func NewGiteaProvider() (*giteaProvider, error) {
	giteaServer := "https://gitea-http.gitea.svc.cluster.local/gitea_admin/"
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
	}, nil
}

func (g *giteaProvider) CloneOrCreate(org, name string) (*git.Repository, error) {
	repo, err := g.Clone(g.serverURL, name)

	if err != nil {
		fmt.Println("Error cloning repo: ", err)
		switch err.Error() {
		case "repository not found":
			_, _, err := g.client.CreateRepo(gitea.CreateRepoOption{
				Name:          name,
				Private:       false,
				DefaultBranch: "main",
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create repo")
			}

			return g.CloneOrCreate(org, name)
		default:
			return nil, errors.Wrap(err, "failed to clone repo")
		}
	}

	return repo, nil
}
