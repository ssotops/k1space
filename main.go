package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
)

type CloudConfig struct {
	StaticPrefix string
	CloudPrefix  string
	Region       string
	Flags        map[string]string
}

type Lockfile struct {
	Configs map[string][]string `json:"configs"`
}

var cloudProviders = []string{
	"Akamai",
	"AWS",
	"Civo",
	"DigitalOcean",
	"Google Cloud",
	"Vultr",
	"K3s",
	"K3d",
}

var cloudRegions = map[string][]string{
	"Akamai":       {"us-east", "us-west", "eu-central"},
	"AWS":          {"us-east-1", "us-west-2", "eu-west-1"},
	"Civo":         {"NYC1", "LON1", "FRA1"},
	"DigitalOcean": {"nyc3", "sfo2", "ams3"},
	"Google Cloud": {"us-central1", "europe-west1", "asia-east1"},
	"Vultr":        {"ewr", "lax", "ams"},
	"K3s":          {"local"},
	"K3d":          {"local"},
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

	err := form.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Options(getRegionOptions(config.CloudPrefix)...).
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

	flagInputs := make([]struct{ Name, Value string }, len(flags))
	flagGroups := make([]huh.Field, 0, len(flags))
	for i, flag := range flags {
		flagInputs[i] = struct{ Name, Value string }{Name: flag}
		flagGroups = append(flagGroups,
			huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Value(&flagInputs[i].Value),
		)
	}

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Update config.Flags with the collected values
	for _, fi := range flagInputs {
		config.Flags[fi.Name] = fi.Value
	}

	err = generateFiles(config)
	if err != nil {
		fmt.Println("Error generating files:", err)
		return
	}

	err = updateLockfile(config)
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

func getRegionOptions(cloudProvider string) []huh.Option[string] {
	regions := cloudRegions[cloudProvider]
	options := make([]huh.Option[string], len(regions))
	for i, region := range regions {
		options[i] = huh.Option[string]{Key: region, Value: region}
	}
	return options
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

func updateLockfile(config CloudConfig) error {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")
	var lockfile Lockfile

	// Read existing lockfile if it exists
	data, err := os.ReadFile(lockfilePath)
	if err == nil {
		err = json.Unmarshal(data, &lockfile)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Initialize the map if it doesn't exist
	if lockfile.Configs == nil {
		lockfile.Configs = make(map[string][]string)
	}

	// Add the new configuration
	key := fmt.Sprintf("%s_%s", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	lockfile.Configs[key] = []string{
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "00-init.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "01-kubefirst-cloud.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), ".local.cloud.env"),
	}

	// Write the updated lockfile
	data, err = json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockfilePath, data, 0644)
}
