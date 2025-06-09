package main

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

// ensures color thresholds are in correct order
func ValidateColorThresholds(thresholds *ColorThresholds) error {
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

// tests connectivity to the metrics API
func TestMetricsAPI(metricsClientset *metrics.Clientset) error {
	_, err := metricsClientset.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{Limit: 1})
	return err
}
