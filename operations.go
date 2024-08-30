package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/fatih/color"
)

var (
	consolePrinter      = color.New(color.FgCyan)
	kubefirstAPIPrinter = color.New(color.FgMagenta)
	kubefirstPrinter    = color.New(color.FgYellow)
)

const maxLogLines = 100
const kubefirstAPISetupScript = `#!/bin/bash
set -e

LOG_FILE="${HOME}/.ssot/k1space/.logs/kubefirst-api-setup.log"
API_DIR="${HOME}/.ssot/k1space/.repositories/kubefirst-api"

exec > >(tee -a "${LOG_FILE}") 2>&1

echo "Starting setup script"
echo "Current working directory: $(pwd)"
echo "Log file: ${LOG_FILE}"
echo "API directory: ${API_DIR}"

cd "${API_DIR}"

# Check for required tools
for cmd in go k3d kubectl make air swag; do
    if ! command -v $cmd &> /dev/null; then
        echo "ERROR: $cmd could not be found. Please install it and try again."
        exit 1
    fi
done

echo "Installing required tools..."
go install github.com/air-verse/air@latest
go install github.com/swaggo/swag/cmd/swag@latest

# Set environment variables
export K1_LOCAL_DEBUG=true
export K1_LOCAL_KUBECONFIG_PATH=$(k3d kubeconfig get dev)
export CLUSTER_ID="local-dev"
export CLUSTER_TYPE="k3d"
export INSTALL_METHOD="local"
export K1_ACCESS_TOKEN="local-dev-token"
export IS_CLUSTER_ZERO=true

# Create and source .env file
ENV_FILE="${API_DIR}/.env"
echo "Checking for .env file at: ${ENV_FILE}"
if [ ! -f "${ENV_FILE}" ]; then
    echo "Creating .env file from .env.example"
    cp "${API_DIR}/.env.example" "${ENV_FILE}"
    echo "Created .env file from .env.example"
    echo "Please edit ${ENV_FILE} with your specific values, then press Enter to continue."
    read
else
    echo ".env file already exists"
fi
set -a
source "${ENV_FILE}"
set +a

echo "Environment variables set:"
env | grep -E 'K1_|CLUSTER_|KUBECONFIG' | sed 's/^/  /'

echo "Creating necessary Kubernetes resources..."
kubectl create namespace kubefirst --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic kubefirst-clusters --from-literal=clusters='{}' -n kubefirst --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic kubefirst-catalog --from-literal=catalog='{}' -n kubefirst --dry-run=client -o yaml | kubectl apply -f -

echo "Updating Swagger documentation..."
make updateswagger

echo "Building the kubefirst-api binary..."
make build

echo "Starting kubefirst-api with air for live reloading..."
air
`

type scrollingLog struct {
	lines []string
	mu    sync.Mutex
}

func (sl *scrollingLog) add(line string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.lines = append(sl.lines, line)
	if len(sl.lines) > maxLogLines {
		sl.lines = sl.lines[len(sl.lines)-maxLogLines:]
	}
}

func (sl *scrollingLog) getLastN(n int) []string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.lines) <= n {
		return sl.lines
	}
	return sl.lines[len(sl.lines)-n:]
}

func (sl *scrollingLog) get() string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return strings.Join(sl.lines, "\n")
}

