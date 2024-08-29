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

	columnStyle = boxStyle.Copy().
			Width(90).
			Height(25)

	consoleStyle      = columnStyle.Copy().BorderForeground(lipgloss.Color("#00FFFF"))
	kubefirstAPIStyle = columnStyle.Copy().BorderForeground(lipgloss.Color("#FF00FF"))
	kubefirstStyle    = boxStyle.Copy().BorderForeground(lipgloss.Color("#FFFF00")).Width(180)
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

	// Render Kubefirst-API and Console logs
	apiLogPath := getLogPath("kubefirst-api")
	consoleLogPath := getLogPath("console")

	apiLogs := kubefirstAPIStyle.Render(
		titleStyle.Render("Kubefirst-API Logs") + "\n" +
			pathStyle.Render(apiLogPath) + "\n" +
			formatLogs(kubefirstAPILogs, 88, 20),
	)

	consoleLogsRendered := consoleStyle.Render(
		titleStyle.Render("Console Logs") + "\n" +
			pathStyle.Render(consoleLogPath) + "\n" +
			formatLogs(consoleLogs, 88, 20),
	)

	row := lipgloss.JoinHorizontal(lipgloss.Top, apiLogs, consoleLogsRendered)
	doc.WriteString(row)

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
