package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/olekukonko/tablewriter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	var namespace, outputFilter, kubeconfig, kubeContext, mode string
	var showDifferenceOnly, allNamespaces, ignoreUnset, showHelp bool
	var colorThresholds ColorThresholds
	var colorRedSet, colorYellowSet, colorCyanSet bool

	flag.StringVar(&namespace, "n", "", "The namespace to check (defaults to current context namespace)")
	flag.StringVar(&outputFilter, "o", "", "Filter output to 'cpu' or 'memory'")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (defaults to ~/.kube/config)")
	flag.StringVar(&kubeContext, "context", "", "The kubeconfig context to use")
	flag.StringVar(&mode, "m", "requests", "Mode: 'requests' (default) or 'limits'")
	flag.StringVar(&mode, "mode", "requests", "Mode: 'requests' (default) or 'limits'")
	flag.BoolVar(&showDifferenceOnly, "d", false, "Only show pods with usage over requests/limits")
	flag.BoolVar(&allNamespaces, "a", false, "Check all namespaces")
	flag.BoolVar(&allNamespaces, "A", false, "Check all namespaces")
	flag.BoolVar(&ignoreUnset, "i", false, "Ignore pods without resource requests/limits set")
	flag.BoolVar(&ignoreUnset, "ignore-unset", false, "Ignore pods without resource requests/limits set")
	flag.BoolVar(&showHelp, "h", false, "Show help message")

	// simple color threshold flags - defaults will be set based on mode
	flag.Float64Var(&colorThresholds.RedThreshold, "color-red", 0.0, "Percentage threshold for red color (over-utilized)")
	flag.Float64Var(&colorThresholds.YellowThreshold, "color-yellow", 0.0, "Percentage threshold for yellow color (well-utilized)")
	flag.Float64Var(&colorThresholds.CyanThreshold, "color-cyan", 0.0, "Percentage threshold for cyan color (very under-utilized)")

	flag.Parse()

	// track which color flags were explicitly set
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "color-red":
			colorRedSet = true
		case "color-yellow":
			colorYellowSet = true
		case "color-cyan":
			colorCyanSet = true
		}
	})

	if showHelp {
		ShowHelpMenu()
		return
	}

	// validate mode
	mode = strings.ToLower(mode)
	if mode != "requests" && mode != "limits" {
		fmt.Fprintf(os.Stderr, "Error: mode must be 'requests' or 'limits', got '%s'\n", mode)
		os.Exit(1)
	}

	// set defaults based on mode for any color thresholds not explicitly set
	if mode == "requests" {
		if !colorRedSet {
			colorThresholds.RedThreshold = 0.0
		}
		if !colorYellowSet {
			colorThresholds.YellowThreshold = -20.0
		}
		if !colorCyanSet {
			colorThresholds.CyanThreshold = -90.0
		}
	} else { // limits mode
		if !colorRedSet {
			colorThresholds.RedThreshold = -10.0
		}
		if !colorYellowSet {
			colorThresholds.YellowThreshold = -40.0
		}
		if !colorCyanSet {
			colorThresholds.CyanThreshold = -80.0
		}
	}

	if err := ValidateColorThresholds(&colorThresholds); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// set default kubeconfig path if not provided
	if kubeconfig == "" {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

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

	if err := TestMetricsAPI(metricsClientset); err != nil {
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

	fmt.Printf("Processing %d running pods in %s mode...\n", totalPods, mode)

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

			var comparedCPU, comparedMemory, usedCPU, usedMemory int64
			var cpuDiff, memDiff float64
			var overLimitCPU, overLimitMem bool
			var cpuHasComparison, memHasComparison bool

			for _, container := range pod.Spec.Containers {
				if mode == "requests" {
					comparedCPU += container.Resources.Requests.Cpu().MilliValue()
					comparedMemory += container.Resources.Requests.Memory().Value()
				} else {
					comparedCPU += container.Resources.Limits.Cpu().MilliValue()
					comparedMemory += container.Resources.Limits.Memory().Value()
				}
			}

			cpuHasComparison = comparedCPU > 0
			memHasComparison = comparedMemory > 0

			if ignoreUnset && !cpuHasComparison && !memHasComparison {
				continue
			}

			for _, container := range podMetrics.Containers {
				usedCPU += container.Usage.Cpu().MilliValue()
				usedMemory += container.Usage.Memory().Value()
			}

			if cpuHasComparison {
				cpuDiff = (float64(usedCPU-comparedCPU) / float64(comparedCPU)) * 100
				if usedCPU > comparedCPU {
					overLimitCPU = true
				}
			}

			if memHasComparison {
				memDiff = (float64(usedMemory-comparedMemory) / float64(comparedMemory)) * 100
				if usedMemory > comparedMemory {
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
					FormatMemory(comparedMemory),
					FormatMemory(usedMemory),
					FormatPercentage(memDiff, memHasComparison, colorThresholds))
			}
			if outputFilter == "" || outputFilter == "cpu" {
				rowStrings = append(rowStrings,
					fmt.Sprintf("%dm", comparedCPU),
					fmt.Sprintf("%dm", usedCPU),
					FormatPercentage(cpuDiff, cpuHasComparison, colorThresholds))
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
		memoryHeader := "MEMORY " + strings.ToUpper(mode)
		headerStrings = append(headerStrings, memoryHeader, "USED MEMORY", "MEMORY DIFF (%)")
	}
	if outputFilter == "" || outputFilter == "cpu" {
		cpuHeader := "CPU " + strings.ToUpper(mode)
		headerStrings = append(headerStrings, cpuHeader, "USED CPU", "CPU DIFF (%)")
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