func provisionCluster() {
	log.Info("Starting provisionCluster function")
	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the index.hcl file exists and is correctly formatted.")
		return
	}

	log.Info("Index file loaded successfully", "version", indexFile.Version, "lastUpdated", indexFile.LastUpdated)
	log.Info("Configs found", "count", len(indexFile.Configs))

	// List available configs
	var selectedConfig string
	configOptions := make([]huh.Option[string], 0, len(indexFile.Configs))
	for config, details := range indexFile.Configs {
		log.Info("Config found", "name", config, "fileCount", len(details.Files))
		configOptions = append(configOptions, huh.NewOption(config, config))
	}

	if len(configOptions) == 0 {
		log.Warn("No configurations found in the index file")
		fmt.Println("No configurations available. Please create a configuration first.")
		fmt.Println("You can create a configuration using the 'Config' -> 'Create Config' option in the main menu.")
		return
	}

	log.Info("Presenting config selection to user", "optionCount", len(configOptions))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a configuration").
				Options(configOptions...).
				Value(&selectedConfig),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in config selection", "error", err)
		return
	}
	log.Info("User selected config", "selectedConfig", selectedConfig)

	// Display files in selected config
	files := indexFile.Configs[selectedConfig].Files
	var selectedFile string
	fileOptions := make([]huh.Option[string], 0, len(files))
	for _, file := range files {
		// Remove any surrounding quotes and use only the base filename
		cleanFile := strings.Trim(file, "\"")
		baseFile := filepath.Base(cleanFile)
		fileOptions = append(fileOptions, huh.NewOption(baseFile, cleanFile))
	}

	log.Info("Presenting file selection to user", "optionCount", len(fileOptions))
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a file").
				Options(fileOptions...).
				Value(&selectedFile),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error selecting file", "error", err)
		return
	}
	log.Info("User selected file", "selectedFile", selectedFile)

	// Read and display file contents
	content, err := os.ReadFile(filepath.Clean(selectedFile))
	if err != nil {
		log.Error("Error reading file", "error", err)
		return
	}

	fmt.Printf("Contents of %s:\n\n%s\n\n", filepath.Base(selectedFile), string(content))

	// Prompt user to run 00-init.sh
	var confirmRun bool
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you want to run 00-init.sh to provision the cluster?").
				Value(&confirmRun),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in confirmation prompt", "error", err)
		return
	}

	if confirmRun {
		// Summarize the provision cluster process
		fmt.Println("\nProvisioning Cluster Summary:")
		fmt.Printf("- Configuration: %s\n", selectedConfig)
		fmt.Printf("- Init Script: %s\n", filepath.Join(filepath.Dir(selectedFile), "00-init.sh"))
		fmt.Printf("- Kubefirst Script: %s\n", filepath.Join(filepath.Dir(selectedFile), "01-kubefirst-cloud.sh"))
		fmt.Printf("- Environment File: %s\n", filepath.Join(filepath.Dir(selectedFile), ".local.cloud.env"))

		// Final confirmation
		var finalConfirm bool
		form = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to proceed with provisioning the cluster?").
					Value(&finalConfirm),
			),
		)

		err = form.Run()
		if err != nil {
			log.Error("Error in final confirmation prompt", "error", err)
			return
		}

		if finalConfirm {
			log.Info("User confirmed cluster provisioning")
			fmt.Println("Provisioning cluster...")
			// Add logic here to run 00-init.sh
		} else {
			log.Info("User cancelled cluster provisioning")
			fmt.Println("Cluster provisioning cancelled.")
		}
	} else {
		log.Info("User chose not to run 00-init.sh")
		fmt.Println("Cluster provisioning cancelled.")
	}
}

