package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"golang.org/x/oauth2"

	"github.com/genuinetools/pkg/cli"
	"github.com/google/go-github/github"
	"github.com/jessfraz/secping/version"

	//"github.com/justaugustus/ghnag/version"
	"github.com/sirupsen/logrus"
)

const (
	// BANNER is what is printed for help/info output.
	BANNER = `ghnag [OPTIONS] [REPO] [REPO...]

 .
 Version: %s
 Build: %s

`
)

var (
	token string

	debug bool

	// This list of organizations comes from:
	// https://github.com/kubernetes/community/blob/master/org-owners-guide.md#current-organizations-in-use
	orgs = []string{
		"kubernetes",
		/*
			"kubernetes",
			"kubernetes-client",
			"kubernetes-csi",
			"kubernetes-incubator",
			"kubernetes-retired", // maybe just ignore this one
			"kubernetes-sig-testing",
			"kubernetes-sigs",
		*/
	}

	defaultRepos = []string{
		"features",
	}

	// issueTitle   = "Create a SECURITY_CONTACTS file."
	comment = `This feature current has no milestone, so we'd like to check in and see if there are any plans for this in Kubernetes 1.12.

If so, please ensure that this issue is up-to-date with ALL of the following information:
- One-line feature description (can be used as a release note):
- Primary contact (assignee):
- Responsible SIGs:
- Design proposal link (community repo):
- Link to e2e and/or unit tests:
- Reviewer(s) - (for LGTM) recommend having 2+ reviewers (at least one from code-area OWNERS file) agreed to review. Reviewers from multiple companies preferred:
- Approver (likely from SIG/area to which feature belongs):
- Feature target (which target equals to which milestone):
	- Alpha release target (x.y)
	- Beta release target (x.y)
	- Stable release target (x.y)

Set the following:
- Description
- Assignee(s)
- Labels:
	- stage/{alpha,beta,stable}
	- sig/*
	- kind/feature

Once this feature is appropriately updated, please explicitly ping @justaugustus, @kacole2, @robertsandoval, @rajendar38 to note that it is ready to be included in the [Features Tracking Spreadsheet for Kubernetes 1.12](http://bit.ly/k8s112-features).

---

### Please note that Features Freeze is **tomorrow, July 31st**, after which any incomplete Feature issues will require an [Exception request](https://github.com/kubernetes/features/blob/master/EXCEPTIONS.md) to be accepted into the milestone.

#### In addition, please be aware of the following relevant deadlines:
- Docs deadline (open placeholder PRs): 8/21
- Test case freeze: 8/28

Please make sure all PRs for features have relevant release notes included as well.

Happy shipping!

P.S. This was sent via automation
`

	repoListOptions = github.IssueListByRepoOptions{
		State: "open",
		// TODO: Milestone search doesn't work for version e.g., "v1.12"
		Milestone: "none",
		// TODO: Add support for label filters
		Labels: []string{
			//"",
		},
		ListOptions: github.ListOptions{
			PerPage: 1000,
		},
	}

	excludedLabels = []string{
		"tracked/no",
	}
)

