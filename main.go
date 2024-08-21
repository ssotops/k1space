package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/civo/civogo"
	"github.com/digitalocean/godo"
)

var (
	style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		PaddingTop(1).
		PaddingBottom(1).
		PaddingLeft(4).
		PaddingRight(4)
)

type CloudConfig struct {
	StaticPrefix string
	CloudPrefix  string
	Region       string
	Flags        map[string]string
}

type InstanceSizeInfo struct {
	Name          string
	CPUCores      int
	RAMMegabytes  int
	DiskGigabytes int
}

type Lockfile struct {
	Version        int                           `json:"version"`
	Configs        map[string][]string           `json:"configs"`
	DefaultValues  map[string]string             `json:"defaultValues"`
	CloudRegions   map[string][]string           `json:"cloudRegions"`
	CloudNodeTypes map[string][]InstanceSizeInfo `json:"cloudNodeTypes"`
}

var cloudProviders = []string{
	// "Akamai",
	// "AWS",
	"Civo",
	"DigitalOcean",
	// "Google Cloud",
	// "Vultr",
	// "K3s",
	"K3d",
}

var cloudFlags = map[string][]string{
	"Akamai":       {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"AWS":          {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Civo":         {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"DigitalOcean": {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Google Cloud": {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Vultr":        {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"K3s":          {"alerts-email", "cluster-name", "domain-name", "github-org"},
	"K3d":          {"cluster-name", "github-org"},
}

func main() {
	log.SetOutput(os.Stderr)

	for {
		action := runMainMenu()
		switch action {
		case "Config":
			runConfigMenu()
		case "Kubefirst":
			runKubefirstMenu()
		case "Exit":
			fmt.Println("Exiting k1space. Goodbye!")
			return
		}
	}
}

func runMainMenu() string {
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("K1Space Main Menu").
				Options(
					huh.NewOption("Config", "Config"),
					huh.NewOption("Kubefirst", "Kubefirst"),
					huh.NewOption("Exit", "Exit"),
				).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		log.Error("Error running main menu", "error", err)
		os.Exit(1)
	}

	return selected
}

func runKubefirstMenu() {
	for {
		var selected string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Kubefirst Menu").
					Options(
						huh.NewOption("Setup Repositories", "Setup Repositories"),
						huh.NewOption("Sync Repositories", "Sync Repositories"),
						huh.NewOption("Setup Kubefirst", "Setup Kubefirst"),
						huh.NewOption("Back", "Back"),
					).
					Value(&selected),
			),
		)

		err := form.Run()
		if err != nil {
			log.Error("Error running Kubefirst menu", "error", err)
			return
		}

		switch selected {
		case "Setup Repositories":
			setupKubefirstRepositories()
		case "Sync Repositories":
			syncKubefirstRepositories()
		case "Setup Kubefirst":
			handleKubefirstSetup()
		case "Back":
			return
		}

		// Prompt user to continue or return to main menu
		var continueAction bool
		continueForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to perform another Kubefirst action?").
					Value(&continueAction),
			),
		)

		err = continueForm.Run()
		if err != nil {
			log.Error("Error in continue prompt", "error", err)
			return
		}

		if !continueAction {
			return
		}
	}
}

func setupKubefirstRepositories() {
	repos := []string{
		"github.com/kubefirst/kubefirst",
		"github.com/kubefirst/console",
		"github.com/kubefirst/kubefirst-api",
	}

	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")
	err := os.MkdirAll(repoDir, 0755)
	if err != nil {
		log.Error("Error creating repositories directory", "error", err)
		return
	}

	summary := make([][]string, 0, len(repos)+1)
	summary = append(summary, []string{"Repository", "Clone Path", "Symlink Path", "Status"})

	for _, repo := range repos {
		repoName := filepath.Base(repo)
		repoPath := filepath.Join(repoDir, repoName)
		symlinkPath := filepath.Join(baseDir, repoName)

		if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
			// Repository already exists, sync instead
			fmt.Printf("Repository %s already exists. Syncing...\n", repo)
			status := syncRepository(repoPath)
			summary = append(summary, []string{repo, repoPath, symlinkPath, status})
			continue
		}

		fmt.Printf("Cloning %s...\n", repo)

		cmd := exec.Command("git", "clone", "https://"+repo+".git", repoPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Error cloning repository", "repo", repo, "error", err, "output", string(output))
			summary = append(summary, []string{repo, repoPath, symlinkPath, "Failed to clone"})
			continue
		}

		err = os.Symlink(repoPath, symlinkPath)
		if err != nil {
			if !os.IsExist(err) {
				log.Error("Error creating symlink", "repo", repo, "error", err)
				summary = append(summary, []string{repo, repoPath, symlinkPath, "Cloned, failed to symlink"})
				continue
			}
			// Symlink already exists, which is fine
		}

		summary = append(summary, []string{repo, repoPath, symlinkPath, "Success"})
		fmt.Printf("Repository %s setup complete\n", repo)
	}

	printSummaryTable(summary)
}

