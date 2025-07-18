package github

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/ryo246912/gh-actions-dash/internal/models"
)

// ErrorType represents different types of GitHub API errors
type ErrorType string

const (
	ErrorTypeAuth       ErrorType = "authentication"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypePermission ErrorType = "permission"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeUnknown    ErrorType = "unknown"
)

// GitHubError represents a detailed GitHub API error
type GitHubError struct {
	Type    ErrorType
	Message string
	Details string
	Err     error
}

func (e *GitHubError) Error() string {
	return e.Message
}

func (e *GitHubError) Unwrap() error {
	return e.Err
}

// categorizeError categorizes the error based on HTTP status code and error message
func categorizeError(err error) *GitHubError {
	if err == nil {
		return nil
	}

	errorMsg := err.Error()

	// Check for authentication errors
	if strings.Contains(errorMsg, "401") || strings.Contains(errorMsg, "authentication") ||
		strings.Contains(errorMsg, "Bad credentials") || strings.Contains(errorMsg, "token") {
		return &GitHubError{
			Type:    ErrorTypeAuth,
			Message: "認証エラー: GitHub トークンが無効または期限切れです",
			Details: "gh auth login を実行してGitHubにログインしてください",
			Err:     err,
		}
	}

	// Check for permission errors
	if strings.Contains(errorMsg, "403") || strings.Contains(errorMsg, "Forbidden") {
		return &GitHubError{
			Type:    ErrorTypePermission,
			Message: "権限エラー: このリポジトリへのアクセス権限がありません",
			Details: "リポジトリが存在し、アクセス権限があることを確認してください",
			Err:     err,
		}
	}

	// Check for not found errors
	if strings.Contains(errorMsg, "404") || strings.Contains(errorMsg, "Not Found") {
		return &GitHubError{
			Type:    ErrorTypeNotFound,
			Message: "リポジトリまたはリソースが見つかりません",
			Details: "リポジトリ名とオーナー名が正しいことを確認してください",
			Err:     err,
		}
	}

	// Check for rate limit errors
	if strings.Contains(errorMsg, "429") || strings.Contains(errorMsg, "rate limit") {
		return &GitHubError{
			Type:    ErrorTypeRateLimit,
			Message: "API利用制限に達しました",
			Details: "しばらく待ってから再試行してください",
			Err:     err,
		}
	}

	// Check for network errors
	if strings.Contains(errorMsg, "connection") || strings.Contains(errorMsg, "timeout") ||
		strings.Contains(errorMsg, "network") || strings.Contains(errorMsg, "dns") {
		return &GitHubError{
			Type:    ErrorTypeNetwork,
			Message: "ネットワークエラー: GitHubに接続できません",
			Details: "インターネット接続を確認してください",
			Err:     err,
		}
	}

	// Unknown error
	return &GitHubError{
		Type:    ErrorTypeUnknown,
		Message: "予期しないエラーが発生しました",
		Details: errorMsg,
		Err:     err,
	}
}

// RetryConfig defines retry configuration
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
	}
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorMsg := err.Error()
	// Network errors that should be retried
	retryableErrors := []string{
		"connection",
		"timeout",
		"network",
		"dns",
		"dial",
		"i/o timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"server misbehaving",
		"502", // Bad Gateway
		"503", // Service Unavailable
		"504", // Gateway Timeout
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errorMsg), retryable) {
			return true
		}
	}

	return false
}

// retryWithBackoff executes a function with exponential backoff retry
func retryWithBackoff(config RetryConfig, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Only retry if it's a retryable error
		if !isRetryableError(err) {
			break
		}

		// Calculate delay with exponential backoff
		delay := config.InitialDelay * time.Duration(1<<attempt)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}

		time.Sleep(delay)
	}

	return lastErr
}

// Client wraps GitHub API client
type Client struct {
	restClient  api.RESTClient
	retryConfig RetryConfig
}

// NewClient creates a new GitHub API client
func NewClient() (*Client, error) {
	restClient, err := api.DefaultRESTClient()
	if err != nil {
		return nil, categorizeError(err)
	}

	return &Client{
		restClient:  *restClient,
		retryConfig: DefaultRetryConfig(),
	}, nil
}

// GetCurrentUser returns the current authenticated user
func (c *Client) GetCurrentUser() (string, error) {
	response := struct {
		Login string `json:"login"`
	}{}

	err := c.restClient.Get("user", &response)
	if err != nil {
		return "", categorizeError(err)
	}

	return response.Login, nil
}

// GetRepository returns repository information
func (c *Client) GetRepository(owner, repo string) (*models.Repository, error) {
	var repository models.Repository
	err := c.restClient.Get(fmt.Sprintf("repos/%s/%s", owner, repo), &repository)
	if err != nil {
		return nil, categorizeError(err)
	}

	return &repository, nil
}

