package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	titleStyle = lipgloss.NewStyle().
			Foreground(special).
			Padding(0, 1).
			Bold(true)

	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	boxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	summaryStyle = boxStyle.Copy().
			BorderForeground(highlight).
			Width(180).
			Align(lipgloss.Center)

	kubefirstStyle = boxStyle.Copy().
			BorderForeground(lipgloss.Color("#FFFF00")).
			Width(180)

	consoleStyle = boxStyle.Copy().
			BorderForeground(lipgloss.Color("#00FFFF")).
			Width(180)

	kubefirstAPIStyle = boxStyle.Copy().
				BorderForeground(lipgloss.Color("#FF00FF")).
				Width(180)

	// New styles from clusters.go
	clusterTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7D56F4")).
				Bold(true).
				Padding(0, 1)

	filePathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5F9F9F")).
			Italic(true)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(1).
			Width(100)

	configStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF69B4")).
			Padding(1).
			Width(100)
)

func renderDashboard(kubefirstAPILogs, consoleLogs, kubefirstLogs *scrollingLog) string {
	doc := strings.Builder{}

	// Render summary
	summary := fmt.Sprintf("Kubefirst repositories running\nStatus: All systems operational\nLast updated: %s", time.Now().Format("15:04:05"))
	doc.WriteString(summaryStyle.Render(summary))
	doc.WriteString("\n\n")

	// Render Kubefirst logs
	kubefirstLogPath := getLogPath("kubefirst")
	kubefirstLogsContent := formatLogs(kubefirstLogs, 178, 3)
	kubefirstLogsSection := kubefirstStyle.Render(
		titleStyle.Render("Kubefirst Logs") + "\n" +
			pathStyle.Render(kubefirstLogPath) + "\n" +
			kubefirstLogsContent,
	)
	doc.WriteString(kubefirstLogsSection)
	doc.WriteString("\n\n")

	// Render Console logs
	consoleLogPath := getLogPath("console")
	consoleLogsContent := formatLogs(consoleLogs, 178, 10)
	consoleLogsSection := consoleStyle.Render(
		titleStyle.Render("Console Logs") + "\n" +
			pathStyle.Render(consoleLogPath) + "\n" +
			consoleLogsContent,
	)
	doc.WriteString(consoleLogsSection)
	doc.WriteString("\n\n")

	// Render Kubefirst-API logs
	apiLogPath := getLogPath("kubefirst-api")
	apiLogsContent := formatLogs(kubefirstAPILogs, 178, 20)
	apiLogsSection := kubefirstAPIStyle.Render(
		titleStyle.Render("Kubefirst-API Logs") + "\n" +
			pathStyle.Render(apiLogPath) + "\n" +
			apiLogsContent,
	)
	doc.WriteString(apiLogsSection)

	return doc.String()
}

func formatLogs(logs *scrollingLog, width, height int) string {
	var result strings.Builder
	lines := logs.getLastN(height)
	for _, line := range lines {
		result.WriteString(truncateOrWrap(removeDateFromLog(line), width) + "\n")
	}
	return result.String()
}

func truncateOrWrap(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width-3] + "..."
}

func removeDateFromLog(log string) string {
	parts := strings.SplitN(log, "]", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return log
}

func getLogPath(serviceName string) string {
	homeDir, _ := os.UserHomeDir()
	logDir := filepath.Join(homeDir, ".ssot", "k1space", ".logs")

	files, err := os.ReadDir(logDir)
	if err != nil {
		return "Error reading log directory"
	}

	var latestFile string
	var latestTime time.Time

	prefix := serviceName + "-"
	for _, file := range files {
		if strings.HasPrefix(file.Name(), prefix) {
			filePath := filepath.Join(logDir, file.Name())
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				continue
			}
			if fileInfo.ModTime().After(latestTime) {
				latestFile = filePath
				latestTime = fileInfo.ModTime()
			}
		}
	}

	if latestFile == "" {
		return "No log file found for " + serviceName
	}

	return latestFile
}

func renderClusterProvisioningTUI(selectedConfig string, configContent string, fileContents []string, filePaths []string) string {
	var sb strings.Builder

	// Config summary
	sb.WriteString(configStyle.Render(clusterTitleStyle.Render("Configuration Summary") + "\n" + configContent))
	sb.WriteString("\n\n")

	// File contents
	for i, content := range fileContents {
		fileName := filepath.Base(filePaths[i])
		sb.WriteString(contentStyle.Render(
			clusterTitleStyle.Render(fileName) + "\n" +
				filePathStyle.Render(filePaths[i]) + "\n\n" +
				content,
		))
		sb.WriteString("\n\n")
	}

	return sb.String()
}
