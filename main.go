package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v47/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	AATP_TOKEN          = "AATP_TOKEN"
	AATP_USER           = "AATP_USER"
	AATP_PATH           = "AATP_PATH"
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
	addedIssues := make(map[string]struct{})

	sopt := &github.SearchOptions{
		Sort:      "updated",
		Order:     "desc",
		TextMatch: false,
	}
	since := WeekStart(time.Now().ISOWeek()).Format("2006-01-02")

	for {
		issues, resp, err := c.restClient.Search.Issues(ctx, fmt.Sprintf("is:issue involves:Xuanwo updated:>=%s", since), sopt)

		if err != nil {
			log.Fatalf("list issues failed: %s", err)
		}
		if issues == nil {
			break
		}
		for _, issue := range issues.Issues {
			issue := issue
			if _, ok := addedIssues[*issue.HTMLURL]; ok {
				continue
			}
			addedIssues[*issue.HTMLURL] = struct{}{}
			allIssues = append(allIssues, issue)
		}

		if resp.NextPage == 0 {
			break
		}
		sopt.Page = resp.NextPage
	}

	sopt = &github.SearchOptions{
		Sort:      "updated",
		Order:     "desc",
		TextMatch: false,
	}
	for {
		issues, resp, err := c.restClient.Search.Issues(ctx, fmt.Sprintf("is:pull-request involves:Xuanwo updated:>=%s", since), sopt)
		if err != nil {
			log.Fatalf("list issues failed: %s", err)
		}
		if issues == nil {
			break
		}
		for _, issue := range issues.Issues {
			issue := issue
			if _, ok := addedIssues[*issue.HTMLURL]; ok {
				continue
			}
			addedIssues[*issue.HTMLURL] = struct{}{}
			allIssues = append(allIssues, issue)
		}

		if resp.NextPage == 0 {
			break
		}
		sopt.Page = resp.NextPage
	}

	return allIssues
}

// title:: Iteration/6
// type:: [[Iteration]]
// date:: 2022-01-29 - 2022-02-11
//
// - content
func (c *Client) WriteMarkdown(ctx context.Context, issues []*github.Issue) string {
	w := &bytes.Buffer{}

	now := time.Now()
	year, week := now.ISOWeek()
	start, end := WeekStart(year, week), WeekStart(year, week).AddDate(0, 0, 6)

	w.WriteString(fmt.Sprintf("title:: Iteration/%d-%d\n", year, week))
	w.WriteString("type:: [[Iteration]]\n")
	w.WriteString(fmt.Sprintf("date:: %s - %s\n", start.Format("2006-01-02"), end.Format("2006-01-02")))
	w.WriteString("\n")

	m := map[string][]*github.Issue{}
	for _, issue := range issues {
		name := ""
		if issue.Repository == nil {
			name = strings.ReplaceAll(*issue.RepositoryURL, "https://api.github.com/repos/", "")
			name = fmt.Sprintf("[[%s]]", name)
		} else {
			name = fmt.Sprintf("[[%s/%s]]", *issue.Repository.Owner.Login, *issue.Repository.Name)
		}

		m[name] = append(m[name], issue)
	}

	repos := make([]string, 0, len(m))
	for k := range m {
		repos = append(repos, k)
	}
	sort.Strings(repos)

	for _, repo := range repos {
		w.WriteString(fmt.Sprintf("- %s\n", repo))

		sort.Slice(m[repo], func(i, j int) bool {
			return m[repo][i].UpdatedAt.Before(*m[repo][j].UpdatedAt)
		})
		for _, issue := range m[repo] {
			w.WriteString(fmt.Sprintf("  - [[%s]] %s [%s](%s)\n", issue.UpdatedAt.Format("2006-01-02"), *issue.State, *issue.Title, *issue.HTMLURL))
		}
	}

	return w.String()
}

func WeekStart(year, week int) time.Time {
	t := time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)

	// Roll back to Monday:
	if wd := t.Weekday(); wd == time.Sunday {
		t = t.AddDate(0, 0, -6)
	} else {
		t = t.AddDate(0, 0, -int(wd)+1)
	}

	// Difference in weeks:
	_, w := t.ISOWeek()
	t = t.AddDate(0, 0, (week-w)*7)

	return t
}

func main() {
	ctx := context.Background()
	client := NewClient(ctx)

	// List events
	issues := client.ListIssues(ctx)

	// Wirte into markdown
	bs := client.WriteMarkdown(ctx, issues)

	println(bs)

	now := time.Now()
	year, week := now.ISOWeek()

	f, err := os.Create(fmt.Sprintf("%s/Iteration___%d-%d.md", os.Getenv(AATP_PATH), year, week))
	if err != nil {
		log.Fatalf("create file: %v", err)
	}
	defer f.Close()

	_, err = f.WriteString(bs)
	if err != nil {
		log.Fatalf("write file: %v", err)
	}
}
