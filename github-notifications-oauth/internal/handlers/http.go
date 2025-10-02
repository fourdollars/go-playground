package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github-notifications-oauth/internal/config"
	"github-notifications-oauth/internal/services"
	"golang.org/x/oauth2"
)

// GitHubServiceFactory defines a function type for creating GitHubService instances.
type GitHubServiceFactory func(ctx context.Context, token string) services.GitHubService

// Handler struct holds dependencies for HTTP handlers.
type Handler struct {
	GitHubServiceFactory GitHubServiceFactory
}

// NewHandler creates a new Handler instance.
func NewHandler(factory GitHubServiceFactory) *Handler {
	return &Handler{
		GitHubServiceFactory: factory,
	}
}

// extractToken extracts the Bearer token from the Authorization header.
func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

// HandleMain serves the main index.html page.
func HandleMain(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/index.html")
}

// HandleGitHubLogin redirects the user to the GitHub authorization page.
func HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	url := config.OauthConf.AuthCodeURL(config.OauthStateString, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleGitHubCallback handles the request from the GitHub callback.
func HandleGitHubCallback(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	if r.FormValue("state") != config.OauthStateString {
		log.Println("Invalid oauth state")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")
	token, err := config.OauthConf.Exchange(ctx, code)
	if err != nil {
		log.Printf("oauthConf.Exchange() failed: %v\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	tmpl, err := template.ParseFiles("web/callback.html")
	if err != nil {
		log.Printf("Could not parse callback.html template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, token.AccessToken)
	if err != nil {
		log.Printf("Could not execute callback.html template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// APINotificationsHandler handles API requests to get notifications and returns them as JSON.
func (h *Handler) APINotificationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractToken(r)
	if token == "" {
		http.Error(w, "Authorization header missing", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()
	// Create a GitHubService instance with the extracted token for this request
	gitHubService := h.GitHubServiceFactory(ctx, token)
	notifications, _, err := gitHubService.ListNotifications(ctx, nil)
	if err != nil {
		log.Printf("Could not get notifications: %v", err)
		http.Error(w, "Could not retrieve notifications from GitHub API", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(notifications); err != nil {
		log.Printf("Could not encode notifications to JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// MarkReadRequest is used to parse the JSON request body from the frontend.
type MarkReadRequest struct {
	ThreadID int64 `json:"thread_id"`
}

// APIMarkAsReadHandler handles API requests to mark a notification as read.
func (h *Handler) APIMarkAsReadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractToken(r)
	if token == "" {
		http.Error(w, "Authorization header missing", http.StatusUnauthorized)
		return
	}

	var reqBody MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if reqBody.ThreadID == 0 {
		http.Error(w, "Missing thread_id", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	// Create a GitHubService instance with the extracted token for this request
	gitHubService := h.GitHubServiceFactory(ctx, token)
	_, err := gitHubService.MarkThreadRead(ctx, reqBody.ThreadID)
	if err != nil {
		log.Printf("Could not mark notification as read (ID: %d): %v", reqBody.ThreadID, err)
		http.Error(w, "Could not mark notification as read", http.StatusInternalServerError)
		return
	}

	log.Printf("Notification %d marked as read", reqBody.ThreadID)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message": "Notification successfully marked as read"}`)
}
