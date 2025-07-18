package models

import "time"

// Workflow represents a GitHub Actions workflow
type Workflow struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	URL         string    `json:"url"`
	HTMLUrl     string    `json:"html_url"`
	BadgeURL    string    `json:"badge_url"`
	Path        string    `json:"path"`
	NodeID      string    `json:"node_id"`
}

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	NodeID       string          `json:"node_id"`
	CheckSuiteID int64           `json:"check_suite_id"`
	HeadBranch   string          `json:"head_branch"`
	HeadSha      string          `json:"head_sha"`
	Path         string          `json:"path"`
	RunNumber    int             `json:"run_number"`
	Event        string          `json:"event"`
	Status       string          `json:"status"`
	Conclusion   string          `json:"conclusion"`
	WorkflowID   int64           `json:"workflow_id"`
	URL          string          `json:"url"`
	HTMLURL      string          `json:"html_url"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	RunStartedAt time.Time       `json:"run_started_at"`
	JobsURL      string          `json:"jobs_url"`
	LogsURL      string          `json:"logs_url"`
	RunAttempt   int             `json:"run_attempt"`
	Actor        Actor           `json:"actor"`
	HeadCommit   Commit          `json:"head_commit"`
	Repository   Repository      `json:"repository"`
	PullRequests []PullRequest   `json:"pull_requests"`
}

// Job represents a job in a workflow run
type Job struct {
	ID          int64     `json:"id"`
	RunID       int64     `json:"run_id"`
	RunURL      string    `json:"run_url"`
	RunAttempt  int       `json:"run_attempt"`
	NodeID      string    `json:"node_id"`
	HeadSha     string    `json:"head_sha"`
	URL         string    `json:"url"`
	HTMLURL     string    `json:"html_url"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Name        string    `json:"name"`
	Steps       []Step    `json:"steps"`
}

// Step represents a step in a job
type Step struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	Number      int       `json:"number"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Actor represents a GitHub user
type Actor struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	NodeID    string `json:"node_id"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// Commit represents a git commit
type Commit struct {
	ID        string    `json:"id"`
	TreeID    string    `json:"tree_id"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Author    Author    `json:"author"`
	Committer Author    `json:"committer"`
}

// Author represents a git author
type Author struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// Repository represents a GitHub repository
type Repository struct {
	ID       int64  `json:"id"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    Actor  `json:"owner"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
	URL      string `json:"url"`
}

// PullRequest represents a GitHub pull request
type PullRequest struct {
	ID      int64  `json:"id"`
	Number  int    `json:"number"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	Head    struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"base"`
}