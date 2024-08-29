package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"time"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	titleStyle = lipgloss.NewStyle().
			Foreground(special).
			Padding(0, 1).
			Bold(true)

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

	// Render summary and Kubefirst logs
	summary := fmt.Sprintf("Kubefirst repositories running\nStatus: All systems operational\nLast updated: %s", time.Now().Format("15:04:05"))
	kubefirstLogsContent := formatLogs(kubefirstLogs, 178, 5)
	topSection := summaryStyle.Render(summary + "\n\n" + titleStyle.Render("Kubefirst Logs") + "\n" + kubefirstLogsContent)
	doc.WriteString(topSection)
	doc.WriteString("\n\n")

	// Render Kubefirst-API and Console logs
	apiLogs := kubefirstAPIStyle.Render(titleStyle.Render("Kubefirst-API Logs") + "\n" + formatLogs(kubefirstAPILogs, 88, 23))
	consoleLogsRendered := consoleStyle.Render(titleStyle.Render("Console Logs") + "\n" + formatLogs(consoleLogs, 88, 23))

	row := lipgloss.JoinHorizontal(lipgloss.Top, apiLogs, consoleLogsRendered)
	doc.WriteString(row)

	return doc.String()
}

func formatLogs(logs *scrollingLog, width, height int) string {
	var result strings.Builder
	lines := logs.getLastN(height)
	for _, line := range lines {
		result.WriteString(truncateOrWrap(line, width) + "\n")
	}
	return result.String()
}

func truncateOrWrap(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width-3] + "..."
}
