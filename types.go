package main

import (
	"github.com/charmbracelet/lipgloss"
  "sync"
	"time"
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
	StaticPrefix     string
	CloudPrefix      string
	Region           string
	Flags            *sync.Map
	SelectedNodeType string
}

func NewCloudConfig() *CloudConfig {
	return &CloudConfig{
		StaticPrefix:     "",
		CloudPrefix:      "",
		Region:           "",
		Flags:            &sync.Map{},
		SelectedNodeType: "",
	}
}

type IndexFile struct {
	Version       int               `hcl:"version"`
	LastUpdated   string            `hcl:"last_updated"`
	Configs       map[string]Config `hcl:"configs"`
	DefaultValues map[string]string `hcl:"default_values"`
}

type Config struct {
	Files []string          `hcl:"files"`
	Flags map[string]string `hcl:"flags,omitempty"`
}

type CloudsFile struct {
	LastUpdated    string                        `hcl:"last_updated"`
	CloudRegions   map[string][]string           `hcl:"cloud_regions"`
	CloudNodeTypes map[string][]InstanceSizeInfo `hcl:"cloud_node_types"`
}

type InstanceSizeInfo struct {
	Name          string
	CPUCores      int
	RAMMegabytes  int
	DiskGigabytes int
}

// GitHubRelease represents the structure of a GitHub release
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
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
