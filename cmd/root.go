package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/ryo246912/gh-actions-dash/internal/git"
	"github.com/ryo246912/gh-actions-dash/internal/github"
	"github.com/ryo246912/gh-actions-dash/internal/tui"
	"github.com/spf13/cobra"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	owner string
	repo  string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-actions-dash",
	Short: "A TUI for GitHub Actions",
	Long:  `A terminal user interface for managing and viewing GitHub Actions workflows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize GitHub client
		client, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}

		// Verify authentication
		_, err = client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to authenticate with GitHub: %w", err)
		}

		// If no owner/repo specified, try to get from current directory
		if owner == "" || repo == "" {
			repoInfo, err := git.GetCurrentRepoInfo()
			if err != nil {
				return fmt.Errorf("failed to detect repository from current directory: %w\n\nPlease run this command in a git repository or specify owner and repo with --owner and --repo flags", err)
			}
			
			if owner == "" {
				owner = repoInfo.Owner
			}
			if repo == "" {
				repo = repoInfo.Repo
			}
		}

		// Create TUI app
		app := tui.NewApp(client, owner, repo)
		
		// Start the TUI
		p := tea.NewProgram(app, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running TUI: %w", err)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&owner, "owner", "o", "", "Repository owner")
	rootCmd.Flags().StringVarP(&repo, "repo", "r", "", "Repository name")
}

// parseRepoFlag parses a repository flag in the format "owner/repo"
func parseRepoFlag(repoFlag string) (string, string, error) {
	parts := strings.Split(repoFlag, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository format, expected 'owner/repo'")
	}
	return parts[0], parts[1], nil
}