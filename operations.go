package main

import (
	"bufio"
	"bytes"
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
	"github.com/charmbracelet/log"
	"github.com/fatih/color"
)

var (
	consolePrinter      = color.New(color.FgCyan)
	kubefirstAPIPrinter = color.New(color.FgMagenta)
	kubefirstPrinter    = color.New(color.FgYellow)
)

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
	log.Info("Starting runKubefirstAPI function", "repoDir", repoDir, "logsDir", logsDir)
	currentDir, _ := os.Getwd()
	log.Info("Current working directory", "dir", currentDir)

	kubeconfigPath, err := setupK3dCluster()
	if err != nil {
		log.Error("Failed to set up k3d cluster", "error", err)
		return
	}

	// Set environment variables
	os.Setenv("KUBECONFIG", kubeconfigPath)
	os.Setenv("K1_LOCAL_KUBECONFIG_PATH", kubeconfigPath)
	log.Info("Environment variables set",
		"KUBECONFIG", os.Getenv("KUBECONFIG"),
		"K1_LOCAL_KUBECONFIG_PATH", os.Getenv("K1_LOCAL_KUBECONFIG_PATH"))

	// Run kubefirst-api directly instead of using air
	apiCmd := exec.Command("go", "run", ".")
	apiCmd.Dir = filepath.Join(repoDir, "kubefirst-api")
	apiCmd.Env = append(os.Environ(),
		"KUBECONFIG="+kubeconfigPath,
		"K1_LOCAL_KUBECONFIG_PATH="+kubeconfigPath,
		"K1_LOCAL_DEBUG=true",
		"CLUSTER_ID=local-dev",
		"CLUSTER_TYPE=k3d",
		"INSTALL_METHOD=local",
		"K1_ACCESS_TOKEN=local-dev-token",
		"IS_CLUSTER_ZERO=true")

	// Capture and log the output
	var stdout, stderr bytes.Buffer
	apiCmd.Stdout = &stdout
	apiCmd.Stderr = &stderr

	log.Info("Starting kubefirst-api...")
	err = apiCmd.Start()
	if err != nil {
		log.Error("Failed to start kubefirst-api", "error", err)
		return
	}

	// Log the process ID
	log.Info("Started kubefirst-api", "pid", apiCmd.Process.Pid)

	// Use a goroutine to continuously log the output
	go func() {
		for {
			log.Info("kubefirst-api stdout", "output", stdout.String())
			log.Info("kubefirst-api stderr", "output", stderr.String())
			stdout.Reset()
			stderr.Reset()
			time.Sleep(5 * time.Second)
		}
	}()

	// Wait for the command to finish
	err = apiCmd.Wait()
	if err != nil {
		log.Error("kubefirst-api exited with error", "error", err)
	}
}

func createKubernetesResources() error {
	log.Info("Creating Kubernetes resources...")
	resources := []struct {
		args []string
		name string
	}{
		{[]string{"create", "namespace", "kubefirst", "--dry-run=client", "-o", "yaml"}, "namespace"},
		{[]string{"create", "secret", "generic", "kubefirst-clusters", "--from-literal=clusters={}", "-n", "kubefirst", "--dry-run=client", "-o", "yaml"}, "clusters secret"},
		{[]string{"create", "secret", "generic", "kubefirst-catalog", "--from-literal=catalog={}", "-n", "kubefirst", "--dry-run=client", "-o", "yaml"}, "catalog secret"},
	}

	for _, resource := range resources {
		log.Info("Creating resource", "resource", resource.name)
		cmd := exec.Command("kubectl", resource.args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Failed to generate resource YAML", "resource", resource.name, "error", err, "output", string(output))
			return fmt.Errorf("failed to generate %s YAML: %w", resource.name, err)
		}

		applyCmd := exec.Command("kubectl", "apply", "-f", "-")
		applyCmd.Stdin = bytes.NewReader(output)
		applyOutput, err := applyCmd.CombinedOutput()
		if err != nil {
			log.Error("Failed to apply resource", "resource", resource.name, "error", err, "output", string(applyOutput))
			return fmt.Errorf("failed to apply %s: %w", resource.name, err)
		}
		log.Info("Successfully created resource", "resource", resource.name)
	}

	log.Info("All Kubernetes resources created successfully")
	return nil
}

func installGoPackages() error {
	cmds := []string{
		"go install github.com/air-verse/air@latest",
		"go install github.com/swaggo/swag/cmd/swag@latest",
	}

	for _, cmd := range cmds {
		if err := exec.Command("bash", "-c", cmd).Run(); err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

func createK3dCluster() (string, error) {
	log.Info("Creating k3d cluster...")
	cmd := exec.Command("k3d", "cluster", "create", "dev", "--wait")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create k3d cluster: %w\nOutput: %s", err, output)
	}
	log.Info("k3d cluster created successfully")

	// Get kubeconfig
	kubeconfigCmd := exec.Command("k3d", "kubeconfig", "get", "dev")
	kubeconfigContent, err := kubeconfigCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get k3d kubeconfig: %w", err)
	}

	// Write kubeconfig to file
	homeDir, _ := os.UserHomeDir()
	kubeconfigPath := filepath.Join(homeDir, ".k3d", "kubeconfig-dev.yaml")
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for kubeconfig: %w", err)
	}
	if err := os.WriteFile(kubeconfigPath, kubeconfigContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig file: %w", err)
	}
	log.Info("Kubeconfig written successfully", "path", kubeconfigPath)

	return kubeconfigPath, nil
}

