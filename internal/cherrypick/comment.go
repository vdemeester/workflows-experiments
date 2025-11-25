package cherrypick

import (
	"context"
	"fmt"
	"log"

	"github.com/google/go-github/v66/github"
)

// CommentPoster handles posting comments to GitHub
type CommentPoster struct {
	client      *github.Client
	repoOwner   string
	repoName    string
	issueNumber int
}

// NewCommentPoster creates a new comment poster
func NewCommentPoster(client *github.Client, repoOwner, repoName string, issueNumber int) *CommentPoster {
	return &CommentPoster{
		client:      client,
		repoOwner:   repoOwner,
		repoName:    repoName,
		issueNumber: issueNumber,
	}
}

// AddReaction adds a reaction to a comment
func (cp *CommentPoster) AddReaction(ctx context.Context, commentID int64, reaction string) error {
	if commentID == 0 {
		return nil
	}

	_, _, err := cp.client.Reactions.CreateIssueCommentReaction(ctx, cp.repoOwner, cp.repoName, commentID, reaction)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}
	return nil
}

// PostError posts an error comment
func (cp *CommentPoster) PostError(ctx context.Context, message string) error {
	if cp.issueNumber == 0 {
		return nil
	}

	body := fmt.Sprintf("❌ **Cherry-pick failed**: %s\n\n"+
		"**Usage**: `/cherry-pick <target-branch> [<target-branch2> ...]`\n"+
		"**Examples**:\n"+
		"- `/cherry-pick release-v1.0`\n"+
		"- `/cherry-pick release-v1.0 release-v1.1 release-v2.0`\n", message)

	return cp.postComment(ctx, body)
}

// PostResults posts result comments for each branch
func (cp *CommentPoster) PostResults(ctx context.Context, results []*Result) {
	if cp.issueNumber == 0 {
		return
	}

	for _, result := range results {
		body := cp.formatResult(result)
		if err := cp.postComment(ctx, body); err != nil {
			log.Printf("Error posting result comment for %s: %v", result.Branch, err)
		}
	}
}

func (cp *CommentPoster) formatResult(result *Result) string {
	if result.ExistingPR != nil {
		return fmt.Sprintf("ℹ️ **Cherry-pick to `%s` already exists!**\n\n"+
			"A pull request for this cherry-pick already exists: #%d\n\n"+
			"**PR**: %s\n",
			result.Branch, result.ExistingPR.GetNumber(), result.ExistingPR.GetHTMLURL())
	}

	if result.Success && result.NewPR != nil {
		return fmt.Sprintf("✅ **Cherry-pick to `%s` successful!**\n\n"+
			"A new pull request has been created to cherry-pick this change to `%s`.\n\n"+
			"**PR**: %s\n\n"+
			"Please review and merge the cherry-pick PR.\n",
			result.Branch, result.Branch, result.NewPR.GetHTMLURL())
	}

	return fmt.Sprintf("❌ **Cherry-pick to `%s` failed!**\n\n"+
		"The automatic cherry-pick to `%s` failed.\n\n"+
		"**Error:**\n"+
		"```\n%s\n```\n\n"+
		"**Next steps:**\n"+
		"- If the PR is not merged, merge it first and try again\n"+
		"- If there are conflicts, you'll need to manually cherry-pick this PR\n",
		result.Branch, result.Branch, result.ErrorMessage)
}

func (cp *CommentPoster) postComment(ctx context.Context, body string) error {
	_, _, err := cp.client.Issues.CreateComment(ctx, cp.repoOwner, cp.repoName, cp.issueNumber, &github.IssueComment{
		Body: &body,
	})
	return err
}
