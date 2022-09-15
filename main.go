package main

import (
	"context"
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

// ListIssues will list all issues of mine.
//
// GitHub V3 treat every PR as an issue, so we don't need to list PRs.
func (c *Client) ListIssues(ctx context.Context) []*github.Issue {
	var allIssues []*github.Issue

	opt := &github.IssueListOptions{
		Filter: "assigned,created",
		State:  "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	for {
		issues, resp, err := c.restClient.Issues.List(ctx, true, opt)
		if err != nil {
			log.Fatalf("list issues failed: %s", err)
		}
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allIssues
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
	issues := client.ListIssues(ctx)

	// Add into project.
	//
	// Issues have been filtered at GitHub server side.
	// It's safe for us to add into project directly.
	for _, issue := range issues {
		client.AddToProject(ctx, projectId, issue.GetHTMLURL())
	}
}
