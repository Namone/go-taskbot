package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

type gitHubServer map[string]string //Placeholder stype to hold the functions

type pullRequestHook struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Title    string `json:"title"`
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
var token *oauth2.Token

var (
	prTitle = flag.String("pr-title", "example(TSK-01): commit description", "Title of the pull request. If not specified, no pull request will be created.")
	prBody  = flag.String("pr-body", "_Please update your PR to include a link to the relevant ticket._", "Text to put in the description of the pull request.")
)
var query struct {
	Repository struct {
		Description string
	} `graphql:"repository(owner: \"Namone\", name: \"Task-Bot\")"`
}

const htmlIndex = `<html><body>
Log in with <a href="/login">GitHub</a>
</body></html>
`

var (
	oauthConf = &oauth2.Config{
		ClientID:     "cc617ba84077022614b7",                     //os.Getenv("CLIENT_ID"),
		ClientSecret: "3138b62da5226ce5713b97f836588c6ba5153205", //os.Getenv("CLIENT_SECRET"),
		Scopes: []string{
			"repo",
			"read:repo_hook",
			"write:discussion",
			"workflow",
		},
		Endpoint: githuboauth.Endpoint,
	}
	oauthStateString = "randomsalt"
)

func (g gitHubServer) tokenToJSON(t *oauth2.Token) (string, error) {
	if d, err := json.Marshal(t); err != nil {
		return "", err
	} else {
		return string(d), nil
	}
}

func (g gitHubServer) tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var t oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (g gitHubServer) handleMain(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(htmlIndex))
}

func (g gitHubServer) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	url := oauthConf.AuthCodeURL(oauthStateString, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (g gitHubServer) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")

	if state != oauthStateString {
		fmt.Printf("invalid oauth state, expected '%s', got '%s'\n", oauthStateString, state)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")
	tok, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		fmt.Printf("oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	token = tok
	oauthClient := oauthConf.Client(oauth2.NoContext, tok)
	client := github.NewClient(oauthClient)
	user, _, err := client.Users.Get(oauth2.NoContext, "")
	if err != nil {
		fmt.Printf("client.Users.Get() faled with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	tokenAsString, err := g.tokenToJSON(tok)
	if err != nil {
		fmt.Printf("Error when converting token to a string: '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	os.Setenv("OAUTH_TOKEN", tokenAsString)
	fmt.Printf("Logged in as GitHub user: %s\n", *user.Login)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (g gitHubServer) handleWebhooks(w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	var pullRequest *pullRequestHook
	err := decoder.Decode(&pullRequest)
	if err != nil {
		panic(err)
	}

	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	client := github.NewClient(oauthClient)

	switch action := pullRequest.Action; action {
	case "opened":
		newPR := &github.PullRequest{
			Title:               prTitle,
			Body:                prBody,
			MaintainerCanModify: github.Bool(false),
		}

		r, _ := regexp.Compile("/\b[a-zA-Z]{3}\\-{1}\\d{1,}\b|\b\\d{6}\b/g")

		fmt.Println(pullRequest.PullRequest.Title)
		fmt.Println(r.FindAllString(pullRequest.PullRequest.Title, -1))
		_, _, err := client.PullRequests.Edit(
			oauth2.NoContext,
			pullRequest.Repository.Owner.Login,
			pullRequest.Repository.Name,
			pullRequest.Number,
			newPR)
		if err != nil {
			panic(err)
		}
	default:
		// Nothing.
	}
}

//https://www.integralist.co.uk/posts/understanding-golangs-func-type/

func main() {
	// Load .env values...
	godotenv.Load()

	//Create a waitgroup to prevent program from exiting while HTTP server is in use
	// httpServerExitDone := &sync.WaitGroup{}
	server := gitHubServer{}
	http.HandleFunc("/", server.handleMain)
	http.HandleFunc("/login", server.handleGitHubLogin)
	http.HandleFunc("/github_go_taskbot", server.handleGitHubCallback)
	http.HandleFunc("/webhooks", server.handleWebhooks)

	fmt.Print("Started running on http://127.0.0.1:7000\n")

	log.Fatal(http.ListenAndServe("localhost:7000", nil))
}

//https://stackoverflow.com/questions/39320025/how-to-stop-http-listenandserve
func startHTTPServer(wg *sync.WaitGroup) *http.Server {
	srv := &http.Server{Addr: "7000"}

	go func() {
		defer wg.Done() // let main know we are done cleaning up

		// always returns error. ErrServerClosed on graceful close
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			// unexpected error. port in use?
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	// returning reference so caller can call Shutdown()
	return srv
}
