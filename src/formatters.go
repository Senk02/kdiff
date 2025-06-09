package main

import (
	"fmt"

	"github.com/fatih/color"
)

// formats memory values in human-readable format
func FormatMemory(bytes int64) string {
	if bytes == 0 {
		return "0"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ci", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatPercentage colors the output string based on percentage difference and thresholds
// hasComparison indicates whether resource requests/limits are actually set
func FormatPercentage(p float64, hasComparison bool, thresholds ColorThresholds) string {
	c := color.New()

	if !hasComparison {
		c.Add(color.FgMagenta) // magenta for no requests/limits set
		return c.Sprintf("inf%%")
	}

	switch {
	case p >= thresholds.RedThreshold:
		c.Add(color.FgRed) // over-utilized
	case p >= thresholds.YellowThreshold:
		c.Add(color.FgYellow) // well-utilized
	case p >= thresholds.CyanThreshold:
		c.Add(color.FgGreen) // under-utilized
	default:
		c.Add(color.FgCyan) // very under-utilized
	}
	return c.Sprintf("%.2f%%", p)
}
