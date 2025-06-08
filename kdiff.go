package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

// holds threshold values for color coding resource usage percentages
type ColorThresholds struct {
	RedThreshold    float64 // above this percentage = red (over-utilized)
	YellowThreshold float64 // above this percentage = yellow (well-utilized)
	CyanThreshold   float64 // below this percentage = cyan (very under-utilized)
	// between cyan and yellow = green (under-utilized)
}

func main() {
	var namespace, outputFilter, kubeContext string
	var showDifferenceOnly, allNamespaces, ignoreUnset, showHelp bool
	var colorThresholds ColorThresholds

	flag.StringVar(&namespace, "n", "", "The namespace to check (defaults to current context namespace)")
	flag.StringVar(&outputFilter, "o", "", "Filter output to 'cpu' or 'memory'")
	flag.StringVar(&kubeContext, "context", "", "The kubeconfig context to use")
	flag.BoolVar(&showDifferenceOnly, "d", false, "Only show pods with usage over requests")
	flag.BoolVar(&allNamespaces, "a", false, "Check all namespaces")
	flag.BoolVar(&ignoreUnset, "i", false, "Ignore pods without resource requests set")
	flag.BoolVar(&ignoreUnset, "ignore-unset", false, "Ignore pods without resource requests set")
	flag.BoolVar(&showHelp, "h", false, "Show help message")

	flag.Float64Var(&colorThresholds.RedThreshold, "color-red", 0.0, "Percentage threshold for red color (over-utilized)")
	flag.Float64Var(&colorThresholds.YellowThreshold, "color-yellow", -20.0, "Percentage threshold for yellow color (well-utilized)")
	flag.Float64Var(&colorThresholds.CyanThreshold, "color-cyan", -90.0, "Percentage threshold for cyan color (very under-utilized)")

	flag.Parse()

	if showHelp {
		showHelpMenu()
		return
	}

	if err := validateColorThresholds(&colorThresholds); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	var config *rest.Config
	var err error

	if kubeContext != "" {
		// load config with specific context override
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
			&clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating kubeconfig: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure your kubeconfig is valid and accessible\n")
		os.Exit(1)
	}

	// get current context namespace if no namespace specified and not all namespaces
	if namespace == "" && !allNamespaces {
		kubeconfigRaw, err := clientcmd.LoadFromFile(kubeconfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading kubeconfig: %v\n", err)
			os.Exit(1)
		}

		contextToUse := kubeContext
		if contextToUse == "" {
			contextToUse = kubeconfigRaw.CurrentContext
		}

		if context, exists := kubeconfigRaw.Contexts[contextToUse]; exists && context.Namespace != "" {
			namespace = context.Namespace
		} else {
			namespace = "default"
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Kubernetes clientset: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please check your cluster connection and credentials\n")
		os.Exit(1)
	}

	metricsClientset, err := metrics.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating metrics clientset: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure metrics-server is installed and running in your cluster\n")
		os.Exit(1)
	}

	if err := testMetricsAPI(metricsClientset); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to metrics API: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure metrics-server is properly installed and running\n")
		os.Exit(1)
	}

	var namespacesToCheck []string
	if allNamespaces {
		fmt.Print("Fetching namespaces... ")
		namespaceList, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError fetching namespaces: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("found %d namespaces\n", len(namespaceList.Items))

		for _, ns := range namespaceList.Items {
			namespacesToCheck = append(namespacesToCheck, ns.Name)
		}
	} else {
		namespacesToCheck = []string{namespace}
	}

	// use slice of slices of `any` to hold all row data, as required by Bulk()
	var allRows [][]any
	totalPods := 0
	processedPods := 0

	// first pass: count total pods to process for progress tracking
	for _, ns := range namespacesToCheck {
		pods, err := clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error fetching pods in namespace %s: %v\n", ns, err)
			continue
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == v1.PodRunning {
				totalPods++
			}
		}
	}

	if totalPods == 0 {
		fmt.Println("No running pods found in the specified namespace(s)")
		return
	}

	fmt.Printf("Processing %d running pods...\n", totalPods)

	for _, ns := range namespacesToCheck {
		pods, err := clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error fetching pods in namespace %s: %v\n", ns, err)
			continue
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase != v1.PodRunning {
				continue
			}

			processedPods++
			fmt.Printf("\rProgress: %d/%d pods", processedPods, totalPods)

			podMetrics, err := metricsClientset.MetricsV1beta1().PodMetricses(ns).Get(context.TODO(), pod.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nWarning: Could not get metrics for pod %s in namespace %s: %v\n", pod.Name, ns, err)
				continue
			}

			var requestedCPU, requestedMemory, usedCPU, usedMemory int64
			var cpuDiff, memDiff float64
			var overLimitCPU, overLimitMem bool
			var cpuHasRequests, memHasRequests bool

			for _, container := range pod.Spec.Containers {
				requestedCPU += container.Resources.Requests.Cpu().MilliValue()
				requestedMemory += container.Resources.Requests.Memory().Value()
			}

			cpuHasRequests = requestedCPU > 0
			memHasRequests = requestedMemory > 0

			if ignoreUnset && !cpuHasRequests && !memHasRequests {
				continue
			}

			for _, container := range podMetrics.Containers {
				usedCPU += container.Usage.Cpu().MilliValue()
				usedMemory += container.Usage.Memory().Value()
			}

			if cpuHasRequests {
				cpuDiff = (float64(usedCPU-requestedCPU) / float64(requestedCPU)) * 100
				if usedCPU > requestedCPU {
					overLimitCPU = true
				}
			}

			if memHasRequests {
				memDiff = (float64(usedMemory-requestedMemory) / float64(requestedMemory)) * 100
				if usedMemory > requestedMemory {
					overLimitMem = true
				}
			}

			if showDifferenceOnly && !overLimitCPU && !overLimitMem {
				continue
			}

			var rowStrings []string
			if allNamespaces {
				rowStrings = []string{ns, pod.Name}
			} else {
				rowStrings = []string{pod.Name}
			}

			if outputFilter == "" || outputFilter == "memory" {
				rowStrings = append(rowStrings,
					formatMemory(requestedMemory),
					formatMemory(usedMemory),
					formatPercentage(memDiff, memHasRequests, colorThresholds))
			}
			if outputFilter == "" || outputFilter == "cpu" {
				rowStrings = append(rowStrings,
					fmt.Sprintf("%dm", requestedCPU),
					fmt.Sprintf("%dm", usedCPU),
					formatPercentage(cpuDiff, cpuHasRequests, colorThresholds))
			}

			// convert []string row to []any row for the Bulk method
			rowAnys := make([]any, len(rowStrings))
			for i, v := range rowStrings {
				rowAnys[i] = v
			}
			allRows = append(allRows, rowAnys)
		}
	}

	fmt.Printf("\rProgress: %d/%d pods - Complete!\n\n", processedPods, totalPods)

	if len(allRows) == 0 {
		fmt.Println("No pods match the specified criteria")
		return
	}

	var headerStrings []string
	if allNamespaces {
		headerStrings = []string{"NAMESPACE", "POD"}
	} else {
		headerStrings = []string{"POD"}
	}

	if outputFilter == "" || outputFilter == "memory" {
		headerStrings = append(headerStrings, "REQUESTED MEMORY", "USED MEMORY", "MEMORY DIFF (%)")
	}
	if outputFilter == "" || outputFilter == "cpu" {
		headerStrings = append(headerStrings, "REQUESTED CPU", "USED CPU", "CPU DIFF (%)")
	}

	// convert header from []string to []any for the Header() method
	headerAnys := make([]any, len(headerStrings))
	for i, v := range headerStrings {
		headerAnys[i] = v
	}

	table := tablewriter.NewTable(os.Stdout)
	table.Header(headerAnys...)
	table.Bulk(allRows)
	table.Render()
}