func syncRepository(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("Error syncing repository", "repo", repoPath, "error", err, "output", string(output))
		return "Failed to sync"
	}

	if strings.Contains(string(output), "Already up to date.") {
		return "Up to date"
	}
	return "Updated"
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
	summary = append(summary, []string{"Repository", "Path", "Status"})

	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}

		repoPath := filepath.Join(repoDir, repo.Name())
		fmt.Printf("Syncing %s...\n", repo.Name())

		status := syncRepository(repoPath)
		summary = append(summary, []string{repo.Name(), repoPath, status})
		fmt.Printf("Repository %s sync complete\n", repo.Name())
	}

	printSummaryTable(summary)
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

func runConfigMenu() {
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Config Menu").
				Options(
					huh.NewOption("Create Config", "Create Config"),
					huh.NewOption("Back", "Back"),
				).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		log.Error("Error running config menu", "error", err)
		return
	}

	switch selected {
	case "Create Config":
		createConfig()
	case "Back":
		return
	}
}

func createConfig() {
	config := CloudConfig{
		Flags: make(map[string]string),
	}

	lockfile, err := loadLockfile()
	if err != nil {
		log.Error("Error loading lockfile", "error", err)
		return
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter static prefix").
				Description("Default is 'CS'").
				Placeholder("CS").
				Value(&config.StaticPrefix),

			huh.NewSelect[string]().
				Title("Select cloud provider").
				Options(getCloudProviderOptions()...).
				Value(&config.CloudPrefix),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in initial config form", "error", err)
		return
	}

	if config.CloudPrefix == "DigitalOcean" {
		err = updateDigitalOceanRegions(&lockfile)
		if err != nil {
			log.Error("Error updating DigitalOcean regions", "error", err)
			return
		}
		err = updateDigitalOceanNodeTypes(&lockfile)
		if err != nil {
			log.Error("Error updating DigitalOcean node types", "error", err)
			return
		}
	} else if config.CloudPrefix == "Civo" {
		err = updateCivoRegions(&lockfile)
		if err != nil {
			log.Error("Error updating Civo regions", "error", err)
			return
		}
		err = updateCivoNodeTypes(&lockfile)
		if err != nil {
			log.Error("Error updating Civo node types", "error", err)
			return
		}
	}

	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Options(getRegionOptions(config.CloudPrefix, lockfile)...).
				Value(&config.Region),
		),
	)

	err = regionForm.Run()
	if err != nil {
		log.Error("Error in region selection", "error", err)
		return
	}

	flags := cloudFlags[config.CloudPrefix]
	if len(flags) == 0 {
		log.Error("No flags found for the selected cloud provider")
		return
	}

	flagInputs := make([]struct{ Name, Value string }, 0, len(flags))
	flagGroups := make([]huh.Field, 0, len(flags))
	for _, flag := range flags {
		if flag == "cloud-region" || flag == "node-type" {
			continue // Skip cloud-region and node-type as we've already set them
		}

		defaultValue := lockfile.DefaultValues[flag]
		flagInput := struct{ Name, Value string }{Name: flag, Value: defaultValue}
		flagInputs = append(flagInputs, flagInput)
		flagGroups = append(flagGroups,
			huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Placeholder(defaultValue).
				Value(&flagInputs[len(flagInputs)-1].Value),
		)
	}

	var selectedNodeType string
	nodeTypeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select node type").
				Options(getNodeTypeOptions(config.CloudPrefix, lockfile)...).
				Value(&selectedNodeType),
		),
	)

	err = nodeTypeForm.Run()
	if err != nil {
		log.Error("Error in node type selection", "error", err)
		return
	}

	// Extract the actual node type name from the selected value
	nodeTypeName := strings.Split(selectedNodeType, " ")[0]

	// Set the node-type flag to the selected node type name
	config.Flags["node-type"] = nodeTypeName

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		log.Error("Error in flag input form", "error", err)
		return
	}

	// Update config.Flags and lockfile.DefaultValues with the collected values
	for _, fi := range flagInputs {
		config.Flags[fi.Name] = fi.Value
		lockfile.DefaultValues[fi.Name] = fi.Value
	}

	// Ensure cloud-region and node-type is set in the lockfile
	lockfile.DefaultValues["cloud-region"] = config.Region
	lockfile.DefaultValues["node-type"] = selectedNodeType

	err = generateFiles(config)
	if err != nil {
		log.Error("Error generating files", "error", err)
		return
	}

	err = updateLockfile(config, lockfile)
	if err != nil {
		log.Error("Error updating lockfile", "error", err)
		return
	}

	// Define baseDir
	baseDir := filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))

	// Pretty-print the summary
	fmt.Println(style.Render("✅ Configuration completed successfully! Summary:"))
	fmt.Println()

	fmt.Printf("☁️  Cloud Provider: %s\n", config.CloudPrefix)
	fmt.Printf("🌎 Region: %s\n", config.Region)
	fmt.Printf("💻 Node Type: %s\n", selectedNodeType)

	// Print relevant file paths
	fmt.Println(style.Render("\n📁 Generated Files:"))
	filePrefix := "  "
	fmt.Printf("%sInit Script: %s\n", filePrefix, filepath.Join(baseDir, "00-init.sh"))
	fmt.Printf("%sKubefirst Script: %s\n", filePrefix, filepath.Join(baseDir, "01-kubefirst-cloud.sh"))
	fmt.Printf("%sEnvironment File: %s\n", filePrefix, filepath.Join(baseDir, ".local.cloud.env"))

	// Print command to run the generated init script
	fmt.Println(style.Render("\n🚀 To run the initialization script, use the following command:"))
	fmt.Printf("cd %s && ./00-init.sh\n", baseDir)
}

