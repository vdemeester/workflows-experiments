package cherrypick

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-github/v66/github"
)

// Mock implementations for testing

type mockGitHubClient struct {
	getPR            func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error)
	findExistingPR   func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error)
	createPR         func(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error)
}

func (m *mockGitHubClient) GetPR(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	if m.getPR != nil {
		return m.getPR(ctx, owner, repo, number)
	}
	return nil, errors.New("not implemented")
}

func (m *mockGitHubClient) FindExistingPR(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
	if m.findExistingPR != nil {
		return m.findExistingPR(ctx, owner, repo, head, base)
	}
	return nil, nil
}

func (m *mockGitHubClient) CreatePR(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error) {
	if m.createPR != nil {
		return m.createPR(ctx, owner, repo, pr)
	}
	return nil, errors.New("not implemented")
}

type mockGitRunner struct {
	commands [][]string
	runFunc  func(args ...string) error
}

func (m *mockGitRunner) Run(args ...string) error {
	m.commands = append(m.commands, args)
	if m.runFunc != nil {
		return m.runFunc(args...)
	}
	return nil
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

// Tests

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				PRNumber:  123,
				Branches:  []string{"main"},
				RepoOwner: "owner",
				RepoName:  "repo",
			},
			wantErr: false,
		},
		{
			name: "missing PR number",
			cfg: &Config{
				Branches:  []string{"main"},
				RepoOwner: "owner",
				RepoName:  "repo",
			},
			wantErr: true,
		},
		{
			name: "missing branches",
			cfg: &Config{
				PRNumber:  123,
				RepoOwner: "owner",
				RepoName:  "repo",
			},
			wantErr: true,
		},
		{
			name: "missing repo owner",
			cfg: &Config{
				PRNumber: 123,
				Branches: []string{"main"},
				RepoName: "repo",
			},
			wantErr: true,
		},
		{
			name: "missing repo name",
			cfg: &Config{
				PRNumber:  123,
				Branches:  []string{"main"},
				RepoOwner: "owner",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessBranch_PRNotMerged(t *testing.T) {
	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged: boolPtr(false),
				State:  stringPtr("open"),
			}, nil
		},
	}

	mockGit := &mockGitRunner{}
	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:  123,
		RepoOwner: "owner",
		RepoName:  "repo",
	}

	result := service.ProcessBranch(context.Background(), cfg, "main")

	if result.Success {
		t.Error("Expected failure for unmerged PR")
	}

	if result.ErrorMessage == "" {
		t.Error("Expected error message for unmerged PR")
	}

	if len(mockGit.commands) > 0 {
		t.Error("Expected no git commands for unmerged PR")
	}
}

func TestProcessBranch_ExistingPR(t *testing.T) {
	existingPR := &github.PullRequest{
		Number:  intPtr(456),
		HTMLURL: stringPtr("https://github.com/owner/repo/pull/456"),
	}

	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged:         boolPtr(true),
				MergeCommitSHA: stringPtr("abc123"),
			}, nil
		},
		findExistingPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
			return existingPR, nil
		},
	}

	mockGit := &mockGitRunner{}
	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:  123,
		RepoOwner: "owner",
		RepoName:  "repo",
	}

	result := service.ProcessBranch(context.Background(), cfg, "release")

	if !result.Success {
		t.Error("Expected success when existing PR found")
	}

	if result.ExistingPR == nil {
		t.Error("Expected ExistingPR to be set")
	}

	if result.ExistingPR.GetNumber() != 456 {
		t.Errorf("Expected PR #456, got #%d", result.ExistingPR.GetNumber())
	}

	if len(mockGit.commands) > 0 {
		t.Error("Expected no git commands when PR already exists")
	}
}

