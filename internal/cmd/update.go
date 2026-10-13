package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	updateCheckOnly bool
	updateForce     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates to Sift",
	Long:  "Check GitHub Releases for newer releases of Sift (sf) and update your local installation.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUpdate(cmd, args); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	},
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVarP(&updateCheckOnly, "check", "c", false, "Check for updates without downloading")
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "Force reinstallation even if already latest")
}

type gitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	currentVer := version
	if currentVer == "" {
		currentVer = "dev"
	}

	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("▸ Sift: Checking GitHub releases for updates..."))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/SadeepaNHerath/Spacefold/releases/latest", nil)
	if err != nil {
		return fmt.Errorf("failed to create update request: %w", err)
	}
	req.Header.Set("User-Agent", "Sift-CLI/"+currentVer)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network error checking releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(out, "✔ Sift is up to date (%s). No published releases found yet.\n", currentVer)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var rel gitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("failed to parse release metadata: %w", err)
	}

	latestVer := strings.TrimPrefix(rel.TagName, "v")
	cleanCurrent := strings.TrimPrefix(currentVer, "v")

	if cleanCurrent == latestVer && !updateForce {
		fmt.Fprintf(out, lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Sift is already on the latest version (%s).\n"), rel.TagName)
		return nil
	}

	fmt.Fprintf(out, "\n🚀 A new version of Sift is available: %s → %s\n", currentVer, rel.TagName)
	fmt.Fprintf(out, "Release page: %s\n\n", rel.HTMLURL)

	fmt.Fprintln(out, "To upgrade to the latest release, run:")
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51")).Render(
		"  curl -fsSL https://raw.githubusercontent.com/SadeepaNHerath/Spacefold/main/install.sh | bash",
	))
	fmt.Fprintln(out)

	return nil
}