func getCivoClient() (*civogo.Client, error) {
	token := os.Getenv("CIVO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("CIVO_TOKEN not found in environment. Please set it and try again")
	}
	return civogo.NewClient(token, "")
}

func updateCivoRegions(lockfile *Lockfile) error {
	client, err := getCivoClient()
	if err != nil {
		return err
	}

	regions, err := client.ListRegions()
	if err != nil {
		return err
	}

	var regionCodes []string
	for _, region := range regions {
		regionCodes = append(regionCodes, region.Code)
	}

	lockfile.CloudRegions["Civo"] = regionCodes
	return nil
}

func updateCivoNodeTypes(lockfile *Lockfile) error {
	client, err := getCivoClient()
	if err != nil {
		return err
	}

	sizes, err := client.ListInstanceSizes()
	if err != nil {
		return err
	}

	var sizeInfos []InstanceSizeInfo
	for _, size := range sizes {
		sizeInfos = append(sizeInfos, InstanceSizeInfo{
			Name:          size.Name,
			CPUCores:      size.CPUCores,
			RAMMegabytes:  size.RAMMegabytes,
			DiskGigabytes: size.DiskGigabytes,
		})
	}

	lockfile.CloudNodeTypes["Civo"] = sizeInfos
	return nil
}

func getCloudProviderOptions() []huh.Option[string] {
	options := make([]huh.Option[string], len(cloudProviders))
	for i, provider := range cloudProviders {
		options[i] = huh.Option[string]{Key: provider, Value: provider}
	}
	return options
}

func getRegionOptions(cloudProvider string, lockfile Lockfile) []huh.Option[string] {
	regions := lockfile.CloudRegions[cloudProvider]
	options := make([]huh.Option[string], len(regions))
	for i, region := range regions {
		options[i] = huh.Option[string]{Key: region, Value: region}
	}
	return options
}

func getNodeTypeOptions(cloudProvider string, lockfile Lockfile) []huh.Option[string] {
	nodeTypes := lockfile.CloudNodeTypes[cloudProvider]
	options := make([]huh.Option[string], len(nodeTypes))
	for i, nodeType := range nodeTypes {
		displayName := fmt.Sprintf("%s (CPU Cores: %d, RAM: %d MB, Disk: %d GB)",
			nodeType.Name,
			nodeType.CPUCores,
			nodeType.RAMMegabytes,
			nodeType.DiskGigabytes)
		options[i] = huh.Option[string]{
			Key:   nodeType.Name,
			Value: displayName,
		}
	}
	return options
}

