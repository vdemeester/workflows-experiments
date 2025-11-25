package cherrypick

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"

	"github.com/google/go-github/v66/github"
)

// Config holds the configuration for cherry-pick operations
type Config struct {
	PRNumber     int
	Branches     []string
	RepoOwner    string
	RepoName     string
	GitUserName  string
	GitUserEmail string
}

// Result represents the outcome of a cherry-pick operation
type Result struct {
	Branch       string
	Success      bool
	ExistingPR   *github.PullRequest
	NewPR        *github.PullRequest
	Error        error
	ErrorMessage string
}

// GitRunner defines the interface for git operations
type GitRunner interface {
	Run(args ...string) error
}

// CommandGitRunner runs actual git commands
type CommandGitRunner struct{}

func (r *CommandGitRunner) Run(args ...string) error {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

// GitHubClient defines the interface for GitHub operations
type GitHubClient interface {
	GetPR(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error)
	FindExistingPR(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error)
	CreatePR(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error)
}

// DefaultGitHubClient wraps the go-github client
type DefaultGitHubClient struct {
	client *github.Client
}

func NewDefaultGitHubClient(client *github.Client) *DefaultGitHubClient {
	return &DefaultGitHubClient{client: client}
}

func (c *DefaultGitHubClient) GetPR(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	return pr, err
}

func (c *DefaultGitHubClient) FindExistingPR(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
	opts := &github.PullRequestListOptions{
		State: "all",
		Head:  fmt.Sprintf("%s:%s", owner, head),
		Base:  base,
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	}

	prs, _, err := c.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, err
	}

	if len(prs) > 0 {
		return prs[0], nil
	}

	return nil, nil
}

func (c *DefaultGitHubClient) CreatePR(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error) {
	newPR, _, err := c.client.PullRequests.Create(ctx, owner, repo, pr)
	return newPR, err
}

// Service handles cherry-pick operations
type Service struct {
	github GitHubClient
	git    GitRunner
}

// NewService creates a new cherry-pick service
func NewService(github GitHubClient, git GitRunner) *Service {
	return &Service{
		github: github,
		git:    git,
	}
}

// ProcessBranches processes multiple branches concurrently
func (s *Service) ProcessBranches(ctx context.Context, cfg *Config) []*Result {
	var wg sync.WaitGroup
	results := make([]*Result, len(cfg.Branches))

	for i, branch := range cfg.Branches {
		wg.Add(1)
		go func(index int, targetBranch string) {
			defer wg.Done()
			results[index] = s.ProcessBranch(ctx, cfg, targetBranch)
		}(i, branch)
	}

	wg.Wait()
	return results
}

// ProcessBranch handles cherry-picking to a single branch
func (s *Service) ProcessBranch(ctx context.Context, cfg *Config, targetBranch string) *Result {
	result := &Result{
		Branch:  targetBranch,
		Success: false,
	}

	log.Printf("🤖 Starting cherry-pick to %s...", targetBranch)

	// Get PR information
	pr, err := s.github.GetPR(ctx, cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		result.Error = err
		result.ErrorMessage = fmt.Sprintf("Failed to fetch PR #%d: %v", cfg.PRNumber, err)
		return result
	}

	// Check if PR is merged
	if pr.Merged == nil || !*pr.Merged {
		result.ErrorMessage = fmt.Sprintf("PR #%d is not merged yet (state: %s). Cherry-pick requires merged PRs.", cfg.PRNumber, pr.GetState())
		return result
	}

	mergeCommit := pr.GetMergeCommitSHA()
	log.Printf("Found merge commit: %s", mergeCommit)

	// Check if cherry-pick PR already exists
	cherryPickBranch := fmt.Sprintf("cherry-pick-%d-to-%s", cfg.PRNumber, targetBranch)
	existingPR, err := s.github.FindExistingPR(ctx, cfg.RepoOwner, cfg.RepoName, cherryPickBranch, targetBranch)
	if err != nil {
		log.Printf("Warning: error checking for existing PR: %v", err)
	}

	if existingPR != nil {
		log.Printf("ℹ️  Cherry-pick PR already exists: #%d", existingPR.GetNumber())
		result.Success = true
		result.ExistingPR = existingPR
		return result
	}

	// Perform git operations
	if err := s.performGitOperations(cfg, targetBranch, cherryPickBranch, mergeCommit); err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}

	// Create pull request
	title := fmt.Sprintf("Cherry-pick #%d to %s", cfg.PRNumber, targetBranch)
	body := fmt.Sprintf("Automatic cherry-pick of #%d to `%s`", cfg.PRNumber, targetBranch)

	newPR, err := s.github.CreatePR(ctx, cfg.RepoOwner, cfg.RepoName, &github.NewPullRequest{
		Title: &title,
		Body:  &body,
		Head:  &cherryPickBranch,
		Base:  &targetBranch,
	})

	if err != nil {
		result.Error = err
		result.ErrorMessage = fmt.Sprintf("Failed to create pull request: %v", err)
		return result
	}

	log.Printf("✅ Cherry-pick completed successfully! PR #%d created", newPR.GetNumber())
	result.Success = true
	result.NewPR = newPR
	return result
}

func (s *Service) performGitOperations(cfg *Config, targetBranch, cherryPickBranch, mergeCommit string) error {
	// Configure git
	if err := s.git.Run("config", "user.name", cfg.GitUserName); err != nil {
		return fmt.Errorf("failed to configure git user name: %w", err)
	}

	if err := s.git.Run("config", "user.email", cfg.GitUserEmail); err != nil {
		return fmt.Errorf("failed to configure git user email: %w", err)
	}

	// Fetch target branch
	log.Printf("Fetching target branch: %s...", targetBranch)
	if err := s.git.Run("fetch", "origin", targetBranch); err != nil {
		return fmt.Errorf("target branch '%s' does not exist or cannot be fetched: %w", targetBranch, err)
	}

	// Create new branch for cherry-pick
	log.Printf("Creating cherry-pick branch: %s...", cherryPickBranch)
	if err := s.git.Run("checkout", "-b", cherryPickBranch, fmt.Sprintf("origin/%s", targetBranch)); err != nil {
		return fmt.Errorf("failed to create cherry-pick branch: %w", err)
	}

	// Perform cherry-pick
	log.Printf("Cherry-picking commit %s...", mergeCommit)
	if err := s.git.Run("cherry-pick", "-m", "1", mergeCommit); err != nil {
		// Abort cherry-pick on failure
		_ = s.git.Run("cherry-pick", "--abort")
		return fmt.Errorf("cherry-pick failed due to conflicts or other errors: %w", err)
	}

	// Push the new branch
	log.Printf("Pushing cherry-pick branch...")
	if err := s.git.Run("push", "origin", cherryPickBranch); err != nil {
		return fmt.Errorf("failed to push cherry-pick branch: %w", err)
	}

	return nil
}

// ValidateConfig validates the cherry-pick configuration
func ValidateConfig(cfg *Config) error {
	if cfg.PRNumber == 0 {
		return fmt.Errorf("PR number is required")
	}

	if len(cfg.Branches) == 0 {
		return fmt.Errorf("at least one target branch is required")
	}

	if cfg.RepoOwner == "" || cfg.RepoName == "" {
		return fmt.Errorf("repository owner and name are required")
	}

	return nil
}
