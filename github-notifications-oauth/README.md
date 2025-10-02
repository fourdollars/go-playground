# Go GitHub Notification Web App (OAuth Version)

A Single Page Application (SPA) built with Go for the backend API and vanilla JavaScript for the frontend, designed to securely read and manage your GitHub notifications. This version leverages the GitHub OAuth2 flow, eliminating the need for manual Personal Access Tokens and providing a seamless, secure authentication experience.

## Table of Contents
- [Introduction](#introduction)
- [Features](#features)
- [Project Structure](#project-structure)
- [Technologies Used](#technologies-used)
- [Setup and Run](#setup-and-run)
  - [Prerequisites](#prerequisites)
  - [Step 1: Create a GitHub OAuth App](#step-1-create-a-github-oauth-app)
  - [Step 2: Clone the Repository](#step-2-clone-the-repository)
  - [Step 3: Configure Environment Variables](#step-3-configure-environment-variables)
  - [Step 4: Run the Application](#step-4-run-the-application)
- [Usage](#usage)
- [Contributing](#contributing)

## Features

*   **Secure OAuth2 Authentication:** Guides users through the GitHub authorization process to securely obtain an API access token.
*   **Client-Side Token Management:** Safely stores the fetched `access_token` in the browser's Local Storage.
*   **Dynamic Notification Loading:** Asynchronously fetches and displays unread notifications via the backend API.
*   **Clear Separation of Concerns:** The Go backend handles the OAuth flow and provides a JSON API, while the frontend manages all rendering and user interaction.
*   **Notification Management:** Provides "Mark as Read" and "Logout" functionalities.

## Project Structure

```
github-notifications-oauth/
├── go.mod                 # Go module definition
├── go.sum                 # Go module checksums
├── README.md              # This README file
├── cmd/
│   └── server/
│       └── main.go        # Backend server entry point
├── internal/
│   ├── config/
│   │   └── config.go      # Application configuration loading
│   ├── handlers/
│   │   └── http.go        # HTTP request handlers (OAuth, API)
│   └── services/
│       └── github.go      # GitHub API interaction logic
└── web/
    ├── callback.html      # OAuth callback page
    └── index.html         # Main frontend application page
```

## Technologies Used

*   **Go:** For the backend API and OAuth flow management.
*   **HTML, CSS, JavaScript:** For the frontend Single Page Application.

## Setup and Run

### Prerequisites

*   [Go](https://go.dev/doc/install) (version 1.25.1 or higher) installed.
*   A GitHub account.

### Step 1: Create a GitHub OAuth App

To enable GitHub OAuth, you need to register a new OAuth App on GitHub to obtain a `Client ID` and `Client Secret`.

1.  Log in to your GitHub account.
2.  Navigate to **Settings** > **Developer settings** > **OAuth Apps**.
3.  Click the **New OAuth App** button.
4.  Fill out the form with the following details:
    *   **Application name:** `Go Notification Manager` (or any descriptive name)
    *   **Homepage URL:** `http://localhost:8080`
    *   **Authorization callback URL:** `http://localhost:8080/github/callback` (This URL is critical and must match exactly.)
5.  Click **Register application**.
6.  On the next page, note down your **Client ID**.
7.  Click **Generate a new client secret** and copy the generated **Client Secret**.
    *   **Important:** Securely save both your `Client ID` and `Client Secret`. You will need them in the next steps.

### Step 2: Clone the Repository

First, clone the project repository to your local machine:

```bash
git clone https://github.com/your-username/github-notifications-oauth.git # Replace with actual repo URL
cd github-notifications-oauth
```

### Step 3: Configure Environment Variables

The application requires three environment variables: `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `OAUTH_STATE_STRING`.

*   `GITHUB_CLIENT_ID`: Your GitHub OAuth App's Client ID.
*   `GITHUB_CLIENT_SECRET`: Your GitHub OAuth App's Client Secret.
*   `OAUTH_STATE_STRING`: A secure, random string used to prevent cross-site request forgery (CSRF) attacks during the OAuth flow. Generate a strong, unique string for this.

**Example for macOS / Linux:**

```bash
export GITHUB_CLIENT_ID="YOUR_CLIENT_ID"
export GITHUB_CLIENT_SECRET="YOUR_CLIENT_SECRET"
export OAUTH_STATE_STRING="YOUR_SECURE_RANDOM_STRING"
```

**Example for Windows (Command Prompt):**

```cmd
set GITHUB_CLIENT_ID=YOUR_CLIENT_ID
set GITHUB_CLIENT_SECRET=YOUR_CLIENT_SECRET
set OAUTH_STATE_STRING=YOUR_SECURE_RANDOM_STRING
```

**Example for Windows (PowerShell):**

```powershell
$env:GITHUB_CLIENT_ID="YOUR_CLIENT_ID"
$env:GITHUB_CLIENT_SECRET="YOUR_CLIENT_SECRET"
$env:OAUTH_STATE_STRING="YOUR_SECURE_RANDOM_STRING"
```

Replace `YOUR_CLIENT_ID`, `YOUR_CLIENT_SECRET`, and `YOUR_SECURE_RANDOM_STRING` with your actual values.

### Step 4: Run the Application

Navigate to the `cmd/server` directory and run the Go application. You can optionally specify the listen address using the `-listenAddr` flag (defaults to `:8891`).

```bash
cd cmd/server
go mod tidy # To download dependencies
go run main.go -listenAddr ":8080" # Example: run on port 8080
```

You should see output similar to `Server started at http://localhost:8080` in your terminal (or the address you specified).

## Usage

1.  Once the server is running, open your web browser and navigate to: `http://localhost:8080`
2.  Click the "Login with GitHub" button.
3.  You will be redirected to GitHub's authorization page. Review the requested permissions and click "Authorize".
4.  Upon successful authorization, you will be redirected back to the application, which will then display your unread GitHub notifications.
5.  You can use the "Mark as Read" functionality for individual notifications or click "Logout" to clear your session.

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.