func TestProcessBranch_Success(t *testing.T) {
	newPR := &github.PullRequest{
		Number:  intPtr(789),
		HTMLURL: stringPtr("https://github.com/owner/repo/pull/789"),
	}

	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged:         boolPtr(true),
				MergeCommitSHA: stringPtr("abc123"),
			}, nil
		},
		findExistingPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
			return nil, nil
		},
		createPR: func(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error) {
			return newPR, nil
		},
	}

	mockGit := &mockGitRunner{}
	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:     123,
		RepoOwner:    "owner",
		RepoName:     "repo",
		GitUserName:  "Test Bot",
		GitUserEmail: "bot@test.com",
	}

	result := service.ProcessBranch(context.Background(), cfg, "release")

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.ErrorMessage)
	}

	if result.NewPR == nil {
		t.Error("Expected NewPR to be set")
	}

	if result.NewPR.GetNumber() != 789 {
		t.Errorf("Expected PR #789, got #%d", result.NewPR.GetNumber())
	}

	// Verify git commands were called
	// Should have: config (user.name), config (user.email), fetch, checkout, cherry-pick, push
	expectedNumCommands := 6
	if len(mockGit.commands) != expectedNumCommands {
		t.Errorf("Expected %d git commands, got %d", expectedNumCommands, len(mockGit.commands))
	}

	// Verify config commands (first two should be config)
	if len(mockGit.commands) > 0 && mockGit.commands[0][0] != "config" {
		t.Error("Expected first command to be 'config'")
	}
	if len(mockGit.commands) > 1 && mockGit.commands[1][0] != "config" {
		t.Error("Expected second command to be 'config'")
	}
}

func TestProcessBranch_GitFetchFails(t *testing.T) {
	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged:         boolPtr(true),
				MergeCommitSHA: stringPtr("abc123"),
			}, nil
		},
		findExistingPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
			return nil, nil
		},
	}

	mockGit := &mockGitRunner{
		runFunc: func(args ...string) error {
			if args[0] == "fetch" {
				return errors.New("fetch failed: branch not found")
			}
			return nil
		},
	}

	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:     123,
		RepoOwner:    "owner",
		RepoName:     "repo",
		GitUserName:  "Test Bot",
		GitUserEmail: "bot@test.com",
	}

	result := service.ProcessBranch(context.Background(), cfg, "nonexistent")

	if result.Success {
		t.Error("Expected failure when git fetch fails")
	}

	if result.Error == nil {
		t.Error("Expected error to be set")
	}
}

func TestProcessBranch_CherryPickFails(t *testing.T) {
	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged:         boolPtr(true),
				MergeCommitSHA: stringPtr("abc123"),
			}, nil
		},
		findExistingPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
			return nil, nil
		},
	}

	mockGit := &mockGitRunner{
		runFunc: func(args ...string) error {
			if args[0] == "cherry-pick" {
				return errors.New("cherry-pick failed: conflicts")
			}
			return nil
		},
	}

	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:     123,
		RepoOwner:    "owner",
		RepoName:     "repo",
		GitUserName:  "Test Bot",
		GitUserEmail: "bot@test.com",
	}

	result := service.ProcessBranch(context.Background(), cfg, "release")

	if result.Success {
		t.Error("Expected failure when cherry-pick fails")
	}

	if result.Error == nil {
		t.Error("Expected error to be set")
	}

	// Verify abort was called
	foundAbort := false
	for _, cmd := range mockGit.commands {
		if len(cmd) >= 2 && cmd[0] == "cherry-pick" && cmd[1] == "--abort" {
			foundAbort = true
			break
		}
	}

	if !foundAbort {
		t.Error("Expected cherry-pick --abort to be called")
	}
}

func TestProcessBranches_Concurrent(t *testing.T) {
	mockGH := &mockGitHubClient{
		getPR: func(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
			return &github.PullRequest{
				Merged:         boolPtr(true),
				MergeCommitSHA: stringPtr("abc123"),
			}, nil
		},
		findExistingPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
			return nil, nil
		},
		createPR: func(ctx context.Context, owner, repo string, pr *github.NewPullRequest) (*github.PullRequest, error) {
			return &github.PullRequest{
				Number:  intPtr(1),
				HTMLURL: stringPtr("https://github.com/owner/repo/pull/1"),
			}, nil
		},
	}

	mockGit := &mockGitRunner{}
	service := NewService(mockGH, mockGit)

	cfg := &Config{
		PRNumber:     123,
		Branches:     []string{"release-1.0", "release-2.0", "release-3.0"},
		RepoOwner:    "owner",
		RepoName:     "repo",
		GitUserName:  "Test Bot",
		GitUserEmail: "bot@test.com",
	}

	results := service.ProcessBranches(context.Background(), cfg)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	for i, result := range results {
		if result.Branch != cfg.Branches[i] {
			t.Errorf("Result %d: expected branch %s, got %s", i, cfg.Branches[i], result.Branch)
		}

		if !result.Success {
			t.Errorf("Result %d: expected success for branch %s", i, result.Branch)
		}
	}
}
