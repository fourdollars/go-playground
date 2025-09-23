package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	//     "golang.org/x/crypto/acme/autocert"
	"hash"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strings"
	//     "reflect"
)

func GetSignature(input []byte, key string, crypto func() hash.Hash) string {
	key_for_sign := []byte(key)
	h := hmac.New(crypto, key_for_sign)
	h.Write(input)
	sum := h.Sum(nil)
	return fmt.Sprintf("%x", sum)
}

var html = template.Must(template.New("https").Parse(`
<html>
<head>
  <title>Webhook</title>
</head>
<body>
  <h1>Webhook</h1>
</body>
</html>
`))

type Data struct {
	SourceGitRepository       string `json:"source_git_repository"`
	PrerequisiteBranch        string `json:"prerequisite_branch"`
	TargetBranch              string `json:"target_branch"`
	Description               string `json:"description"`
	SourceBranch              string `json:"source_branch"`
	Registrant                string `json:"registrant"`
	QueueStatus               string `json:"queue_status"`
	Whiteboard                string `json:"whiteboard"`
	SourceGitPath             string `json:"source_git_path"`
	PrerequisiteGitPath       string `json:"prerequisite_git_path"`
	TargetGitPath             string `json:"target_git_path"`
	PreviewDiff               string `json:"preview_diff"`
	CommitMessage             string `json:"commit_message"`
	TargetGitRepository       string `json:"target_git_repository"`
	PrerequisiteGitRepository string `json:"prerequisite_git_repository"`
}

type Sender struct {
	Login string `json:"login"`
}

type PullRequest struct {
	Reviewers []map[string]interface{} `json:"requested_reviewers"`
	Title     string                   `json:"title"`
	Url       string                   `json:"html_url"`
	State     string                   `json:"state"`
}

type PullEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	Sender      Sender      `json:"sender"`
	PullRequest PullRequest `json:"pull_request"`
}

type MergeEvent struct {
	Action        string `json:"action"`
	Old           Data   `json:"old"`
	New           Data   `json:"new"`
	MergeProposal string `json:"merge_proposal"`
}

type Commit struct {
	CommitSha1 string `json:"commit_sha1"`
}

type Change struct {
	New Commit `json:"new"`
	Old Commit `json:"old"`
}

type PushEvent struct {
	GitRepository     string            `json:"git_repository"`
	RefChanges        map[string]Change `json:"ref_changes"`
	GitRepositoryPath string            `json:"git_repository_path"`
}