func main() {
	// Create a new cli program.
	p := cli.NewProgram()
	p.Name = "ghnag"
	p.Description = "A tool for creating issue comments by label & milestone"

	// Set the GitCommit and Version.
	p.GitCommit = version.GITCOMMIT
	p.Version = version.VERSION

	// Setup the global flags.
	p.FlagSet = flag.NewFlagSet("global", flag.ExitOnError)
	p.FlagSet.StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub API token (or env var GITHUB_TOKEN)")

	p.FlagSet.BoolVar(&debug, "d", false, "enable debug logging")

	// Set the before function.
	p.Before = func(ctx context.Context) error {
		// Set the log level.
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		if token == "" {
			return errors.New("GitHub token cannot be empty")
		}

		return nil
	}

	// Set the main program action.
	p.Action = func(ctx context.Context, repos []string) error {
		// On ^C, or SIGTERM handle exit.
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		signal.Notify(c, syscall.SIGTERM)
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		go func() {
			for sig := range c {
				logrus.Infof("Received %s, exiting.", sig.String())
				cancel()
				os.Exit(0)
			}
		}()

		// Create the http client.
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)

		// Create the github client.
		client := github.NewClient(tc)

		fmt.Printf("Orgs: %v, Repos: %v\n", len(orgs), len(defaultRepos))

		issueList, err := getIssueList(ctx, client, orgs[0], defaultRepos[0], &repoListOptions)

		if err == nil {
			fmt.Printf("Issue count: %v\n", len(issueList))
			nag(ctx, client, comment, issueList)
		}

		/*
			if err != nil {
				return nil
			}
		*/
		/*
			// If the user passed a repo or repos, just get the contacts for those.
			for _, repo := range repos {
				// Parse git repo for username and repo name.
				r := strings.SplitN(repo, "/", 2)
				if len(r) < 2 {
					logrus.WithFields(logrus.Fields{
						"repo": repo,
					}).Fatal("Repository name could not be parsed. Try something like: kubernetes/kubernetes")
				}

				_, err := getIssueList(ctx, client, orgs[0], repos[0], &repoListOptions)

				if err != nil {
					return err
				}

				// nag := nag(ctx, client, owner, repo, comment, number, issues)

				/*
					// Get the security contacts for the repository.
					if err := getSecurityContactsForRepo(ctx, client, r[0], r[1]); err != nil {
						logrus.WithFields(logrus.Fields{
							"repo": repo,
						}).Fatal(err)
					}
			}
		*/

		/*
			if len(repos) > 0 {
				// Return early if the user specified specific repositories,
				// as we don't want to also return all of them.
				return nil
			}

			// The user did not pass a specific repo so get all.
			for _, org := range orgs {
				page := 1
				perPage := 100
				if err := getRepositories(ctx, client, page, perPage, org); err != nil {
					logrus.WithFields(logrus.Fields{
						"org": org,
					}).Fatal(err)
				}
			}
		*/

		return nil
	}

	// Run our program.
	p.Run()
}

func getIssueList(ctx context.Context, client *github.Client, owner, repo string, opt *github.IssueListByRepoOptions) ([]*github.Issue, error) {

	var (
		filteredIssues []*github.Issue
		addIssue       bool
	)

	issues, _, err := client.Issues.ListByRepo(ctx, owner, repo, opt)

	if err != nil {
		return nil, fmt.Errorf("listing issues in %s/%s failed: %v", owner, repo, err)
	}

	/*
		// Try to match the title to the issue.
		for _, issue := range issues {
			if issue.GetTitle() == issueTitle {
				return issue, nil
			}
		}
	*/

	for _, issue := range issues {
		addIssue = true
		fmt.Printf("Checking %v\n", *issue.Number)

		for _, label := range excludedLabels {
			for _, issueLabel := range issue.Labels {
				fmt.Printf("Checking %s against %s\n", label, *issueLabel.Name)

				if label == *issueLabel.Name {
					addIssue = false
					fmt.Println("Issue has excluded label. Skipping...")
					break
				}
			}
		}

		if addIssue {
			fmt.Printf("Adding %v to list\n", *issue.Number)
			filteredIssues = append(filteredIssues, issue)
			fmt.Println("=================================\n")
		}
	}

	fmt.Printf("Issue count: %v\n", len(filteredIssues))
	return filteredIssues, err
}

// nag comments on a set of issues according to a template
func nag(ctx context.Context, client *github.Client, comment string, issues []*github.Issue) {
	re := regexp.MustCompile(`https://api.github.com/repos/([\w]+)/([\w]+)`)
	for _, issue := range issues {
		// TODO: Process owner, repo, issue number
		match := re.FindStringSubmatch(*issue.RepositoryURL)
		owner, repo := match[1], match[2]
		issueNumber := *issue.Number
		fmt.Printf("Owner: %v, Repo: %v, Issue #: %v", owner, repo, issueNumber)

		_, _, err := client.Issues.CreateComment(ctx, owner, repo, issueNumber, &github.IssueComment{
			Body: &comment,
		})
		fmt.Println("=================================\n")

		if err != nil {
			fmt.Errorf("Error: %v", err)
		}
	}
	return
}
