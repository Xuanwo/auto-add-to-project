package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/google/go-github/v47/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	AATP_TOKEN          = "AATP_TOKEN"
	AATP_USER           = "AATP_USER"
	AATP_PROJECT_NUMBER = "AATP_PROJECT_NUMBER"
)

type Client struct {
	restClient    *github.Client
	graphqlClient *githubv4.Client
}

func NewClient(ctx context.Context) *Client {
	token := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv(AATP_TOKEN)},
	)
	httpClient := oauth2.NewClient(ctx, token)
	restClient := github.NewClient(httpClient)
	graphqlClient := githubv4.NewClient(httpClient)

	return &Client{
		restClient, graphqlClient,
	}
}

func (c *Client) ListEvents(ctx context.Context) []*github.Event {
	events, _, err := c.restClient.Activity.ListEventsPerformedByUser(ctx, os.Getenv(AATP_USER), true, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		log.Fatalf("list event failed: %s", err)
	}

	log.Printf("got %d events", len(events))
	return events
}

func (c *Client) GetProjectId(ctx context.Context, owner string, number int) string {
	var q struct {
		User struct {
			ProjectV2 struct {
				Id string
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"user(login: $owner)"`
	}
	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(number),
	}

	err := c.graphqlClient.Query(ctx, &q, variables)
	if err != nil {
		log.Fatalf("get project id failed: %s", err)
	}

	log.Printf("project id is: %s", q.User.ProjectV2.Id)
	return q.User.ProjectV2.Id
}

func (c *Client) AddToProject(ctx context.Context, projectId, contentUrl string) {
	var m struct {
		AddProjectV2DraftIssue struct {
			ProjectItem struct {
				Id string
			}
		} `graphql:"addProjectV2DraftIssue(input: $input)"`
	}

	type AddProjectV2DraftIssueInput struct {
		// The ID of the Project to add the item to. (Required.)
		ProjectID string `json:"projectId"`
		// The content id of the item (Issue or PullRequest). (Required.)
		Title string `json:"title"`
	}

	input := AddProjectV2DraftIssueInput{
		ProjectID: projectId,
		Title:     contentUrl,
	}

	err := c.graphqlClient.Mutate(ctx, &m, input, nil)
	if err != nil {
		// Print and ignore errors.
		log.Printf("add to project failed: %s", err)
	}
	log.Printf("content %s has been added in project %s", contentUrl, projectId)
}

func main() {
	ctx := context.Background()
	client := NewClient(ctx)

	// Get value from env
	user := os.Getenv(AATP_USER)

	// Get project id
	projectNumber, err := strconv.Atoi(os.Getenv(AATP_PROJECT_NUMBER))
	if err != nil {
		log.Fatalf("input project number is invalid: %s", os.Getenv(AATP_PROJECT_NUMBER))
	}
	projectId := client.GetProjectId(ctx, user, projectNumber)

	// List events
	events := client.ListEvents(ctx)

	// Filter events
	for _, event := range events {
		switch event.GetType() {
		case "IssuesEvent":
			payload := &github.IssueEvent{}
			err := json.Unmarshal(event.GetRawPayload(), &payload)
			if err != nil {
				log.Fatalf("unmarshal IssuesEvent failed: %s", err)
			}
			if payload.GetIssue().GetUser().GetLogin() != user && payload.GetIssue().GetAssignee().GetLogin() != user {
				continue
			}
			client.AddToProject(ctx, projectId, payload.GetIssue().GetHTMLURL())
		case "PullRequestEvent":
			payload := &github.PullRequestEvent{}
			err := json.Unmarshal(event.GetRawPayload(), &payload)
			if err != nil {
				log.Fatalf("unmarshal PullRequestEvent failed: %s", err)
			}
			if payload.GetPullRequest().GetUser().GetLogin() != os.Getenv(AATP_USER) {
				continue
			}
			client.AddToProject(ctx, projectId, payload.GetPullRequest().GetHTMLURL())
		default:
			log.Printf("event type is %s, ignore", event.GetType())
			continue
		}
	}
}
