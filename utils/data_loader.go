package utils

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	mcDataGen_DataURL = "https://github.com/reallyoldfogie/mc-data-gen/tree/master/data"
)

// ResolveDataPath determines if input is a URL or local path.
// If URL: downloads and caches the data, returns path to cached data
// If local path: validates it exists and returns the path
// If empty: returns defaultPath
func ResolveDataPath(input string, cacheDir string, defaultPath string) (string, error) {
	if input == "" {
		input = mcDataGen_DataURL
	}

	// Use default if input is empty
	if input == "" {
		input = defaultPath
	}

	// Check if it's a URL
	if isURL(input) {
		return downloadAndCache(input, cacheDir)
	}

	// It's a local path - validate it exists
	if err := validateLocalPath(input); err != nil {
		return "", fmt.Errorf("local path validation failed: %w", err)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return absPath, nil
}

// isURL checks if the string is an HTTP or HTTPS URL
func isURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// validateLocalPath checks if the path exists
func validateLocalPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// GitHubRepo represents parsed GitHub repository information
type GitHubRepo struct {
	Owner string
	Repo  string
	Ref   string // branch, tag, or commit
	Path  string // optional subdirectory path
}

// parseGitHubURL parses a GitHub URL and extracts repository information
// Supports formats:
// - https://github.com/owner/repo
// - https://github.com/owner/repo/tree/branch
// - https://github.com/owner/repo/tree/branch/path/to/dir
func parseGitHubURL(urlStr string) (*GitHubRepo, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	if u.Host != "github.com" {
		return nil, fmt.Errorf("not a github.com URL")
	}

	// Match pattern: /owner/repo[/tree/ref[/path...]]
	re := regexp.MustCompile(`^/([^/]+)/([^/]+)(?:/tree/([^/]+)(?:/(.+))?)?`)
	matches := re.FindStringSubmatch(u.Path)

	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid GitHub URL format")
	}

	repo := &GitHubRepo{
		Owner: matches[1],
		Repo:  matches[2],
		Ref:   "main", // default branch
	}

	if len(matches) > 3 && matches[3] != "" {
		repo.Ref = matches[3]
	}

	if len(matches) > 4 && matches[4] != "" {
		repo.Path = matches[4]
	}

	return repo, nil
}

// getGitHubArchiveURL converts a GitHub repository to its archive download URL
func getGitHubArchiveURL(repo *GitHubRepo) string {
	// GitHub archive URL format: https://github.com/owner/repo/archive/refs/heads/branch.zip
	// For tags: https://github.com/owner/repo/archive/refs/tags/tagname.zip
	// Simpler format that works for both: https://github.com/owner/repo/archive/{ref}.zip

	// return fmt.Sprintf("https://github.com/%s/%s/archive/%s.zip", repo.Owner, repo.Repo, repo.Ref)
	return fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", repo.Owner, repo.Repo, repo.Ref)
}

// getLatestCommitSHA queries the GitHub API for the latest commit SHA on a branch.
// Returns the full 40-character SHA string.
func getLatestCommitSHA(repo *GitHubRepo) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", repo.Owner, repo.Repo, repo.Ref)

	client := &http.Client{Timeout: 10 * time.Second}
	request, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github.sha")

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// findExistingCache looks for any previously cached directory that matches the
// given URL hash prefix. Returns the full path if found, or empty string.
func findExistingCache(cacheDir string, urlHash string) string {
	prefix := fmt.Sprintf("downloaded-%s-", urlHash)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(cacheDir, entry.Name())
		}
	}
	return ""
}

// cleanOldCache removes previously cached directories for the same URL hash,
// keeping only the directory specified by keepDir.
func cleanOldCache(cacheDir string, urlHash string, keepDir string) {
	prefix := fmt.Sprintf("downloaded-%s-", urlHash)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			fullPath := filepath.Join(cacheDir, entry.Name())
			if fullPath != keepDir {
				os.RemoveAll(fullPath)
			}
		}
	}
}