// GetWorkflows returns all workflows for a repository
func (c *Client) GetWorkflows(owner, repo string) ([]models.Workflow, error) {
	response := struct {
		Workflows []models.Workflow `json:"workflows"`
	}{}

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(fmt.Sprintf("repos/%s/%s/actions/workflows", owner, repo), &response)
	})

	if err != nil {
		return nil, categorizeError(err)
	}

	return response.Workflows, nil
}

// GetWorkflowsPaginated returns workflows for a repository with pagination support
func (c *Client) GetWorkflowsPaginated(owner, repo string, page, perPage int) ([]models.Workflow, int, error) {
	response := struct {
		Workflows  []models.Workflow `json:"workflows"`
		TotalCount int               `json:"total_count"`
	}{}

	endpoint := fmt.Sprintf("repos/%s/%s/actions/workflows?page=%d&per_page=%d", owner, repo, page, perPage)

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(endpoint, &response)
	})

	if err != nil {
		return nil, 0, categorizeError(err)
	}

	return response.Workflows, response.TotalCount, nil
}

// GetWorkflowRuns returns workflow runs for a workflow
func (c *Client) GetWorkflowRuns(owner, repo string, workflowID int64) ([]models.WorkflowRun, error) {
	response := struct {
		WorkflowRuns []models.WorkflowRun `json:"workflow_runs"`
	}{}

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(fmt.Sprintf("repos/%s/%s/actions/workflows/%d/runs", owner, repo, workflowID), &response)
	})

	if err != nil {
		return nil, categorizeError(err)
	}

	return response.WorkflowRuns, nil
}

// GetWorkflowRunJobs returns jobs for a workflow run
func (c *Client) GetWorkflowRunJobs(owner, repo string, runID int64) ([]models.Job, error) {
	response := struct {
		Jobs []models.Job `json:"jobs"`
	}{}

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(fmt.Sprintf("repos/%s/%s/actions/runs/%d/jobs", owner, repo, runID), &response)
	})

	if err != nil {
		return nil, categorizeError(err)
	}

	return response.Jobs, nil
}

// GetAllWorkflowRuns returns all workflow runs for a repository (across all workflows)
func (c *Client) GetAllWorkflowRuns(owner, repo string) ([]models.WorkflowRun, error) {
	response := struct {
		WorkflowRuns []models.WorkflowRun `json:"workflow_runs"`
	}{}

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(fmt.Sprintf("repos/%s/%s/actions/runs", owner, repo), &response)
	})

	if err != nil {
		return nil, categorizeError(err)
	}

	return response.WorkflowRuns, nil
}

// GetAllWorkflowRunsPaginated returns workflow runs for a repository with pagination support
func (c *Client) GetAllWorkflowRunsPaginated(owner, repo string, page, perPage int) ([]models.WorkflowRun, int, error) {
	response := struct {
		WorkflowRuns []models.WorkflowRun `json:"workflow_runs"`
		TotalCount   int                  `json:"total_count"`
	}{}

	endpoint := fmt.Sprintf("repos/%s/%s/actions/runs?page=%d&per_page=%d", owner, repo, page, perPage)

	err := retryWithBackoff(c.retryConfig, func() error {
		return c.restClient.Get(endpoint, &response)
	})

	if err != nil {
		return nil, 0, categorizeError(err)
	}

	return response.WorkflowRuns, response.TotalCount, nil
}

// GetWorkflowRunLogs returns logs for a workflow run
func (c *Client) GetWorkflowRunLogs(owner, repo string, runID int64) (string, error) {
	// Try to get actual logs from GitHub API
	actualLogs, err := c.downloadWorkflowRunLogs(owner, repo, runID)
	if err != nil {
		// Fallback to job/step information if log download fails
		fallbackLogs, fallbackErr := c.getJobStepInfo(owner, repo, runID)
		if fallbackErr != nil {
			return "", categorizeError(fallbackErr)
		}

		// Add notice about log download failure
		var content strings.Builder
		content.WriteString("⚠️  ログダウンロードに失敗しました。ジョブ・ステップ情報を表示します。\n")
		content.WriteString("📋 エラー詳細: ")
		content.WriteString(err.Error())
		content.WriteString("\n\n")
		content.WriteString("💡 実際のログを確認するには、GitHub Web UIをご利用ください。\n")
		content.WriteString("🔗 https://github.com/")
		content.WriteString(owner)
		content.WriteString("/")
		content.WriteString(repo)
		content.WriteString("/actions/runs/")
		content.WriteString(fmt.Sprintf("%d", runID))
		content.WriteString("\n\n")
		content.WriteString("=" + strings.Repeat("=", 60) + "\n\n")
		content.WriteString(fallbackLogs)

		return content.String(), nil
	}

	return actualLogs, nil
}

