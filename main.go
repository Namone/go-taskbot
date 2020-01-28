package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	github "github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

const htmlIndex = `<html><body>
Log in with <a href="/login">GitHub</a>
</body></html>
`

var (
	oauthConf = &oauth2.Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Scopes: []string{
			"repo:status",
			"repo_deployment",
			"read:repo_hook",
			"write:discussion",
			"workflow",
		},
		Endpoint: githuboauth.Endpoint,
	}
	oauthStateString = "randomsalt"
)

func tokenToJSON(token *oauth2.Token) (string, error) {
	if d, err := json.Marshal(token); err != nil {
		return "", err
	} else {
		return string(d), nil
	}
}

func tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(htmlIndex))
}

func handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	url := oauthConf.AuthCodeURL(oauthStateString, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	if state != oauthStateString {
		fmt.Printf("invalid oauth state, expected '%s', got '%s'\n", oauthStateString, state)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")
	token, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		fmt.Printf("oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	client := github.NewClient(oauthClient)
	user, _, err := client.Users.Get(oauth2.NoContext, "")
	if err != nil {
		fmt.Printf("client.Users.Get() faled with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	tokenAsString, err := tokenToJSON(token)
	if err != nil {
		fmt.Printf("Error when converting token to a string: '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	os.Setenv("OAUTH_TOKEN", tokenAsString)
	fmt.Printf("Logged in as GitHub user: %s\n", *user.Login)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

type pullRequestHook struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		URL      string `json:"url"`
		Body     string `json:"body"`
		State    string `json:"state"`
		ClosedAt string `json:"closed_at"`
	} `json:"pull_request"`
	Description string `json:"description"`
	Repository  struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

var client *github.Client
var ctx = context.Background()
var (
	prSubject     = flag.String("pr-title", "test", "Title of the pull request. If not specified, no pull request will be created.")
	prDescription = flag.String("pr-text", "testing", "Text to put in the description of the pull request.")
)

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
	}

	var hook pullRequestHook
	err = json.Unmarshal(body, &hook)
	if err != nil {
		panic(err)
	}

	newPR := &github.PullRequest{
		Title:               prSubject,
		Body:                flag.String("pr-body", hook.PullRequest.Body, "PR Body."),
		State:               flag.String("pr-state", hook.PullRequest.State, "PR State."),
		MaintainerCanModify: github.Bool(false),
	}
	if hook.Action == "opened" {
		pr, _, err := client.PullRequests.Edit(
			ctx,
			hook.Repository.Owner.Login,
			hook.Repository.Name,
			hook.Number,
			newPR)
		if err != nil {
			// return err
		}

		fmt.Println(pr)
		fmt.Println("Updated PR!")
	}
	// return nil
}

func main() {
	flag.Parse()
	// Load .env values...
	godotenv.Load()
	http.HandleFunc("/", handleMain)
	http.HandleFunc("/login", handleGitHubLogin)
	http.HandleFunc("/github_go_taskbot", handleGitHubCallback)
	http.HandleFunc("/webhooks", handleWebhooks)
	fmt.Print("Started running on http://127.0.0.1:7000\n")
	fmt.Println(http.ListenAndServe(":7000", nil))
}