// downloadAndCache downloads data from URL and caches it locally.
// For GitHub URLs, the latest commit SHA is included in the cache directory
// name so that upstream changes are detected on subsequent runs.
func downloadAndCache(urlStr string, cacheDir string) (string, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	downloadURL := urlStr
	var githubSubPath string
	var ghRepo *GitHubRepo

	if parsed, err := parseGitHubURL(urlStr); err == nil {
		ghRepo = parsed
		downloadURL = getGitHubArchiveURL(ghRepo)
		githubSubPath = ghRepo.Path
		fmt.Printf("Detected GitHub repository: %s/%s (ref: %s)\n", ghRepo.Owner, ghRepo.Repo, ghRepo.Ref)
		if githubSubPath != "" {
			fmt.Printf("Will extract subdirectory: %s\n", githubSubPath)
		}
	}

	urlHashBytes := sha256.Sum256([]byte(urlStr))
	urlHash := fmt.Sprintf("%x", urlHashBytes[:8])

	// For GitHub repos, include the commit SHA in the cache key to detect updates
	if ghRepo != nil {
		return downloadAndCacheGitHub(ghRepo, downloadURL, githubSubPath, cacheDir, urlHash)
	}

	// Non-GitHub URL: cache based on URL hash only (no update detection)
	targetDir := filepath.Join(cacheDir, fmt.Sprintf("downloaded-%s", urlHash))

	if _, err := os.Stat(targetDir); err == nil {
		fmt.Printf("Using cached data from: %s\n", targetDir)
		dataPath, err := findDataDirectory(targetDir, githubSubPath)
		if err != nil {
			return "", fmt.Errorf("failed to locate data directory in cache: %w", err)
		}
		return dataPath, nil
	}

	fmt.Printf("Downloading data from: %s\n", downloadURL)

	extractedDir, err := downloadAndExtractZip(downloadURL, targetDir)
	if err != nil {
		return "", err
	}

	dataPath, err := findDataDirectory(extractedDir, githubSubPath)
	if err != nil {
		return "", fmt.Errorf("failed to locate data directory: %w", err)
	}
	return dataPath, nil
}

// downloadAndCacheGitHub handles caching for GitHub URLs by checking the
// remote commit SHA. The cache directory name is: downloaded-{urlHash}-{shortSHA}
func downloadAndCacheGitHub(ghRepo *GitHubRepo, downloadURL string, githubSubPath string, cacheDir string, urlHash string) (string, error) {
	commitSHA, err := getLatestCommitSHA(ghRepo)
	if err != nil {
		fmt.Printf("Warning: failed to check for updates: %v\n", err)
		// Fall back to any existing cache for this URL
		if existingDir := findExistingCache(cacheDir, urlHash); existingDir != "" {
			fmt.Printf("Using existing cached mc-data-gen from: %s\n", existingDir)
			dataPath, err := findDataDirectory(existingDir, githubSubPath)
			if err != nil {
				return "", fmt.Errorf("failed to locate data directory in cache: %w", err)
			}
			return dataPath, nil
		}
		// No cache and can't reach API; attempt download with unknown SHA
		commitSHA = "unknown"
	}

	shortSHA := commitSHA
	if len(commitSHA) > 12 {
		shortSHA = commitSHA[:12]
	}

	targetDir := filepath.Join(cacheDir, fmt.Sprintf("downloaded-%s-%s", urlHash, shortSHA))

	if _, err := os.Stat(targetDir); err == nil {
		fmt.Printf("Using cached mc-data-gen from: %s\n", targetDir)
		dataPath, err := findDataDirectory(targetDir, githubSubPath)
		if err != nil {
			return "", fmt.Errorf("failed to locate data directory in cache: %w", err)
		}
		return dataPath, nil
	}

	fmt.Printf("Downloading mc-data-gen data from: %s\n", downloadURL)

	extractedDir, err := downloadAndExtractZip(downloadURL, targetDir)
	if err != nil {
		return "", err
	}

	cleanOldCache(cacheDir, urlHash, targetDir)

	dataPath, err := findDataDirectory(extractedDir, githubSubPath)
	if err != nil {
		return "", fmt.Errorf("failed to locate data directory: %w", err)
	}
	return dataPath, nil
}