func setupKubefirstRepositories() {
	repos := []string{
		"github.com/kubefirst/kubefirst",
		"github.com/kubefirst/console",
		"github.com/kubefirst/kubefirst-api",
	}

	var branch string
	err := huh.NewInput().
		Title("Enter the branch name to checkout (default: main)").
		Value(&branch).
		Run()

	if err != nil {
		log.Error("Error getting branch name", "error", err)
		return
	}

	if branch == "" {
		branch = "main"
	}

	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")
	err = os.MkdirAll(repoDir, 0755)
	if err != nil {
		log.Error("Error creating repositories directory", "error", err)
		return
	}

	summary := make([][]string, 0, len(repos)+1)
	summary = append(summary, []string{"Repository", "Clone Path", "Symlink Path", "Branch", "Status"})

	for _, repo := range repos {
		repoName := filepath.Base(repo)
		repoPath := filepath.Join(repoDir, repoName)
		symlinkPath := filepath.Join(baseDir, repoName)

		if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
			// Repository already exists, sync instead
			fmt.Printf("Repository %s already exists. Syncing...\n", repo)
			status := syncRepository(repoPath, branch)
			summary = append(summary, []string{repo, repoPath, symlinkPath, branch, status})
			continue
		}

		fmt.Printf("Cloning %s...\n", repo)

		cmd := exec.Command("git", "clone", "-b", branch, "https://"+repo+".git", repoPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Error cloning repository", "repo", repo, "error", err, "output", string(output))
			summary = append(summary, []string{repo, repoPath, symlinkPath, branch, "Failed to clone"})
			continue
		}

		err = os.Symlink(repoPath, symlinkPath)
		if err != nil {
			if !os.IsExist(err) {
				log.Error("Error creating symlink", "repo", repo, "error", err)
				summary = append(summary, []string{repo, repoPath, symlinkPath, branch, "Cloned, failed to symlink"})
				continue
			}
			// Symlink already exists, which is fine
		}

		summary = append(summary, []string{repo, repoPath, symlinkPath, branch, "Success"})
		fmt.Printf("Repository %s setup complete\n", repo)
	}

	printSummaryTable(summary)
}

func syncKubefirstRepositories() {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")

	repos, err := os.ReadDir(repoDir)
	if err != nil {
		log.Error("Error reading repositories directory", "error", err)
		return
	}

	summary := make([][]string, 0, len(repos)+1)
	summary = append(summary, []string{"Repository", "Path", "Current Branch", "Status"})

	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}

		repoPath := filepath.Join(repoDir, repo.Name())
		fmt.Printf("Syncing %s...\n", repo.Name())

		// Get current branch
		branch, err := getCurrentBranch(repoPath)
		if err != nil {
			log.Error("Error getting current branch", "repo", repo.Name(), "error", err)
			summary = append(summary, []string{repo.Name(), repoPath, "Unknown", "Failed to get branch"})
			continue
		}

		status := syncRepository(repoPath, branch)
		summary = append(summary, []string{repo.Name(), repoPath, branch, status})
		fmt.Printf("Repository %s sync complete\n", repo.Name())
	}

	printSummaryTable(summary)
}

// Helper functions

func getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error getting current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func printSummaryTable(summary [][]string) {
	fmt.Println(style.Render("\nRepository Setup Summary:"))

	colWidths := make([]int, len(summary[0]))
	for _, row := range summary {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	for i, row := range summary {
		for j, cell := range row {
			fmt.Printf("%-*s", colWidths[j]+2, cell)
		}
		fmt.Println()
		if i == 0 {
			for _, width := range colWidths {
				fmt.Print(strings.Repeat("-", width+2))
			}
			fmt.Println()
		}
	}
}

func runKubefirst(repoDir, logsDir string) {
	kubefirstDir := filepath.Join(repoDir, "kubefirst")
	logFile := filepath.Join(logsDir, "kubefirst.log")

	buildCmd := exec.Command("go", "build", "-o", "kubefirst")
	buildCmd.Dir = kubefirstDir

	err := runAndLogCommand(buildCmd, logFile, color.FgYellow)
	if err != nil {
		log.Error("Error building kubefirst", "error", err)
		return
	}

	log.Info("Kubefirst binary built successfully", "path", filepath.Join(kubefirstDir, "kubefirst"))
}

func runKubefirstAPI(repoDir, logsDir string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Error("Failed to get user home directory", "error", err)
		return
	}

	apiDir := filepath.Join(homeDir, ".ssot", "k1space", ".repositories", "kubefirst-api")
	logFile := filepath.Join(logsDir, "kubefirst-api.log")
	scriptFile := filepath.Join(apiDir, "setup_and_run.sh")

	log.Info("Preparing kubefirst-api setup",
		"apiDir", apiDir,
		"logFile", logFile,
		"scriptFile", scriptFile)

	// Check if apiDir exists
	if _, err := os.Stat(apiDir); os.IsNotExist(err) {
		log.Error("API directory does not exist", "path", apiDir)
		return
	}

	setupScript := `#!/bin/bash
set -e

exec > >(tee -a "` + logFile + `") 2>&1

echo "Starting setup script"
echo "Current working directory: $(pwd)"
echo "Log file: ` + logFile + `"
echo "API directory: ` + apiDir + `"


function log_error {
    echo "ERROR: $1" >&2
    echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: $1" >> "$LOG_FILE"
}

function log_info {
    echo "INFO: $1"
    echo "$(date '+%Y-%m-%d %H:%M:%S') - INFO: $1" >> "$LOG_FILE"
}

LOG_FILE="` + logFile + `"
API_DIR="` + apiDir + `"
cd "$API_DIR"

log_info "Current working directory: $(pwd)"
log_info "Log file: $LOG_FILE"
log_info "API directory: $API_DIR"

# Check for required tools
for cmd in go k3d kubectl make air swag; do
    if ! command -v $cmd &> /dev/null; then
        log_error "$cmd could not be found. Please install it and try again."
        exit 1
    fi
done

log_info "Installing required tools..."
go install github.com/air-verse/air@latest
go install github.com/swaggo/swag/cmd/swag@latest

# Check k3d cluster
if ! k3d cluster list | grep -q "dev"; then
    log_info "Creating k3d cluster 'dev'..."
    k3d cluster create dev
else
    log_info "k3d cluster 'dev' already exists."
fi

log_info "Ensuring kubeconfig is accessible..."
KUBECONFIG_PATH=$(k3d kubeconfig write dev)
export KUBECONFIG="$KUBECONFIG_PATH"
log_info "Kubeconfig path: $KUBECONFIG_PATH"

# Wait for the cluster to be ready
log_info "Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=300s

# Set environment variables
export K1_LOCAL_DEBUG=true
export K1_LOCAL_KUBECONFIG_PATH="$KUBECONFIG_PATH"
export CLUSTER_ID="local-dev"
export CLUSTER_TYPE="k3d"
export INSTALL_METHOD="local"
export K1_ACCESS_TOKEN="local-dev-token"
export IS_CLUSTER_ZERO=true

# Create and source .env file
ENV_FILE="$API_DIR/.env"
log_info "Checking for .env file at: $ENV_FILE"
if [ ! -f "$ENV_FILE" ]; then
    log_info "Creating .env file from .env.example"
    cp "$API_DIR/.env.example" "$ENV_FILE"
    log_info "Created .env file from .env.example"
    log_info "Please edit $ENV_FILE with your specific values, then press Enter to continue."
    read
else
    log_info ".env file already exists"
fi
set -a
source "$ENV_FILE"
set +a

log_info "Environment variables set:"
env | grep -E 'K1_|CLUSTER_|KUBECONFIG' | sed 's/^/  /'

log_info "Creating necessary Kubernetes resources..."
kubectl create namespace kubefirst --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic kubefirst-clusters --from-literal=clusters='{}' -n kubefirst --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic kubefirst-catalog --from-literal=catalog='{}' -n kubefirst --dry-run=client -o yaml | kubectl apply -f -

log_info "Updating Swagger documentation..."
make updateswagger

log_info "Building the kubefirst-api binary..."
make build

log_info "Starting kubefirst-api with air for live reloading..."
air
`

	// Create the script file
	err = os.WriteFile(scriptFile, []byte(setupScript), 0755)
	if err != nil {
		log.Error("Failed to create setup script", "error", err, "path", scriptFile)
		return
	}
	log.Info("Created setup script", "path", scriptFile)

	// Check if the script file was actually created
	if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
		log.Error("Failed to create setup script", "error", err, "path", scriptFile)
		return
	}
	log.Info("Verified setup script creation", "path", scriptFile)

	// Execute the script
	log.Info("Running kubefirst-api setup script")
	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = apiDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Error("Error running kubefirst-api setup script", "error", err)
		// Try to read and log the script content
		scriptContent, readErr := os.ReadFile(scriptFile)
		if readErr != nil {
			log.Error("Failed to read script file", "error", readErr)
		} else {
			log.Info("Script content", "content", string(scriptContent))
		}
	} else {
		log.Info("Successfully ran kubefirst-api setup script")
	}

	// Check if the script file exists after execution
	if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
		log.Error("Script file does not exist after execution", "path", scriptFile)
	} else {
		log.Info("Script file exists after execution", "path", scriptFile)
	}

	// Print the contents of the log file
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		log.Error("Failed to read log file", "error", err)
	} else {
		log.Info("Log file contents", "content", string(logContent))
	}
}