func waitForK3dCluster() error {
	for i := 0; i < 30; i++ {
		cmd := exec.Command("k3d", "cluster", "list", "--no-headers")
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Failed to list k3d clusters", "error", err)
		} else {
			if strings.Contains(string(output), "dev") {
				log.Info("k3d cluster is ready")
				return nil
			}
		}
		log.Info("Waiting for k3d cluster to be ready...", "attempt", i+1)
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("timeout waiting for k3d cluster to be ready")
}

func setEnvironmentVariables(apiDir string) error {
	vars := map[string]string{
		"K1_LOCAL_DEBUG":           "true",
		"K1_LOCAL_KUBECONFIG_PATH": "", // We'll set this dynamically
		"CLUSTER_ID":               "local-dev",
		"CLUSTER_TYPE":             "k3d",
		"INSTALL_METHOD":           "local",
		"K1_ACCESS_TOKEN":          "local-dev-token",
		"IS_CLUSTER_ZERO":          "true",
	}

	// Get the kubeconfig path
	out, err := exec.Command("k3d", "kubeconfig", "get", "dev").Output()
	if err != nil {
		return fmt.Errorf("failed to get k3d kubeconfig: %w", err)
	}
	vars["K1_LOCAL_KUBECONFIG_PATH"] = strings.TrimSpace(string(out))

	// Set environment variables
	for k, v := range vars {
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", k, err)
		}
	}

	// Create or update .env file
	envFile := filepath.Join(apiDir, ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		exampleEnv := filepath.Join(apiDir, ".env.example")
		if err := exec.Command("cp", exampleEnv, envFile).Run(); err != nil {
			return fmt.Errorf("failed to copy .env.example to .env: %w", err)
		}
	}

	return nil
}

func updateSwaggerAndBuild(apiDir string) error {
	cmds := []string{
		"make updateswagger",
		"make build",
	}

	for _, cmd := range cmds {
		command := exec.Command("bash", "-c", cmd)
		command.Dir = apiDir
		if err := command.Run(); err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

func readEnvFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	envMap := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if equal := strings.Index(line, "="); equal >= 0 {
			if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
				value := ""
				if len(line) > equal {
					value = strings.TrimSpace(line[equal+1:])
				}
				envMap[key] = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return envMap, nil
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
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	apiDir := filepath.Join(baseDir, "kubefirst-api")

	// Checkout specified branch
	cmd := exec.Command("git", "-C", apiDir, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out %s: %w\nOutput: %s", branch, err, output)
	}

	fmt.Printf("Checked out %s branch for Kubefirst API\n", branch)

	// TODO: Add instructions to run the API locally
	fmt.Println("Please follow the instructions in the Kubefirst API README to set up and run the API locally.")

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
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")
	logsDir := filepath.Join(baseDir, ".logs")

	err := os.MkdirAll(logsDir, 0755)
	if err != nil {
		log.Error("Error creating logs directory", "error", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		runConsole(repoDir, logsDir)
	}()

	go func() {
		defer wg.Done()
		runKubefirstAPI(repoDir, logsDir)
	}()

	go func() {
		defer wg.Done()
		runKubefirst(repoDir, logsDir)
	}()

	wg.Wait()
}

func runServiceWithColoredLogs(serviceName, serviceDir, logsDir, timestamp string, printer *color.Color, cmdCreator func(string) *exec.Cmd) {
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

	go logOutput(serviceName, stdout, f, printer)
	go logOutput(serviceName, stderr, f, printer)

	err = cmd.Wait()
	if err != nil {
		log.Error("Service exited with error", "service", serviceName, "error", err)
	}
}

func logOutput(serviceName string, reader io.Reader, logFile *os.File, printer *color.Color) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		formattedLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, printer.Sprint(serviceName), line)
		fmt.Print(formattedLine)
		logFile.WriteString(formattedLine)
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
func setupK3dCluster() (string, error) {
	log.Info("Setting up k3d cluster...")

	cmd := exec.Command("k3d", "cluster", "create", "dev", "--wait", "--timeout", "5m")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Error("Failed to create k3d cluster", "stdout", stdout.String(), "stderr", stderr.String())
		return "", fmt.Errorf("failed to create k3d cluster: %w", err)
	}

	log.Info("k3d cluster created, waiting for it to be ready...")

	// Wait for the cluster to be ready
	if err := waitForK3dCluster(); err != nil {
		return "", fmt.Errorf("k3d cluster failed to become ready: %w", err)
	}

	// Get kubeconfig
	kubeconfigCmd := exec.Command("k3d", "kubeconfig", "get", "dev")
	kubeconfigContent, err := kubeconfigCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get k3d kubeconfig: %w", err)
	}

	// Write kubeconfig to file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(homeDir, ".k3d", "kubeconfig-dev.yaml")
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for kubeconfig: %w", err)
	}
	if err := os.WriteFile(kubeconfigPath, kubeconfigContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig file: %w", err)
	}

	log.Info("Kubeconfig written", "path", kubeconfigPath)

	return kubeconfigPath, nil
}

func printRepositorySummary(repoDir string) {
	repos := []string{"kubefirst", "console", "kubefirst-api"}

	log.Info("Repository Summary:")
	for _, repo := range repos {
		repoPath := filepath.Join(repoDir, repo)
		cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
		branch, err := cmd.Output()
		if err != nil {
			log.Error("Failed to get branch for repo", "repo", repo, "error", err)
			continue
		}

		log.Info("Repository info",
			"name", repo,
			"path", repoPath,
			"branch", strings.TrimSpace(string(branch)))
	}
}