// downloadAndExtractZip downloads a zip file and extracts it
// Returns the extraction directory (not the data directory - caller should use findDataDirectory)
func downloadAndExtractZip(urlStr string, targetDir string) (string, error) {
	// Create temporary file for download
	tmpFile, err := os.CreateTemp("", "mc-data-gen-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download with retry
	if err := downloadWithRetry(urlStr, tmpFile, 3); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Extract zip
	if err := extractZip(tmpFile.Name(), targetDir); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	fmt.Printf("Successfully downloaded and extracted to: %s\n", targetDir)
	return targetDir, nil
}

// downloadWithRetry downloads a file with exponential backoff retry
func downloadWithRetry(urlStr string, dst io.Writer, maxRetries int) error {
	var lastErr error
	backoff := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("Retrying download (attempt %d/%d) after %v...\n", attempt+1, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
		}

		resp, err := http.Get(urlStr)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
			continue
		}

		_, err = io.Copy(dst, resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		// Success!
		return nil
	}

	return fmt.Errorf("download failed after %d attempts: %w", maxRetries, lastErr)
}

// extractZip extracts a zip archive to the target directory
func extractZip(zipPath string, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipFile(f, targetDir); err != nil {
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}
	}

	return nil
}

// extractZipFile extracts a single file from a zip archive
func extractZipFile(f *zip.File, targetDir string) error {
	// Build target path
	targetPath := filepath.Join(targetDir, f.Name)

	// Prevent zip slip vulnerability
	if !strings.HasPrefix(targetPath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path: %s", f.Name)
	}

	// Create directory if it's a directory entry
	if f.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, f.Mode())
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}

	// Extract file
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// findDataDirectory locates the actual data directory within extracted files
// Handles cases where zip has a top-level directory (e.g., mc-data-gen-main/data/)
// If subPath is provided (from GitHub URL), navigates to that subdirectory first
func findDataDirectory(rootDir string, subPath string) (string, error) {
	// GitHub archives extract to {repo-name}-{ref}/ directory
	// So we need to find that directory first
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted directory: %w", err)
	}

	// Start search from root or the single top-level directory (GitHub archive structure)
	searchRoot := rootDir
	if len(entries) == 1 && entries[0].IsDir() {
		// GitHub archives typically have a single top-level directory like "repo-main/"
		searchRoot = filepath.Join(rootDir, entries[0].Name())
	}

	// If subPath is specified (e.g., "data" from GitHub URL tree/branch/data)
	// navigate to that subdirectory
	if subPath != "" {
		searchRoot = filepath.Join(searchRoot, subPath)
		if _, err := os.Stat(searchRoot); err != nil {
			return "", fmt.Errorf("specified subdirectory not found: %s", subPath)
		}
	}

	// Check if searchRoot itself is valid (contains version directories)
	if isValidDataDir(searchRoot) {
		return searchRoot, nil
	}

	// Look for "data" subdirectory within searchRoot
	dataPath := filepath.Join(searchRoot, "data")
	if isValidDataDir(dataPath) {
		return dataPath, nil
	}

	// If searchRoot has only one directory, check inside it
	searchEntries, err := os.ReadDir(searchRoot)
	if err == nil && len(searchEntries) == 1 && searchEntries[0].IsDir() {
		innerPath := filepath.Join(searchRoot, searchEntries[0].Name())
		if isValidDataDir(innerPath) {
			return innerPath, nil
		}

		// Check for data/ inside that directory
		innerDataPath := filepath.Join(innerPath, "data")
		if isValidDataDir(innerDataPath) {
			return innerDataPath, nil
		}
	}

	// Return searchRoot as fallback
	return searchRoot, nil
}

// isValidDataDir checks if a directory looks like mc-data-gen data structure
// (contains version directories like 1.21.5/)
func isValidDataDir(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	// Look for version-like directories (e.g., "1.21.5")
	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), ".") {
			// Check if it has a "blocks" subdirectory
			blocksPath := filepath.Join(path, entry.Name(), "blocks")
			if _, err := os.Stat(blocksPath); err == nil {
				return true
			}
		}
	}

	return false
}
