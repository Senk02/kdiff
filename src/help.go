package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

// display the help info
func ShowHelpMenu() {
	fmt.Println("Kdiff - Compare pod resource requests/limits vs actual usage")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [flags]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -n string            The namespace to check (defaults to current context namespace)")
	fmt.Println("  -o string            Filter output to 'cpu' or 'memory'")
	fmt.Println("  --context string     The kubeconfig context to use")
	fmt.Println("  -m, --mode string    Mode: 'requests' (default) or 'limits'")
	fmt.Println("  -d                   Only show pods with usage over requests/limits")
	fmt.Println("  -a, -A               Check all namespaces")
	fmt.Println("  -i, --ignore-unset   Ignore pods without resource requests/limits set")
	fmt.Println("  -h                   Show this help message")
	fmt.Println()
	fmt.Println("Color Customization:")
	fmt.Printf("  --color-%s float            Percentage threshold for red color\n", color.RedString("red"))
	fmt.Printf("  --color-%s float         Percentage threshold for yellow color\n", color.YellowString("yellow"))
	fmt.Printf("  --color-%s float           Percentage threshold for cyan color\n", color.CyanString("cyan"))
	fmt.Println()
	fmt.Println("Default Color Thresholds:")
	fmt.Println("  Requests Mode:")
	fmt.Printf("    %s: above %.1f%% (over requests), %s: %.1f%% (warning zone), %s: well-utilized, %s: below %.1f%% (very under-utilized)\n",
		color.RedString("Red"), 0.0,
		color.YellowString("Yellow"), -20.0,
		color.GreenString("Green"),
		color.CyanString("Cyan"), -90.0)
	fmt.Println("  Limits Mode:")
	fmt.Printf("    %s: above %.1f%% (near limits), %s: %.1f%% (warning zone), %s: well-utilized, %s: below %.1f%% (very under-utilized)\n",
		color.RedString("Red"), -10.0,
		color.YellowString("Yellow"), -40.0,
		color.GreenString("Green"),
		color.CyanString("Cyan"), -80.0)
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Check current namespace (requests mode)")
	fmt.Printf("  %s\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Check specific namespace with limits mode")
	fmt.Printf("  %s -n kube-system --mode limits\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Check all namespaces, only show CPU usage in limits mode")
	fmt.Printf("  %s -A -o cpu -m limits\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Show only pods exceeding requests, ignore pods without requests")
	fmt.Printf("  %s -d -i\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Customize color thresholds")
	fmt.Printf("  %s --mode limits --color-%s -5 --color-%s -30 --color-%s -70\n",
		os.Args[0],
		color.RedString("red"),
		color.YellowString("yellow"),
		color.CyanString("cyan"))
	fmt.Println()
	fmt.Println("Mode Differences:")
	fmt.Println("  Requests Mode: Compares usage against resource requests (what pods ask for)")
	fmt.Println("  Limits Mode:   Compares usage against resource limits (maximum allowed)")
	fmt.Println("                 Usually shows negative percentages since limits are typically higher")
	fmt.Println()
	fmt.Println("Color Legend:")
	fmt.Println("  " + color.CyanString("Cyan") + "   - Very under-utilized")
	fmt.Println("  " + color.GreenString("Green") + "  - Well-utilized")
	fmt.Println("  " + color.YellowString("Yellow") + " - Warning zone")
	fmt.Println("  " + color.RedString("Red") + "    - Over-utilized")
	fmt.Println("  " + color.MagentaString("Magenta") + " - No resource requests/limits set (inf%)")
}