func runConsole(repoDir, logsDir string) {
	consoleDir := filepath.Join(repoDir, "console")
	logFile := filepath.Join(logsDir, "console.log")

	log.Info("Starting console setup", "directory", consoleDir)

	// Check if yarn is installed
	if _, err := exec.LookPath("yarn"); err != nil {
		log.Error("yarn is not installed or not in PATH. Please install yarn and try again.")
		return
	}

	// Install dependencies
	log.Info("Installing dependencies...")
	err := runCommandWithLiveOutput("yarn install", consoleDir, logFile)
	if err != nil {
		log.Error("Failed to install dependencies", "error", err)
		return
	}
	log.Info("Dependencies installed successfully")

	// Check if next is installed
	nextPath := filepath.Join(consoleDir, "node_modules", ".bin", "next")
	if _, err := os.Stat(nextPath); os.IsNotExist(err) {
		log.Error("next command not found. Make sure it's listed in package.json dependencies.")
		return
	}
	log.Info("Next.js installation verified")

	// Run the dev command
	log.Info("Starting Next.js development server...")
	cmd := exec.Command("yarn", "dev")
	cmd.Dir = consoleDir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(consoleDir, "node_modules", ".bin")+":"+os.Getenv("PATH"))

	err = runCommandWithLiveOutput("yarn dev", consoleDir, logFile)
	if err != nil {
		log.Error("Error running console", "error", err)
	}
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

func runKubefirstSetup() error {
	// Prompt for branch name
	var branch string
	err := huh.NewInput().
		Title("Enter the branch name to checkout (default: main)").
		Value(&branch).
		Run()

	if err != nil {
		return fmt.Errorf("error getting branch name: %w", err)
	}

	if branch == "" {
		branch = "main"
	}

	// Setup Console Environment
	err = setupConsoleEnvironment()
	if err != nil {
		log.Error("Error setting up Console environment", "error", err)
		return err
	}

	// Setup Kubefirst API
	err = setupKubefirstAPI(branch)
	if err != nil {
		log.Error("Error setting up Kubefirst API", "error", err)
		return err
	}

	// Setup Kubefirst
	err = setupKubefirst(branch)
	if err != nil {
		log.Error("Error setting up Kubefirst", "error", err)
		return err
	}

	return nil
}

func setupConsoleEnvironment() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	consoleDir := filepath.Join(baseDir, "console")
	envExamplePath := filepath.Join(consoleDir, ".env.example")
	envPath := filepath.Join(consoleDir, ".env")

	// Read .env.example
	content, err := os.ReadFile(envExamplePath)
	if err != nil {
		return fmt.Errorf("error reading .env.example: %w", err)
	}

	// Check if .env already exists
	if _, err := os.Stat(envPath); err == nil {
		var overwrite bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("The .env file in `Console` already exists. Do you want to overwrite it?").
					Value(&overwrite),
			),
		)

		err = form.Run()
		if err != nil {
			return fmt.Errorf("error in user prompt: %w", err)
		}

		if !overwrite {
			fmt.Println("Skipping .env creation.")
			return nil
		}
	}

	// Create .env file
	err = os.WriteFile(envPath, content, 0644)
	if err != nil {
		return fmt.Errorf("error creating .env file: %w", err)
	}

	fmt.Println("Created .env file for Console.")

	// Set K1_LOCAL_DEBUG environment variable
	err = os.Setenv("K1_LOCAL_DEBUG", "true")
	if err != nil {
		return fmt.Errorf("error setting K1_LOCAL_DEBUG: %w", err)
	}

	fmt.Println("Set K1_LOCAL_DEBUG=true")

	return nil
}