func validateColorThresholds(thresholds *ColorThresholds) error {
	if thresholds.CyanThreshold >= thresholds.YellowThreshold {
		return fmt.Errorf("cyan threshold (%.1f%%) must be less than yellow threshold (%.1f%%)",
			thresholds.CyanThreshold, thresholds.YellowThreshold)
	}
	if thresholds.YellowThreshold >= thresholds.RedThreshold {
		return fmt.Errorf("yellow threshold (%.1f%%) must be less than red threshold (%.1f%%)",
			thresholds.YellowThreshold, thresholds.RedThreshold)
	}
	return nil
}

func testMetricsAPI(metricsClientset *metrics.Clientset) error {
	_, err := metricsClientset.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{Limit: 1})
	return err
}

func showHelpMenu() {
	fmt.Println("Kubernetes Resource Monitor - Compare pod resource requests vs actual usage")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [flags]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -n string          The namespace to check (defaults to current context namespace)")
	fmt.Println("  -o string          Filter output to 'cpu' or 'memory'")
	fmt.Println("  --context string   The kubeconfig context to use")
	fmt.Println("  -d                 Only show pods with usage over requests")
	fmt.Println("  -a                 Check all namespaces")
	fmt.Println("  -i, --ignore-unset Ignore pods without resource requests set")
	fmt.Println("  -h                 Show this help message")
	fmt.Println()
	fmt.Println("Color Customization:")
	fmt.Printf("  --color-%s float    Percentage threshold for red color (default: 0.0)\n", color.RedString("red"))
	fmt.Printf("  --color-%s float Percentage threshold for yellow color (default: -20.0)\n", color.YellowString("yellow"))
	fmt.Printf("  --color-%s float   Percentage threshold for cyan color (default: -90.0)\n", color.CyanString("cyan"))
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Check current namespace")
	fmt.Printf("  %s\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Check specific namespace")
	fmt.Printf("  %s -n kube-system\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Check all namespaces, only show CPU usage")
	fmt.Printf("  %s -a -o cpu\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Show only pods exceeding requests, ignore pods without requests")
	fmt.Printf("  %s -d -i\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Customize color thresholds")
	fmt.Printf("  %s --color-%s -10 --color-%s 5 --color-%s -80\n",
		os.Args[0],
		color.YellowString("yellow"),
		color.RedString("red"),
		color.CyanString("cyan"))
	fmt.Println()
	fmt.Println("Color Legend:")
	fmt.Println("  " + color.CyanString("Cyan") + "   - Very under-utilized")
	fmt.Println("  " + color.GreenString("Green") + "  - Under-utilized")
	fmt.Println("  " + color.YellowString("Yellow") + " - Warning-utilized")
	fmt.Println("  " + color.RedString("Red") + "    - Over-utilized")
	fmt.Println("  " + color.MagentaString("Magenta") + " - No resource requests set (inf%)")
}

func formatMemory(bytes int64) string {
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

// colors the output string based on percentage difference and thresholds
// hasRequests indicates whether resource requests are actually set
func formatPercentage(p float64, hasRequests bool, thresholds ColorThresholds) string {
	c := color.New()

	if !hasRequests {
		c.Add(color.FgMagenta) // magenta for no requests set
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