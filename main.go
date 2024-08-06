package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/digitalocean/godo"
)

type CloudConfig struct {
	StaticPrefix string
	CloudPrefix  string
	Region       string
	Flags        map[string]string
}

type Lockfile struct {
	Configs        map[string][]string `json:"configs"`
	DefaultValues  map[string]string   `json:"defaultValues"`
	CloudRegions   map[string][]string `json:"cloudRegions"`
	CloudNodeTypes map[string][]string `json:"cloudNodeTypes"`
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
	config := CloudConfig{
		Flags: make(map[string]string),
	}

	lockfile, err := loadLockfile()
	if err != nil {
		fmt.Println("Error loading lockfile:", err)
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
		fmt.Println("Error:", err)
		return
	}

	if config.CloudPrefix == "DigitalOcean" {
		err = updateDigitalOceanRegions(&lockfile)
		if err != nil {
			fmt.Println("Error updating DigitalOcean regions:", err)
			return
		}
		err = updateDigitalOceanNodeTypes(&lockfile)
		if err != nil {
			fmt.Println("Error updating DigitalOcean node types:", err)
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
		fmt.Println("Error:", err)
		return
	}

	flags := cloudFlags[config.CloudPrefix]
	if len(flags) == 0 {
		fmt.Println("No flags found for the selected cloud provider.")
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

	// Node type selection
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
		fmt.Println("Error:", err)
		return
	}

	// Set the node-type flag to the selected node type
	config.Flags["node-type"] = selectedNodeType

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		fmt.Println("Error:", err)
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
		fmt.Println("Error generating files:", err)
		return
	}

	err = updateLockfile(config, lockfile)
	if err != nil {
		fmt.Println("Error updating lockfile:", err)
		return
	}

	fmt.Println("Configuration completed successfully!")
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

func loadLockfile() (Lockfile, error) {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")
	var lockfile Lockfile

	data, err := os.ReadFile(lockfilePath)
	if err == nil {
		err = json.Unmarshal(data, &lockfile)
		if err != nil {
			return lockfile, err
		}
	} else if !os.IsNotExist(err) {
		return lockfile, err
	}

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
		lockfile.CloudNodeTypes = make(map[string][]string)
	}

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

func getNodeTypeOptions(cloudProvider string, lockfile Lockfile) []huh.Option[string] {
	nodeTypes := lockfile.CloudNodeTypes[cloudProvider]
	options := make([]huh.Option[string], len(nodeTypes))
	for i, nodeType := range nodeTypes {
		options[i] = huh.Option[string]{Key: nodeType, Value: nodeType}
	}
	return options
}

func updateDigitalOceanNodeTypes(lockfile *Lockfile) error {
	sizes, err := getDigitalOceanSizes()
	if err != nil {
		return err
	}

	lockfile.CloudNodeTypes["DigitalOcean"] = sizes
	return nil
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

	// Write the updated lockfile
	data, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockfilePath, data, 0644)
}