func loadLockfile() (Lockfile, error) {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")
	var lockfile Lockfile

	data, err := os.ReadFile(lockfilePath)
	if err == nil {
		var tempMap map[string]interface{}
		err = json.Unmarshal(data, &tempMap)
		if err != nil {
			return lockfile, err
		}

		version, ok := tempMap["version"].(float64)
		if !ok || version < 2 {
			// Old version or no version, migrate data
			err = json.Unmarshal(data, &struct {
				Configs        map[string][]string `json:"configs"`
				DefaultValues  map[string]string   `json:"defaultValues"`
				CloudRegions   map[string][]string `json:"cloudRegions"`
				CloudNodeTypes map[string][]string `json:"cloudNodeTypes"`
			}{
				Configs:        lockfile.Configs,
				DefaultValues:  lockfile.DefaultValues,
				CloudRegions:   lockfile.CloudRegions,
				CloudNodeTypes: make(map[string][]string),
			})
			if err != nil {
				return lockfile, err
			}
			// Convert old CloudNodeTypes to new format
			lockfile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)
		} else {
			// Current version, unmarshal directly
			err = json.Unmarshal(data, &lockfile)
			if err != nil {
				return lockfile, err
			}
		}
	} else if !os.IsNotExist(err) {
		return lockfile, err
	}

	// Initialize maps if they don't exist
	if lockfile.Configs == nil {
		lockfile.Configs = make(map[string][]string)
	}
	if lockfile.DefaultValues == nil {
		lockfile.DefaultValues = make(map[string]string)
	}
	if lockfile.CloudRegions == nil {
		lockfile.CloudRegions = make(map[string][]string)
	}
	if lockfile.CloudNodeTypes == nil {
		lockfile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)
	}

	// Set the current version
	lockfile.Version = 2

	return lockfile, nil
}

func getDigitalOceanSizes() ([]string, error) {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DIGITALOCEAN_TOKEN not found in environment. Please set it and try again")
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	sizes, _, err := client.Sizes.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	var sizeSlugs []string
	for _, size := range sizes {
		sizeSlugs = append(sizeSlugs, size.Slug)
	}

	return sizeSlugs, nil
}

func updateDigitalOceanNodeTypes(lockfile *Lockfile) error {
	sizes, err := getDigitalOceanSizes()
	if err != nil {
		return err
	}

	var sizeInfos []InstanceSizeInfo
	for _, size := range sizes {
		cpuCores, ramMB, diskGB := parseDigitalOceanSize(size)
		sizeInfos = append(sizeInfos, InstanceSizeInfo{
			Name:          size,
			CPUCores:      cpuCores,
			RAMMegabytes:  ramMB,
			DiskGigabytes: diskGB,
		})
	}

	lockfile.CloudNodeTypes["DigitalOcean"] = sizeInfos
	return nil
}

func parseDigitalOceanSize(size string) (cpuCores, ramMB, diskGB int) {
	parts := strings.Split(size, "-")
	if len(parts) < 3 {
		return 0, 0, 0
	}

	cpuStr := strings.TrimSuffix(parts[1], "vcpu")
	cpuCores, _ = strconv.Atoi(cpuStr)

	ramStr := strings.TrimSuffix(parts[2], "gb")
	ramGB, _ := strconv.Atoi(ramStr)
	ramMB = ramGB * 1024

	if len(parts) > 3 {
		diskStr := strings.TrimSuffix(parts[3], "gb")
		diskGB, _ = strconv.Atoi(diskStr)
	}

	return cpuCores, ramMB, diskGB
}

func updateDigitalOceanRegions(lockfile *Lockfile) error {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		return fmt.Errorf("DIGITALOCEAN_TOKEN not found in environment. Please set it and try again")
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	regions, _, err := client.Regions.List(ctx, opt)
	if err != nil {
		return err
	}

	var regionSlugs []string
	for _, region := range regions {
		regionSlugs = append(regionSlugs, region.Slug)
	}

	lockfile.CloudRegions["DigitalOcean"] = regionSlugs
	return nil
}