func setupKubefirstAPI(branch string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting user home directory: %w", err)
	}

	apiDir := filepath.Join(homeDir, ".ssot", "k1space", ".repositories", "kubefirst-api")
	scriptFile := filepath.Join(apiDir, "setup_and_run.sh")

	// Create the script file
	err = os.WriteFile(scriptFile, []byte(kubefirstAPISetupScript), 0755)
	if err != nil {
		return fmt.Errorf("failed to create setup script: %w", err)
	}
	log.Info("Created setup script", "path", scriptFile)

	// Spawn a background task to create the k3d cluster
	go func() {
		cmd := exec.Command("k3d", "cluster", "create", "dev")
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Failed to create k3d cluster", "error", err, "output", string(output))
		} else {
			log.Info("Successfully created k3d cluster")
		}
	}()

	// Checkout specified branch
	cmd := exec.Command("git", "-C", apiDir, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out %s: %w\nOutput: %s", branch, err, output)
	}

	fmt.Printf("Checked out %s branch for Kubefirst API\n", branch)
	fmt.Println("Setup script created and k3d cluster creation started in the background.")
	fmt.Println("You can now use the 'Run Kubefirst Repositories' command to start the API.")

	return nil
}

func setupKubefirst(branch string) error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	kubefirstDir := filepath.Join(baseDir, ".repositories", "kubefirst")

	// Set K1_LOCAL_DEBUG environment variable
	err := os.Setenv("K1_LOCAL_DEBUG", "true")
	if err != nil {
		return fmt.Errorf("error setting K1_LOCAL_DEBUG: %w", err)
	}

	fmt.Println("Set K1_LOCAL_DEBUG=true for Kubefirst")

	// Checkout specified branch
	cmd := exec.Command("git", "-C", kubefirstDir, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out %s: %w\nOutput: %s", branch, err, output)
	}

	fmt.Printf("Checked out %s branch for Kubefirst\n", branch)

	// Update go.mod
	apiDir := filepath.Join(baseDir, ".repositories", "kubefirst-api")
	goModPath := filepath.Join(kubefirstDir, "go.mod")

	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}

	// Find the line with kubefirst-api and replace it
	lines := strings.Split(string(goModContent), "\n")
	for i, line := range lines {
		if strings.Contains(line, "github.com/kubefirst/kubefirst-api") {
			lines[i] = fmt.Sprintf("github.com/kubefirst/kubefirst-api v0.0.0")
			break
		}
	}

	// Add the replace directive
	lines = append(lines, fmt.Sprintf("replace github.com/kubefirst/kubefirst-api => %s", apiDir))

	newContent := strings.Join(lines, "\n")

	err = os.WriteFile(goModPath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating go.mod: %w", err)
	}

	fmt.Println("Updated go.mod to point to local Kubefirst API repository")

	// Build the kubefirst binary
	buildCmd := exec.Command("go", "build", "-o", "kubefirst")
	buildCmd.Dir = kubefirstDir
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error building kubefirst: %w\nOutput: %s", err, buildOutput)
	}

	fmt.Println("Built Kubefirst binary successfully")

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

