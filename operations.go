package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
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
	kubefirstDir := filepath.Join(baseDir, "kubefirst")

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
	apiDir := filepath.Join(baseDir, "kubefirst-api")
	goModPath := filepath.Join(kubefirstDir, "go.mod")

	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}

	newContent := strings.Replace(string(goModContent),
		"github.com/kubefirst/kubefirst-api v0.1.26",
		fmt.Sprintf("github.com/kubefirst/kubefirst-api v0.1.26 => %s", apiDir),
		1)

	err = os.WriteFile(goModPath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating go.mod: %w", err)
	}

	fmt.Println("Updated go.mod to point to local Kubefirst API repository")

	// TODO: Add instructions to build and run Kubefirst
	fmt.Println("Please build the Kubefirst binary and execute the create command with the necessary parameters.")

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