func generateFiles(config CloudConfig) error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		return err
	}

	// Generate .local.cloud.env
	envContent := generateEnvContent(config)
	err = os.WriteFile(filepath.Join(baseDir, ".local.cloud.env"), []byte(envContent), 0644)
	if err != nil {
		return err
	}

	// Generate 00-init.sh
	initContent := generateInitContent()
	err = os.WriteFile(filepath.Join(baseDir, "00-init.sh"), []byte(initContent), 0755)
	if err != nil {
		return err
	}

	// Generate 01-kubefirst-cloud.sh
	kubefirstContent := generateKubefirstContent(config)
	err = os.WriteFile(filepath.Join(baseDir, "01-kubefirst-cloud.sh"), []byte(kubefirstContent), 0755)
	if err != nil {
		return err
	}

	return nil
}

func generateEnvContent(config CloudConfig) string {
	var content strings.Builder
	for flag, value := range config.Flags {
		content.WriteString(fmt.Sprintf("export %s_%s_%s_%s=\"%s\"\n", config.StaticPrefix, config.CloudPrefix, strings.ToUpper(config.Region), strings.ToUpper(flag), value))
	}
	return content.String()
}

func generateInitContent() string {
	return `#!/bin/bash
op run --env-file="./.local.cloud.env" -- sh ./01-kubefirst-cloud.sh
`
}

func generateKubefirstContent(config CloudConfig) string {
	var content strings.Builder
	content.WriteString("#!/bin/bash\n\n")
	content.WriteString("./prepare/01-check-dependencies.sh\n\n")
	content.WriteString(fmt.Sprintf("kubefirst %s create \\\n", strings.ToLower(config.CloudPrefix)))

	for flag := range config.Flags {
		content.WriteString(fmt.Sprintf("  --%s $%s_%s_%s_%s \\\n", flag, config.StaticPrefix, config.CloudPrefix, strings.ToUpper(config.Region), strings.ToUpper(flag)))
	}

	return content.String()
}

func updateLockfile(config CloudConfig, lockfile Lockfile) error {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")

	// Add the new configuration
	key := fmt.Sprintf("%s_%s", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	lockfile.Configs[key] = []string{
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "00-init.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "01-kubefirst-cloud.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), ".local.cloud.env"),
	}

	// Ensure cloud-region and node-type are set in the lockfile
	lockfile.DefaultValues["cloud-region"] = config.Region
	lockfile.DefaultValues["node-type"] = config.Flags["node-type"]

	// Set the current version
	lockfile.Version = 2

	// Write the updated lockfile
	data, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockfilePath, data, 0644)
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
					Title("The .env file already exists. Do you want to overwrite it?").
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

func runKubefirstSetup() error {
	// Setup Console Environment
	err := setupConsoleEnvironment()
	if err != nil {
		log.Error("Error setting up Console environment", "error", err)
		return err
	}

	// Setup Kubefirst API
	err = setupKubefirstAPI()
	if err != nil {
		log.Error("Error setting up Kubefirst API", "error", err)
		return err
	}

	// Setup Kubefirst
	err = setupKubefirst()
	if err != nil {
		log.Error("Error setting up Kubefirst", "error", err)
		return err
	}

	return nil
}

func setupKubefirstAPI() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	apiDir := filepath.Join(baseDir, "kubefirst-api")

	// Checkout feature branch
	cmd := exec.Command("git", "-C", apiDir, "checkout", "feat-custom-repo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out feat-custom-repo: %w\nOutput: %s", err, output)
	}

	fmt.Println("Checked out feat-custom-repo branch for Kubefirst API")

	// TODO: Add instructions to run the API locally
	fmt.Println("Please follow the instructions in the Kubefirst API README to set up and run the API locally.")

	return nil
}

func setupKubefirst() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	kubefirstDir := filepath.Join(baseDir, "kubefirst")

	// Set K1_LOCAL_DEBUG environment variable
	err := os.Setenv("K1_LOCAL_DEBUG", "true")
	if err != nil {
		return fmt.Errorf("error setting K1_LOCAL_DEBUG: %w", err)
	}

	fmt.Println("Set K1_LOCAL_DEBUG=true for Kubefirst")

	// Checkout feature branch
	cmd := exec.Command("git", "-C", kubefirstDir, "checkout", "feat-custom-repo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out feat-custom-repo: %w\nOutput: %s", err, output)
	}

	fmt.Println("Checked out feat-custom-repo branch for Kubefirst")

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

// Add this function to your main menu or where appropriate
func handleKubefirstSetup() {
	err := runKubefirstSetup()
	if err != nil {
		log.Error("Error in Kubefirst setup", "error", err)
	} else {
		fmt.Println("Kubefirst setup completed successfully!")
	}
}
