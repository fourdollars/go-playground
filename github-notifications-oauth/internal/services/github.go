package services

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

// GitHubService defines the interface for interacting with the GitHub API.
type GitHubService interface {
	ListNotifications(ctx context.Context, opts *github.NotificationListOptions) ([]*github.Notification, *github.Response, error)
	MarkThreadRead(ctx context.Context, id int64) (*github.Response, error)
}

// githubClient implements GitHubService using the official github.Client.
type githubClient struct {
	client *github.Client
}

func (g *githubClient) ListNotifications(ctx context.Context, opts *github.NotificationListOptions) ([]*github.Notification, *github.Response, error) {
	return g.client.Activity.ListNotifications(ctx, opts)
}

func (g *githubClient) MarkThreadRead(ctx context.Context, id int64) (*github.Response, error) {
	return g.client.Activity.MarkThreadRead(ctx, fmt.Sprintf("%d", id))
}

// NewGitHubService creates a new GitHubService.
// If a token is provided, it creates an authenticated client.
// Otherwise, it creates an unauthenticated client.
func NewGitHubService(ctx context.Context, token string) GitHubService {
	var tc *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc = oauth2.NewClient(ctx, ts)
	}
	return &githubClient{client: github.NewClient(tc)}
}
