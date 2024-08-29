package main

import (
	"strings"

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

	boxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	summaryStyle = boxStyle.Copy().
			BorderForeground(highlight).
			Width(120).
			Align(lipgloss.Center)

	columnStyle = boxStyle.Copy().
			Width(40).
			Height(20)

	consoleStyle      = columnStyle.Copy().BorderForeground(lipgloss.Color("#00FFFF"))
	kubefirstAPIStyle = columnStyle.Copy().BorderForeground(lipgloss.Color("#FF00FF"))
	kubefirstStyle    = columnStyle.Copy().BorderForeground(lipgloss.Color("#FFFF00"))
)

func renderDashboard(kubefirstAPILogs, consoleLogs, kubefirstLogs, summary string) string {
	doc := strings.Builder{}

	doc.WriteString(summaryStyle.Render(summary))
	doc.WriteString("\n\n")

	apiLogs := kubefirstAPIStyle.Render(titleStyle.Render("Kubefirst-API Logs") + "\n" + truncateOrWrap(kubefirstAPILogs, 38))
	consoleLogsRendered := consoleStyle.Render(titleStyle.Render("Console Logs") + "\n" + truncateOrWrap(consoleLogs, 38))
	kubefirstLogsRendered := kubefirstStyle.Render(titleStyle.Render("Kubefirst Logs") + "\n" + truncateOrWrap(kubefirstLogs, 38))

	row := lipgloss.JoinHorizontal(lipgloss.Top, apiLogs, consoleLogsRendered, kubefirstLogsRendered)
	doc.WriteString(row)

	return doc.String()
}

func truncateOrWrap(s string, width int) string {
	var result strings.Builder
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if len(line) > width {
			result.WriteString(line[:width-3] + "...\n")
		} else {
			result.WriteString(line + "\n")
		}
	}
	return result.String()
}
