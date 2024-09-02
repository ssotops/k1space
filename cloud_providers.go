package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/civo/civogo"
	"github.com/digitalocean/godo"
)

func getCivoClient() (*civogo.Client, error) {
	token := os.Getenv("CIVO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("CIVO_TOKEN not found in environment. Please set it and try again")
	}
	return civogo.NewClient(token, "")
}

func updateCivoRegions(cloudsFile *CloudsFile) error {
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

	cloudsFile.CloudRegions["Civo"] = regionCodes
	return nil
}

func updateCivoNodeTypes(cloudsFile *CloudsFile) error {
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

	cloudsFile.CloudNodeTypes["Civo"] = sizeInfos
	return nil
}

func getDigitalOceanClient() (*godo.Client, error) {
	token := os.Getenv("DO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DO_TOKEN not found in environment. Please set it and try again")
	}
	return godo.NewFromToken(token), nil
}

func updateDigitalOceanRegions(cloudsFile *CloudsFile) error {
	client, err := getDigitalOceanClient()
	if err != nil {
		return err
	}

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

	cloudsFile.CloudRegions["DigitalOcean"] = regionSlugs
	return nil
}

func updateDigitalOceanNodeTypes(cloudsFile *CloudsFile) error {
	client, err := getDigitalOceanClient()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	sizes, _, err := client.Sizes.List(ctx, opt)
	if err != nil {
		return err
	}

	var sizeInfos []InstanceSizeInfo
	for _, size := range sizes {
		cpuCores, ramMB, diskGB := parseDigitalOceanSize(size.Slug)
		sizeInfos = append(sizeInfos, InstanceSizeInfo{
			Name:          size.Slug,
			CPUCores:      cpuCores,
			RAMMegabytes:  ramMB,
			DiskGigabytes: diskGB,
		})
	}

	cloudsFile.CloudNodeTypes["DigitalOcean"] = sizeInfos
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

func getCloudProviderOptions() []huh.Option[string] {
	options := make([]huh.Option[string], len(cloudProviders))
	for i, provider := range cloudProviders {
		options[i] = huh.Option[string]{Key: provider, Value: provider}
	}
	return options
}

func getRegionOptions(cloudProvider string, cloudsFile CloudsFile) []huh.Option[string] {
	regions := cloudsFile.CloudRegions[cloudProvider]
	options := make([]huh.Option[string], len(regions))
	for i, region := range regions {
		options[i] = huh.Option[string]{Key: region, Value: region}
	}
	return options
}

func getNodeTypeOptions(cloudProvider string, cloudsFile CloudsFile) []huh.Option[string] {
	nodeTypes := cloudsFile.CloudNodeTypes[cloudProvider]
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

func checkRequiredTokens(cloudProvider string) (bool, string) {
    var tokenName, instructions string
    var tokenExists bool

    switch cloudProvider {
    case "Civo":
        tokenName = "CIVO_TOKEN"
        instructions = "You can create a new Civo API token at https://www.civo.com/account/security"
    case "DigitalOcean":
        tokenName = "DO_TOKEN"
        instructions = "You can create a new DigitalOcean API token at https://cloud.digitalocean.com/account/api/tokens"
    default:
        return true, ""
    }

    tokenExists = os.Getenv(tokenName) != ""
    message := fmt.Sprintf(`
╔════════════════════════════════════════════════════════════════════════════╗
║ Missing Required Token: %s                                                 
║────────────────────────────────────────────────────────────────────────────
║ The %s environment variable is not set.
║ 
║ To set it, run the following command in your terminal:
║ export %s=your_token_here
║ 
║ %s
║ 
║ After setting the token, please restart k1space.
╚════════════════════════════════════════════════════════════════════════════╝
`, tokenName, tokenName, tokenName, instructions)

    return tokenExists, message
}
