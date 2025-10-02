package config

import (
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var (
	OauthConf        *oauth2.Config
	OauthStateString string
)

func LoadConfig() (*oauth2.Config, string, error) {
	clientID := os.Getenv("GITHUB_CLIENT_ID")
	clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	oauthStateString := os.Getenv("OAUTH_STATE_STRING")

	if clientID == "" || clientSecret == "" {
		return nil, "", fmt.Errorf("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET environment variables must be set.")
	}
	if oauthStateString == "" {
		return nil, "", fmt.Errorf("OAUTH_STATE_STRING environment variable must be set.")
	}

	OauthConf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"notifications"},
		Endpoint:     github.Endpoint,
	}

	return OauthConf, oauthStateString, nil
}
