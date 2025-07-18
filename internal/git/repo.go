package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoInfo represents repository information
type RepoInfo struct {
	Owner string
	Repo  string
}

// GetCurrentRepoInfo tries to get repository information from current directory
func GetCurrentRepoInfo() (*RepoInfo, error) {
	// Try to find .git directory
	gitDir, err := findGitDir()
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	// Try to get remote URL
	repoInfo, err := getRepoInfoFromRemote(gitDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository info: %w", err)
	}

	return repoInfo, nil
}

// findGitDir finds the .git directory from current directory upwards
func findGitDir() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(currentDir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return gitDir, nil
		}

		// Move up one directory
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached root directory
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf(".git directory not found")
}

// getRepoInfoFromRemote extracts repository info from git remote
func getRepoInfoFromRemote(gitDir string) (*RepoInfo, error) {
	// Try to get remote URL using git command
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try to read from .git/config
		configInfo, configErr := getRepoInfoFromConfig(gitDir)
		if configErr != nil {
			return nil, fmt.Errorf("no remote 'origin' found: %w", err)
		}
		return configInfo, nil
	}

	remoteURL := strings.TrimSpace(string(output))
	return parseRemoteURL(remoteURL)
}

// getRepoInfoFromConfig reads repository info from .git/config file
func getRepoInfoFromConfig(gitDir string) (*RepoInfo, error) {
	configPath := filepath.Join(gitDir, "config")
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git config: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	inRemoteOrigin := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check if we're in the [remote "origin"] section
		if strings.HasPrefix(line, "[remote \"origin\"]") {
			inRemoteOrigin = true
			continue
		}

		// Check if we've moved to a different section
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[remote \"origin\"]") {
			inRemoteOrigin = false
			continue
		}

		// If we're in the remote origin section and found the url
		if inRemoteOrigin && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			return parseRemoteURL(url)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading git config: %w", err)
	}

	return nil, fmt.Errorf("no remote origin found in git config")
}

// parseRemoteURL parses a git remote URL to extract owner and repo
func parseRemoteURL(url string) (*RepoInfo, error) {
	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Handle different URL formats
	if strings.HasPrefix(url, "https://github.com/") {
		// HTTPS format: https://github.com/owner/repo
		path := strings.TrimPrefix(url, "https://github.com/")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return &RepoInfo{
				Owner: parts[0],
				Repo:  parts[1],
			}, nil
		}
	} else if strings.HasPrefix(url, "git@github.com:") {
		// SSH format: git@github.com:owner/repo
		path := strings.TrimPrefix(url, "git@github.com:")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return &RepoInfo{
				Owner: parts[0],
				Repo:  parts[1],
			}, nil
		}
	} else if strings.Contains(url, "github.com") {
		// Try to extract from any GitHub URL
		parts := strings.Split(url, "/")
		for i, part := range parts {
			if part == "github.com" && i+2 < len(parts) {
				return &RepoInfo{
					Owner: parts[i+1],
					Repo:  parts[i+2],
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported remote URL format: %s", url)
}
