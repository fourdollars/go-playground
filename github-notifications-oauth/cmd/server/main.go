package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github-notifications-oauth/internal/config"
	"github-notifications-oauth/internal/handlers"
	"github-notifications-oauth/internal/services"
)

func main() {
	var err error
	config.OauthConf, config.OauthStateString, err = config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	// Create a new handler instance with the GitHub service factory
	h := handlers.NewHandler(services.NewGitHubService)

	http.HandleFunc("/", handlers.HandleMain)
	http.HandleFunc("/login", handlers.HandleGitHubLogin)
	http.HandleFunc("/github/callback", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleGitHubCallback(w, r, context.Background())
	})
	http.HandleFunc("/api/notifications", h.APINotificationsHandler)
	http.HandleFunc("/api/mark-as-read", h.APIMarkAsReadHandler)

	listenAddr := flag.String("listenAddr", ":8080", "HTTP listen address")
	flag.Parse()

	fmt.Printf("Server started at http://localhost%s\n", *listenAddr)
	fmt.Println("Use Ctrl+C to stop the server")

	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