// downloadWorkflowRunLogs downloads and extracts the actual logs from GitHub API
func (c *Client) downloadWorkflowRunLogs(owner, repo string, runID int64) (string, error) {
	// The GitHub API endpoint for workflow run logs
	endpoint := fmt.Sprintf("repos/%s/%s/actions/runs/%d/logs", owner, repo, runID)

	// Use gh CLI's HTTP client for authentication
	httpClient, err := api.DefaultHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Create HTTP client that doesn't follow redirects
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Make a request to get the redirect URL
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/%s", endpoint), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusFound {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Get the redirect URL
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no redirect location found")
	}

	// Download the ZIP file
	zipResp, err := http.Get(location)
	if err != nil {
		return "", fmt.Errorf("failed to download logs: %w", err)
	}
	defer func() {
		_ = zipResp.Body.Close()
	}()

	if zipResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download logs: status %d", zipResp.StatusCode)
	}

	// Read the ZIP file into memory
	zipData, err := io.ReadAll(zipResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read zip data: %w", err)
	}

	// Extract and parse the ZIP file
	return c.extractLogsFromZip(zipData)
}

// extractLogsFromZip extracts log contents from the ZIP file
func (c *Client) extractLogsFromZip(zipData []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to create zip reader: %w", err)
	}

	var logContent strings.Builder

	// Process each file in the ZIP
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// Open the file within the ZIP
		rc, err := file.Open()
		if err != nil {
			continue // Skip files that can't be opened
		}

		// Read the file content
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			continue // Skip files that can't be read
		}

		// Add file header and content
		logContent.WriteString(fmt.Sprintf("=== %s ===\n", file.Name))
		logContent.WriteString(string(content))
		logContent.WriteString("\n\n")
	}

	return logContent.String(), nil
}

// getJobStepInfo is the fallback method that returns job/step information
func (c *Client) getJobStepInfo(owner, repo string, runID int64) (string, error) {
	jobs, err := c.GetWorkflowRunJobs(owner, repo, runID)
	if err != nil {
		return "", fmt.Errorf("failed to get workflow run jobs: %w", err)
	}

	if len(jobs) == 0 {
		return "📋 このワークフロー実行にはジョブ情報がありません。\n💡 ワークフローが実行中の場合は、完了後に再度確認してください。", nil
	}

	var logContent strings.Builder
	logContent.WriteString(fmt.Sprintf("📊 ジョブ・ステップ情報 (合計: %d ジョブ)\n\n", len(jobs)))

	for i, job := range jobs {
		// Job header with status icon
		statusIcon := "○"
		switch job.Status {
		case "completed":
			switch job.Conclusion {
			case "success":
				statusIcon = "✅"
			case "failure":
				statusIcon = "❌"
			case "cancelled":
				statusIcon = "⏹️"
			case "skipped":
				statusIcon = "⏭️"
			}
		case "in_progress":
			statusIcon = "🔄"
		}

		logContent.WriteString(fmt.Sprintf("=== %s Job %d: %s ===\n", statusIcon, i+1, job.Name))
		logContent.WriteString(fmt.Sprintf("📋 Status: %s", job.Status))
		if job.Conclusion != "" {
			logContent.WriteString(fmt.Sprintf(" (%s)", job.Conclusion))
		}
		logContent.WriteString("\n")

		// Timing information
		if !job.StartedAt.IsZero() {
			logContent.WriteString(fmt.Sprintf("⏰ Started: %s\n", job.StartedAt.Format("2006-01-02 15:04:05")))
		}
		if !job.CompletedAt.IsZero() {
			logContent.WriteString(fmt.Sprintf("🏁 Completed: %s\n", job.CompletedAt.Format("2006-01-02 15:04:05")))
			if !job.StartedAt.IsZero() {
				duration := job.CompletedAt.Sub(job.StartedAt)
				logContent.WriteString(fmt.Sprintf("⏱️  Duration: %v\n", duration.Round(time.Second)))
			}
		}
		logContent.WriteString("\n")

		// Steps information
		if len(job.Steps) > 0 {
			logContent.WriteString(fmt.Sprintf("📋 Steps (%d):\n", len(job.Steps)))
			for j, step := range job.Steps {
				stepIcon := "○"
				switch step.Status {
				case "completed":
					switch step.Conclusion {
					case "success":
						stepIcon = "✅"
					case "failure":
						stepIcon = "❌"
					case "cancelled":
						stepIcon = "⏹️"
					case "skipped":
						stepIcon = "⏭️"
					}
				case "in_progress":
					stepIcon = "🔄"
				}

				logContent.WriteString(fmt.Sprintf("  %d. %s %s", j+1, stepIcon, step.Name))
				if step.Status != "" {
					logContent.WriteString(fmt.Sprintf(" (%s", step.Status))
					if step.Conclusion != "" {
						logContent.WriteString(fmt.Sprintf("/%s", step.Conclusion))
					}
					logContent.WriteString(")")
				}
				logContent.WriteString("\n")

				// Step timing
				if !step.StartedAt.IsZero() && !step.CompletedAt.IsZero() {
					duration := step.CompletedAt.Sub(step.StartedAt)
					logContent.WriteString(fmt.Sprintf("     ⏱️  Duration: %v\n", duration.Round(time.Second)))
				}
			}
		} else {
			logContent.WriteString("📋 No steps information available\n")
		}
		logContent.WriteString("\n")
	}

	return logContent.String(), nil
}
