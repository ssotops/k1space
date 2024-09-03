package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/fatih/color"
)

func getVersion() string {
	// Try to get the GitHub release version
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// If not available, try to get the last commit hash
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// If neither is available, return "unknown"
	return "unknown"
}

func printIntro() {
	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 3)

	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	// Create the title and version strings
	title := titleStyle.Render("k1space")
	version := versionStyle.Render(getVersion())

	// Combine and print
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", version))
	fmt.Println()
}

func printVersionInfo(logger *log.Logger) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	// Local install information
	localVersion := getVersion()

	fmt.Println(titleStyle.Render("\nLocal Install:"))
	fmt.Printf("Version: %s\n", infoStyle.Render(localVersion))

	// Remote/latest version information
	remoteRelease, err := getLatestGitHubRelease("ssotops", "k1space")
	if err != nil {
		logger.Error("Error fetching remote version info", "error", err)
		return
	}

	fmt.Println(titleStyle.Render("\nRemote/Latest Version:"))
	fmt.Printf("Version: %s\n", infoStyle.Render(remoteRelease.TagName))
	fmt.Printf("Released: %s\n", infoStyle.Render(remoteRelease.PublishedAt.Format(time.RFC3339)))

	// Extract commit hash from release body if available
	commitHash := extractCommitHash(remoteRelease.Body)
	if commitHash != "" {
		fmt.Printf("Commit Hash: %s\n", infoStyle.Render(commitHash))
	} else {
		fmt.Println("Commit Hash: Not available")
	}
}

func printConfigPaths(logger *log.Logger) {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	fmt.Println(titleStyle.Render("\n📂 K1space Base Directory:"))
	fmt.Printf("   %s\n\n", pathStyle.Render(fmt.Sprintf("cd %s", baseDir)))

	fmt.Println(titleStyle.Render("📄 K1space Config Files:"))
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".hcl" || filepath.Base(path) == ".local.cloud.env") {
			fmt.Printf("   %s\n", pathStyle.Render(path))
		}
		return nil
	})
	if err != nil {
		logger.Error("Error walking through base directory", "error", err)
	}

	fmt.Println(titleStyle.Render("\n📁 K1space Cloud Directories:"))
	cloudDirs, err := os.ReadDir(baseDir)
	if err != nil {
		logger.Error("Error reading base directory", "error", err)
		return
	}
	for _, dir := range cloudDirs {
		if dir.IsDir() && dir.Name() != ".cache" && dir.Name() != ".repositories" {
			fmt.Printf("   %s\n", pathStyle.Render(filepath.Join(baseDir, dir.Name())))
		}
	}

	fmt.Println() // Add an extra newline for spacing
}

func upgradeK1space(logger *log.Logger) {
	logger.Info("Upgrading k1space...")

	// Define repository details
	repo := "ssotops/k1space"
	binary := "k1space"

	// Determine OS and architecture
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Fetch the latest release information
	logger.Info("Fetching latest release information...")
	releaseInfo, err := fetchLatestReleaseInfo(repo)
	if err != nil {
		logger.Error("Failed to fetch latest release information", "error", err)
		return
	}

	version := releaseInfo.TagName
	logger.Info("Latest version", "version", version)

	// Construct the download URL for the specific asset
	assetName := fmt.Sprintf("%s_%s_%s", binary, osName, arch)
	if osName == "windows" {
		assetName += ".exe"
	}
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, version, assetName)

	// Download the binary
	logger.Info("Downloading new version", "version", version, "os", osName, "arch", arch)
	tempFile, err := downloadBinary(downloadURL)
	if err != nil {
		logger.Error("Failed to download binary", "error", err)
		return
	}
	defer os.Remove(tempFile)

	// Make it executable (skip for Windows)
	if osName != "windows" {
		err = os.Chmod(tempFile, 0755)
		if err != nil {
			logger.Error("Failed to make binary executable", "error", err)
			return
		}
	}

	// Get the path of the current executable
	execPath, err := os.Executable()
	if err != nil {
		logger.Error("Failed to get current executable path", "error", err)
		return
	}

	// Replace the current binary with the new one
	err = os.Rename(tempFile, execPath)
	if err != nil {
		logger.Error("Failed to replace current binary", "error", err)
		return
	}

	logger.Info("k1space has been successfully upgraded!", "version", version)
}

