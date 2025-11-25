package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/go-github/v66/github"
	"github.com/vdemeester/workflows-experiments/internal/cherrypick"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, commentID := parseFlags()

	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(cfg.Token)

	// Create comment poster
	poster := cherrypick.NewCommentPoster(client, cfg.RepoOwner, cfg.RepoName, cfg.IssueNumber)

	// Add reaction to trigger comment
	if err := poster.AddReaction(ctx, commentID, "+1"); err != nil {
		log.Printf("Warning: %v", err)
	}

	// Validate configuration
	if err := cherrypick.ValidateConfig(&cfg.Config); err != nil {
		if postErr := poster.PostError(ctx, err.Error()); postErr != nil {
			log.Printf("Failed to post error comment: %v", postErr)
		}
		return err
	}

	// Create service with real implementations
	githubClient := cherrypick.NewDefaultGitHubClient(client)
	gitRunner := &cherrypick.CommandGitRunner{}
	service := cherrypick.NewService(githubClient, gitRunner)

	// Process all branches
	results := service.ProcessBranches(ctx, &cfg.Config)

	// Post results as comments
	poster.PostResults(ctx, results)

	// Exit with error if any cherry-pick failed
	for _, result := range results {
		if !result.Success && result.ExistingPR == nil {
			return fmt.Errorf("cherry-pick to %s failed", result.Branch)
		}
	}

	return nil
}

type cliConfig struct {
	cherrypick.Config
	Token       string
	IssueNumber int
}

func parseFlags() (cliConfig, int64) {
	var (
		prNumber     = flag.Int("pr-number", 0, "PR number to cherry-pick")
		branches     = flag.String("branches", "", "Comma-separated list of target branches")
		repo         = flag.String("repo", "", "Repository in owner/name format")
		commentID    = flag.Int64("comment-id", 0, "Comment ID to add reaction to")
		issueNumber  = flag.Int("issue-number", 0, "Issue/PR number to comment on")
		gitUserName  = flag.String("git-user-name", "Shortbrain bot", "Git user name")
		gitUserEmail = flag.String("git-user-email", "vincent+bot@sbr.pm", "Git user email")
	)

	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	if *repo == "" {
		log.Fatal("--repo is required")
	}

	parts := strings.SplitN(*repo, "/", 2)
	if len(parts) != 2 {
		log.Fatal("--repo must be in owner/name format")
	}

	branchList := []string{}
	if *branches != "" {
		branchList = strings.Split(*branches, ",")
		for i := range branchList {
			branchList[i] = strings.TrimSpace(branchList[i])
		}
	}

	cfg := cliConfig{
		Config: cherrypick.Config{
			PRNumber:     *prNumber,
			Branches:     branchList,
			RepoOwner:    parts[0],
			RepoName:     parts[1],
			GitUserName:  *gitUserName,
			GitUserEmail: *gitUserEmail,
		},
		Token:       token,
		IssueNumber: *issueNumber,
	}

	return cfg, *commentID
}