func syncRepository(repoPath, branch string) string {
	// Fetch the latest changes
	cmd := exec.Command("git", "-C", repoPath, "fetch", "origin")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("Error fetching repository", "repo", repoPath, "error", err, "output", string(output))
		return "Failed to fetch"
	}

	// Pull the latest changes for the current branch
	cmd = exec.Command("git", "-C", repoPath, "pull", "origin", branch)
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Error("Error pulling latest changes", "repo", repoPath, "branch", branch, "error", err, "output", string(output))
		return "Failed to pull latest changes"
	}

	if strings.Contains(string(output), "Already up to date.") {
		return "Up to date"
	}
	return "Updated"
}

func revertKubefirstToMain() {
	log.Info("Starting revert Kubefirst to main process")

	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repos := []string{"kubefirst", "console", "kubefirst-api"}
	summary := make(map[string]string)

	var stashChanges bool
	err := huh.NewConfirm().
		Title("Local changes detected. Do you want to stash changes in the repositories?").
		Value(&stashChanges).
		Run()

	if err != nil {
		log.Error("Error in user prompt", "error", err)
		return
	}

	if !stashChanges {
		fmt.Println("Operation cancelled. No changes were made.")
		return
	}

	for _, repo := range repos {
		repoPath := filepath.Join(baseDir, ".repositories", repo)

		// Check for local changes
		cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
		output, err := cmd.Output()
		if err != nil {
			log.Error("Error checking git status", "repo", repo, "error", err)
			summary[repo] = "Failed to check status"
			continue
		}

		if len(output) > 0 {
			// Stash changes
			cmd = exec.Command("git", "-C", repoPath, "stash")
			output, err = cmd.CombinedOutput()
			if err != nil {
				log.Error("Error stashing changes", "repo", repo, "error", err, "output", string(output))
				summary[repo] = "Failed to stash changes"
				continue
			}
			summary[repo] = "Changes stashed"
		} else {
			summary[repo] = "No local changes"
		}

		// Checkout main branch
		cmd = exec.Command("git", "-C", repoPath, "checkout", "main")
		output, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("Error checking out main branch", "repo", repo, "error", err, "output", string(output))
			summary[repo] += ", Failed to checkout main"
			continue
		}

		// Pull latest changes
		cmd = exec.Command("git", "-C", repoPath, "pull", "origin", "main")
		output, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("Error pulling latest changes", "repo", repo, "error", err, "output", string(output))
			summary[repo] += ", Failed to pull latest"
			continue
		}

		if !strings.Contains(summary[repo], "Failed") {
			summary[repo] += ", Reverted to main"
		}
	}

	// Revert Console environment
	consoleEnvPath := filepath.Join(baseDir, "console", ".env")
	if err := os.Remove(consoleEnvPath); err != nil && !os.IsNotExist(err) {
		log.Error("Error removing Console .env file", "error", err)
		summary["Console .env"] = "Failed to remove"
	} else {
		summary["Console .env"] = "Removed"
	}

	// Unset K1_LOCAL_DEBUG environment variable
	os.Unsetenv("K1_LOCAL_DEBUG")
	summary["K1_LOCAL_DEBUG"] = "Unset"

	// Print summary
	fmt.Println("\nRevert to Main Summary:")
	fmt.Println("------------------------")
	for item, status := range summary {
		fmt.Printf("%-20s: %s\n", item, status)
	}

	fmt.Println("\nRevert to main process completed")
	fmt.Println("Note: If changes were stashed, use 'git stash pop' in the respective repositories to recover them.")
}

