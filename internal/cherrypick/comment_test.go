package cherrypick

import (
	"strings"
	"testing"

	"github.com/google/go-github/v66/github"
)

func TestFormatResult_ExistingPR(t *testing.T) {
	poster := &CommentPoster{}

	result := &Result{
		Branch:  "release-1.0",
		Success: true,
		ExistingPR: &github.PullRequest{
			Number:  intPtr(456),
			HTMLURL: stringPtr("https://github.com/owner/repo/pull/456"),
		},
	}

	body := poster.formatResult(result)

	if !strings.Contains(body, "already exists") {
		t.Error("Expected 'already exists' in comment body")
	}

	if !strings.Contains(body, "release-1.0") {
		t.Error("Expected branch name in comment body")
	}

	if !strings.Contains(body, "#456") {
		t.Error("Expected PR number in comment body")
	}

	if !strings.Contains(body, "https://github.com/owner/repo/pull/456") {
		t.Error("Expected PR URL in comment body")
	}
}

func TestFormatResult_Success(t *testing.T) {
	poster := &CommentPoster{}

	result := &Result{
		Branch:  "release-1.0",
		Success: true,
		NewPR: &github.PullRequest{
			Number:  intPtr(789),
			HTMLURL: stringPtr("https://github.com/owner/repo/pull/789"),
		},
	}

	body := poster.formatResult(result)

	if !strings.Contains(body, "successful") {
		t.Error("Expected 'successful' in comment body")
	}

	if !strings.Contains(body, "release-1.0") {
		t.Error("Expected branch name in comment body")
	}

	if !strings.Contains(body, "https://github.com/owner/repo/pull/789") {
		t.Error("Expected PR URL in comment body")
	}
}

func TestFormatResult_Failure(t *testing.T) {
	poster := &CommentPoster{}

	result := &Result{
		Branch:       "release-1.0",
		Success:      false,
		ErrorMessage: "cherry-pick failed: conflicts detected",
	}

	body := poster.formatResult(result)

	if !strings.Contains(body, "failed") {
		t.Error("Expected 'failed' in comment body")
	}

	if !strings.Contains(body, "release-1.0") {
		t.Error("Expected branch name in comment body")
	}

	if !strings.Contains(body, "cherry-pick failed: conflicts detected") {
		t.Error("Expected error message in comment body")
	}

	if !strings.Contains(body, "Next steps") {
		t.Error("Expected 'Next steps' in comment body")
	}
}