func getLatestGitHubRelease(owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}

func extractCommitHash(releaseBody string) string {
	lines := strings.Split(releaseBody, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Commit:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Commit:"))
		}
	}
	return ""
}

func fetchLatestReleaseInfo(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}

func downloadBinary(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp("", "k1space-*")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func deleteEmptyDirs(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isEmpty(path) {
				os.Remove(path)
				log.Info("Deleted empty directory", "path", path)
			}
		}
		return nil
	})
}

func isEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) == 0
}

// New utility functions moved from kubefirst.go

func runCommand(cmd *exec.Cmd, dir, logFile string) error {
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, output)
	}
	return appendToLog(logFile, string(output))
}

func appendToLog(logFile, content string) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content + "\n")
	return err
}

func runCommandWithLiveOutput(command, dir, logFile string) error {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = dir

	// Create a pipe for the command's stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}

	// Create a pipe for the command's stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// Create a log file
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("error creating log file: %w", err)
	}
	defer f.Close()

	// Create a multi-writer to write to both console and log file
	multiWriter := io.MultiWriter(os.Stdout, f)

	// Start a goroutine to read from stdout pipe
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Fprintln(multiWriter, color.CyanString("  [stdout] ")+scanner.Text())
		}
	}()

	// Start a goroutine to read from stderr pipe
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintln(multiWriter, color.RedString("  [stderr] ")+scanner.Text())
		}
	}()

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

func runAndLogCommand(cmd *exec.Cmd, logFile string, textColor color.Attribute) error {
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("error creating log file: %w", err)
	}
	defer f.Close()

	// Create a pipe for capturing the command's output
	r, w := io.Pipe()

	// Set up a multi-writer for both the log file and the pipe
	cmd.Stdout = io.MultiWriter(f, w)
	cmd.Stderr = io.MultiWriter(f, os.Stderr)

	colorPrinter := color.New(textColor)

	// Start the command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// Read from the pipe and print colored output
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			colorPrinter.Println(scanner.Text())
		}
		r.Close()
	}()

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("error running command: %w", err)
	}

	w.Close()

	return nil
}

func getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error getting current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Add any other utility functions here as needed

func logOutput(serviceName string, reader io.Reader, logFile *os.File, printer *color.Color, logs *scrollingLog) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		timestamp := time.Now().Format("15:04:05")
		formattedLine := fmt.Sprintf("[%s] %s: %s", timestamp, printer.Sprint(serviceName), line)
		logFile.WriteString(formattedLine + "\n")
		logs.add(formattedLine)
	}
}

func waitForQuit() {
	reader := bufio.NewReader(os.Stdin)
	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			fmt.Println("Error reading input:", err)
			return
		}
		if char == 'q' || char == 'Q' {
			return
		}
	}
}

func getGlobalKubefirstPath() (string, error) {
    path, err := exec.LookPath("kubefirst")
    if err != nil {
        return "", fmt.Errorf("global kubefirst not found: %w", err)
    }
    return path, nil
}

func updateEnvFile(filePath, key, value string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading .local.cloud.env file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	updated := false
	for i, line := range lines {
		if strings.HasPrefix(line, "export "+key+"=") {
			lines[i] = fmt.Sprintf("export %s=\"%s\"", key, value)
			updated = true
			break
		}
	}

	if !updated {
		lines = append(lines, fmt.Sprintf("export %s=\"%s\"", key, value))
	}

	updatedContent := strings.Join(lines, "\n")
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing updated .local.cloud.env file: %w", err)
	}

	return nil
}