func runKubefirstRepositories() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Error("Failed to get user home directory", "error", err)
		return
	}

	baseDir := filepath.Join(homeDir, ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")
	logsDir := filepath.Join(baseDir, ".logs")
	scriptFile := filepath.Join(repoDir, "kubefirst-api", "setup_and_run.sh")

	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		log.Error("Error creating logs directory", "error", err)
		return
	}

	// Check if the script file exists
	if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
		log.Error("Setup script does not exist. Please run 'Setup Kubefirst' first.", "path", scriptFile)
		return
	}

	timestamp := time.Now().Format("2006-01-02-150405")

	kubefirstAPILogs := &scrollingLog{}
	consoleLogs := &scrollingLog{}
	kubefirstLogs := &scrollingLog{}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		runServiceWithColoredLogs("kubefirst-api", filepath.Join(repoDir, "kubefirst-api"), logsDir, timestamp, color.New(color.FgMagenta), func(dir string) *exec.Cmd {
			return exec.Command("bash", scriptFile)
		}, kubefirstAPILogs)
	}()

	go func() {
		defer wg.Done()
		runServiceWithColoredLogs("console", filepath.Join(repoDir, "console"), logsDir, timestamp, color.New(color.FgCyan), func(dir string) *exec.Cmd {
			return exec.Command("yarn", "dev")
		}, consoleLogs)
	}()

	go func() {
		defer wg.Done()
		runServiceWithColoredLogs("kubefirst", filepath.Join(repoDir, "kubefirst"), logsDir, timestamp, color.New(color.FgYellow), func(dir string) *exec.Cmd {
			return exec.Command("go", "run", "main.go")
		}, kubefirstLogs)
	}()

	go updateDisplayWithLogs(kubefirstAPILogs, consoleLogs, kubefirstLogs)

	fmt.Println("Press 'q' to quit and return to the main menu.")
	waitForQuit()
}

func updateDisplayWithLogs(kubefirstAPILogs, consoleLogs, kubefirstLogs *scrollingLog) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			display := renderDashboard(kubefirstAPILogs, consoleLogs, kubefirstLogs)
			fmt.Print("\033[2J") // Clear the screen
			fmt.Print("\033[H")  // Move cursor to top-left corner
			fmt.Print(display)
		}
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

func appendLog(logs []string, newLog string, maxLogs int) []string {
	logs = append(logs, newLog)
	if len(logs) > maxLogs {
		logs = logs[len(logs)-maxLogs:]
	}
	return logs
}

func updateDisplay(consoleChan, kubefirstAPIChan, kubefirstChan <-chan string) {
	for {
		consoleOutput := <-consoleChan
		kubefirstAPIOutput := <-kubefirstAPIChan
		kubefirstOutput := <-kubefirstChan

		display := lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Kubefirst Repositories"),
			lipgloss.JoinHorizontal(lipgloss.Top,
				consoleStyle.Render(consoleOutput),
				kubefirstAPIStyle.Render(kubefirstAPIOutput),
				kubefirstStyle.Render(kubefirstOutput),
			),
		)

		fmt.Print("\033[2J") // Clear the screen
		fmt.Print("\033[H")  // Move cursor to top-left corner
		fmt.Println(display)
	}
}

func runServiceWithColoredLogs(serviceName, serviceDir, logsDir, timestamp string, printer *color.Color, cmdCreator func(string) *exec.Cmd, logs *scrollingLog) {
	logFileName := fmt.Sprintf("%s-%s.log", serviceName, timestamp)
	logFile := filepath.Join(logsDir, logFileName)
	f, err := os.Create(logFile)
	if err != nil {
		log.Error("Error creating log file", "service", serviceName, "error", err)
		return
	}
	defer f.Close()

	cmd := cmdCreator(serviceDir)
	cmd.Dir = serviceDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error("Error creating stdout pipe", "service", serviceName, "error", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error("Error creating stderr pipe", "service", serviceName, "error", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		log.Error("Error starting service", "service", serviceName, "error", err)
		return
	}

	go logOutput(serviceName, stdout, f, printer, logs)
	go logOutput(serviceName, stderr, f, printer, logs)

	err = cmd.Wait()
	if err != nil {
		log.Error("Service exited with error", "service", serviceName, "error", err)
	}
}

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

func startSpinner(message string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Suffix = " " + message
	s.Start()
	return s
}

func stopSpinner(s *spinner.Spinner, success bool) {
	s.Stop()
	if success {
		color.Green("✓ " + s.Suffix)
	} else {
		color.Red("✗ " + s.Suffix)
	}
}
