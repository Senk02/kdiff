package main

// holds threshold values for color coding resource usage percentages
type ColorThresholds struct {
	RedThreshold    float64 // above this percentage = red (over-utilized)
	YellowThreshold float64 // above this percentage = yellow (well-utilized)
	CyanThreshold   float64 // below this percentage = cyan (very under-utilized)
	// between cyan and yellow = green (under-utilized)
}