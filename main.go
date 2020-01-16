package main

import (
	"fmt"
	"net/http"
	"encoding/json"
	"os"
	"io/ioutil"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"github.com/joho/godotenv"
	githuboauth "golang.org/x/oauth2/github"
)

const htmlIndex = `<html><body>
Log in with <a href="/login">GitHub</a>
</body></html>
`
var (
	oauthConf = &oauth2.Config{
			ClientID: os.Getenv("CLIENT_ID"),
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

type hook_struct struct {
		Action string `json:"action"`
		Number int `json:"number"`
		PullRequest struct {
			URL string `json:"url"`
			Body string `json:"body"`
			ClosedAt string `json:"closed_at"`
		} `json:"pull_request"`
		Description string `json:"description"`
		Repository struct {
			Name string `json:"name"`
		} `json:"repository"`
}

var client *github.Client
func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(body))

	var hook hook_struct
	err = json.Unmarshal(body, &hook)
	if err != nil {
		panic(err)
	}

	fmt.Println(hook.Description)
	fmt.Println(hook.Repository)
	fmt.Println(hook.Repository.Name)

	if hook.Action == "pull_request" {
		//pr, _, err := client.PullRequests.Edit(ctx, *prRepoOwner, *prRepo, newPR)
	}
}

func main() {
	// Load .env values...
	godotenv.Load()
	http.HandleFunc("/", handleMain)
	http.HandleFunc("/login", handleGitHubLogin)
	http.HandleFunc("/github_go_taskbot", handleGitHubCallback)
	http.HandleFunc("/webhooks", handleWebhooks)
	fmt.Print("Started running on http://127.0.0.1:7000\n")
	fmt.Println(http.ListenAndServe(":7000", nil))
}