func mattermost(url, json, id string) {
	if id == "" {
		log.Fatal("No id to send ", json)
	}
	var jsonStr = []byte(json)
	req, err := http.NewRequest("POST", url+id, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	log.Print("Send ", json, " to ", id)

	//body, _ := ioutil.ReadAll(resp.Body)
}

func main() {
	var hook string
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to get executable path: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	f, err := os.OpenFile(filepath.Join(exeDir, ".webhook.fcgi.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Static("/js", "./js")
	r.SetHTMLTemplate(html)
	if os.Getenv("_") == "./webhook.fcgi" {
		hook = "/"
	} else {
		hook = "/webhook.fcgi"
	}

	// Read url from .webhook.fcgi.url
	urlFile, err := os.Open(filepath.Join(exeDir, ".webhook.fcgi.url"))
	if err != nil {
		log.Fatal(err)
	}
	defer urlFile.Close()
	urlBytes, err := ioutil.ReadAll(urlFile)
	if err != nil {
		log.Fatal(err)
	}
	url := strings.TrimSpace(string(urlBytes))

	// Read secret from .webhook.fcgi.secret
	secretFile, err := os.Open(filepath.Join(exeDir, ".webhook.fcgi.secret"))
	if err != nil {
		log.Fatal(err)
	}
	defer secretFile.Close()
	secretBytes, err := ioutil.ReadAll(secretFile)
	if err != nil {
		log.Fatal(err)
	}
	secret := strings.TrimSpace(string(secretBytes))

	r.POST(hook, func(c *gin.Context) {
		var r = c.Request
		var status = http.StatusUnauthorized
		var sliceSHA1 = strings.Split(r.Header.Get("X-Hub-Signature"), "=")
		var sliceSHA256 = strings.Split(r.Header.Get("X-Hub-Signature-256"), "=")
		var eventType = r.Header.Get("X-Launchpad-Event-Type")
		if eventType == "" {
			eventType = r.Header.Get("x-github-event")
		}
		var contentType = r.Header.Get("Content-Type")

		c.Request.ParseForm()
		id := c.Request.Form.Get("id")

		body := c.Request.Body
		x, _ := ioutil.ReadAll(body)

		if contentType == "application/json" {
			if len(sliceSHA256) == 2 && sliceSHA256[0] == "sha256" && GetSignature(x, secret, sha256.New) == sliceSHA256[1] {
				status = http.StatusOK
			} else if len(sliceSHA1) == 2 && sliceSHA1[0] == "sha1" && GetSignature(x, secret, sha1.New) == sliceSHA1[1] {
				status = http.StatusOK
			}
		}

		if status != http.StatusOK {
			log.Printf("%d %s\n", status, http.StatusText(status))
			c.JSON(status, gin.H{"status": http.StatusText(status)})
			return
		}

		switch eventType {
		// https://help.launchpad.net/API/Webhooks
		case "git:push:0.1":
			var push PushEvent
			if e := json.Unmarshal(x, &push); e != nil {
				log.Fatal(e)
			}
			for k, v := range push.RefChanges {
				var action, sha1 string
				var slice = strings.Split(k, "/")
				var branch string
				var tag string
				if slice[0] == "refs" {
					switch slice[1] {
					case "heads":
						branch = slice[2]
					case "tags":
						tag = slice[2]
					}
				}
				if v.Old.CommitSha1 == "" {
					action = "created"
					sha1 = v.New.CommitSha1
				} else if v.New.CommitSha1 == "" {
					action = "deleted"
					sha1 = v.Old.CommitSha1
				} else {
					action = "committed"
					sha1 = v.New.CommitSha1
				}
				log.Printf("Git push: https://code.launchpad.net%s, branch:%s, tag:%s, sha1:%s, action:%s\n", push.GitRepository, branch, tag, sha1, action)
				if tag != "" {
					mattermost(url, `{"text": "https://git.launchpad.net`+push.GitRepository+`/commit/?id=`+sha1+` with the '`+tag+`' tag is `+action+`."}`, id)
				}
			}
		case "merge-proposal:0.1":
			var merge MergeEvent
			if e := json.Unmarshal(x, &merge); e != nil {
				log.Fatal(e)
			}
			log.Print(`Merge proposal: https://code.launchpad.net` + merge.MergeProposal + ` ` + merge.Action)
			switch merge.Action {
			case "deleted":
			case "created":
				if merge.New.QueueStatus == "Needs review" {
					payload := fmt.Sprintf("{\"text\": \"https://code.launchpad.net%s from @%s needs review.\"}", merge.MergeProposal, merge.New.Registrant[2:])
					mattermost(url, payload, id)
				}
			case "modified":
				if merge.Old.QueueStatus != "Needs review" && merge.New.QueueStatus == "Needs review" {
					var slice = strings.Split(merge.New.SourceGitPath, "/")
					var branch string
					if slice[0] == "refs" && slice[1] == "heads" {
						branch = slice[2]
					}
					payload := fmt.Sprintf("{\"text\": \"https://code.launchpad.net%s from @%s needs review.\"}", merge.MergeProposal, merge.New.Registrant[2:])
					mattermost(url, payload, id)
					log.Print(`It needs to run tests for https://code.launchpad.net` + merge.New.SourceGitRepository + `/+ref/` + branch + `.`)
				}
				if merge.Old.QueueStatus != "Approved" && merge.New.QueueStatus == "Approved" {
					log.Print(`It needs to merge lp:` + merge.New.SourceGitRepository[1:] + ` into ` + `lp:` + merge.New.TargetGitRepository[1:])
				}
			default:
				log.Printf("Unhandled Action: %s\n", merge.Action)
			}
		// https://docs.github.com/en/webhooks/webhook-events-and-payloads
		case "pull_request":
			var event PullEvent
			if e := json.Unmarshal(x, &event); e != nil {
				log.Fatal(e)
			}
			log.Printf("Pull request: %s\n", event.PullRequest.Url)
			switch event.Action {
			case "opened":
				if event.PullRequest.State == "open" {
					reviewers := []string{}
					for _, reviewer := range event.PullRequest.Reviewers {
						if login, ok := reviewer["login"].(string); ok {
							reviewers = append(reviewers, `@`+login)
						}
					}
					var payload string
					if len(reviewers) == 0 {
						payload = fmt.Sprintf("{\"text\": \"[Pull Request #%d](%s) `%s` from @%s needs review.\"}", event.Number, event.PullRequest.Url, event.PullRequest.Title, event.Sender.Login)
					} else {
						payload = fmt.Sprintf("{\"text\": \"[Pull Request #%d](%s) `%s` from @%s needs %s review.\"}", event.Number, event.PullRequest.Url, event.PullRequest.Title, event.Sender.Login, strings.Join(reviewers, " "))
					}
					mattermost(url, payload, id)
				}
			default:
				log.Printf("Unhandled Action: %s\n", event.Action)
			}
		default:
			log.Print("Unhandled Payload Headers:")
			for k, v := range r.Header {
				log.Print(k + ": " + strings.Join(v, ", "))
			}
		}
		status = http.StatusOK
		c.JSON(status, gin.H{"status": http.StatusText(status)})
	})

	r.GET(hook, func(c *gin.Context) {
		if pusher := c.Writer.Pusher(); pusher != nil {
			if err := pusher.Push("/js/app.js", nil); err != nil {
				log.Printf("Failed to push: %v", err)
			}
			log.Printf("Succeeded to push.")
		}
		c.HTML(http.StatusOK, "https", gin.H{
			"status": http.StatusText(http.StatusOK),
		})
	})

	if os.Getenv("_") == "./webhook.fcgi" {
		log.Print("Running as a standalone server")
		if e := r.Run(); e != nil {
			log.Fatal(e)
		}
	} else if len(os.Args) == 2 {
		socketPath := os.Args[1]
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "net.Listen failed: %v\n", err)
			os.Exit(1)
		}
		log.Print("Running as a FastCGI socket server")
		err = fcgi.Serve(l, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		log.Print("Running as a FastCGI stdin server")
		if e := fcgi.Serve(nil, r); e != nil {
			log.Fatal(e)
		}
	}
}
