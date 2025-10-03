package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"time"

	"github.com/gorilla/sessions"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

var (
	googleOauthConfig   *oauth2.Config
	facebookOauthConfig *oauth2.Config
	githubOauthConfig   *oauth2.Config
	store               = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))
	isFcgiMode          bool
)

const (
	sessionName    = "auth-session"
	oauthStateKey  = "oauth-state"
	userProfileKey = "user-profile"
)

func main() {
	listenAddr := flag.String("listenAddr", "", "address for the standalone server to listen on")
	flag.Parse()

	googleOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}
	facebookOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("FACEBOOK_CLIENT_ID"),
		ClientSecret: os.Getenv("FACEBOOK_CLIENT_SECRET"),
		Scopes:       []string{"public_profile", "email"},
		Endpoint:     facebook.Endpoint,
	}
	githubOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHome)

	if *listenAddr != "" {
		isFcgiMode = false
		log.Printf("Running as a standalone server on %s", *listenAddr)
		h2s := &http2.Server{}
		h2cHandler := h2c.NewHandler(mux, h2s)
		server := &http.Server{
			Addr:    *listenAddr,
			Handler: h2cHandler,
		}
		log.Fatal(server.ListenAndServe())
	} else if len(os.Args) == 2 {
		isFcgiMode = true
		socketPath := os.Args[1]
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			log.Fatalf("Could not remove old socket: %v", err)
		}
		ln, err := net.Listen("unix", socketPath)
		if err != nil {
			log.Fatalf("Failed to listen on socket: %v", err)
		}
		defer ln.Close()
		log.Println("Running as a FastCGI socket server")
		if err := fcgi.Serve(ln, mux); err != nil {
			log.Fatalf("fcgi.Serve failed: %v", err)
		}
	} else {
		isFcgiMode = true
		log.Println("Running as a FastCGI stdin server")
		if err := fcgi.Serve(nil, mux); err != nil {
			log.Fatal(err)
		}
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	loginProvider := r.URL.Query().Get("login")
	callbackProvider := r.URL.Query().Get("callback")
	isLogout := r.URL.Query().Get("logout")

	if loginProvider != "" {
		var config *oauth2.Config
		switch loginProvider {
		case "google":
			config = googleOauthConfig
		case "facebook":
			config = facebookOauthConfig
		case "github":
			config = githubOauthConfig
		default:
			http.Error(w, "Unknown login provider", http.StatusBadRequest)
			return
		}
		handleLogin(w, r, config, loginProvider)
		return
	}

	if callbackProvider != "" {
		var config *oauth2.Config
		var userInfoURL string
		switch callbackProvider {
		case "google":
			config = googleOauthConfig
			userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
		case "facebook":
			config = facebookOauthConfig
			userInfoURL = "https://graph.facebook.com/me?fields=id,name,email"
		case "github":
			config = githubOauthConfig
			userInfoURL = "https://api.github.com/user"
		default:
			http.Error(w, "Unknown callback provider", http.StatusBadRequest)
			return
		}
		handleCallback(w, r, config, userInfoURL, callbackProvider)
		return
	}

	if isLogout == "true" {
		handleLogout(w, r)
		return
	}

	session, err := store.Get(r, sessionName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profile := session.Values[userProfileKey]

	pathPrefix := ""
	if isFcgiMode {
		pathPrefix = "/app-auth.fcgi"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintln(w, "<html><head><title>OAuth2 Login</title></head><body>")
	if profile != nil {
		fmt.Fprintln(w, "<h1>User Profile</h1>")
		fmt.Fprintf(w, "<pre>%s</pre>", profile)
		fmt.Fprintf(w, `<p><a href="%s?logout=true">Logout</a></p>`, pathPrefix)
	} else {
		fmt.Fprintln(w, "<h1>Login</h1>")
		fmt.Fprintf(w, `<p><a href="%s?login=google">Login with Google</a></p>`, pathPrefix)
		fmt.Fprintf(w, `<p><a href="%s?login=facebook">Login with Facebook</a></p>`, pathPrefix)
		fmt.Fprintf(w, `<p><a href="%s?login=github">Login with GitHub</a></p>`, pathPrefix)
	}
	fmt.Fprintln(w, "</body></html>")
}

func handleLogin(w http.ResponseWriter, r *http.Request, config *oauth2.Config, provider string) {
	state := generateStateOauthCookie(w)

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
	}

	pathPrefix := ""
	if isFcgiMode {
		pathPrefix = "/app-auth.fcgi"
	}

	conf := *config
	conf.RedirectURL = fmt.Sprintf("%s://%s%s?callback=%s", scheme, r.Host, pathPrefix, provider)
	log.Printf("Redirecting to OAuth provider with redirect_uri: %s", conf.RedirectURL)

	url := conf.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleCallback(w http.ResponseWriter, r *http.Request, config *oauth2.Config, userInfoURL string, provider string) {
	session, err := store.Get(r, sessionName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check state
	oauthState, _ := r.Cookie(oauthStateKey)
	if r.FormValue("state") != oauthState.Value {
		log.Println("invalid oauth state")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
	}

	pathPrefix := ""
	if isFcgiMode {
		pathPrefix = "/app-auth.fcgi"
	}

	conf := *config
	conf.RedirectURL = fmt.Sprintf("%s://%s%s?callback=%s", scheme, r.Host, pathPrefix, provider)

	// Exchange code for token
	token, err := conf.Exchange(context.Background(), r.FormValue("code"))
	if err != nil {
		log.Printf("Code exchange failed: %s\n", err.Error())
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Get user info
	client := conf.Client(context.Background(), token)
	response, err := client.Get(userInfoURL)
	if err != nil {
		log.Printf("Failed getting user info: %s\n", err.Error())
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer response.Body.Close()

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed reading user info response: %s\n", err.Error())
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Store user info in session
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(contents, &prettyJSON); err != nil {
		log.Printf("Failed to unmarshal user info: %s\n", err.Error())
		session.Values[userProfileKey] = string(contents)
	} else {
		pretty, _ := json.MarshalIndent(prettyJSON, "", "  ")
		session.Values[userProfileKey] = string(pretty)
	}

	if err := session.Save(r, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pathPrefix = ""
	if isFcgiMode {
		pathPrefix = "/app-auth.fcgi"
	}
	http.Redirect(w, r, pathPrefix+"/", http.StatusTemporaryRedirect)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, sessionName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clear session
	session.Values[userProfileKey] = nil
	session.Options.MaxAge = -1

	if err := session.Save(r, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pathPrefix := ""
	if isFcgiMode {
		pathPrefix = "/app-auth.fcgi"
	}
	http.Redirect(w, r, pathPrefix+"/", http.StatusTemporaryRedirect)
}

func generateStateOauthCookie(w http.ResponseWriter) string {
	expiration := time.Now().Add(20 * time.Minute)
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	cookie := http.Cookie{Name: oauthStateKey, Value: state, Expires: expiration}
	http.SetCookie(w, &cookie)
	return state
}
