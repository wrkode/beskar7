package coordination

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// BenchmarkClaimRequest_Creation benchmarks ClaimRequest creation
func BenchmarkClaimRequest_Creation(b *testing.B) {
	machine := &infrastructurev1beta1.Beskar7Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test-namespace",
		},
		Spec: infrastructurev1beta1.Beskar7MachineSpec{
			ImageURL: "http://example.com/image.iso",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		request := ClaimRequest{
			Machine:  machine,
			ImageURL: fmt.Sprintf("http://example.com/image-%d.iso", i),
			RequiredSpecs: HostRequirements{
				MinCPUCores:  2,
				MinMemoryGB:  8,
				RequiredTags: []string{"worker"},
			},
		}
		_ = request // Use the request to avoid optimization
	}
}

// BenchmarkHostRequirements_Validation benchmarks host requirements validation
func BenchmarkHostRequirements_Validation(b *testing.B) {
	requirements := HostRequirements{
		MinCPUCores:   4,
		MinMemoryGB:   16,
		RequiredTags:  []string{"worker", "zone-a"},
		PreferredTags: []string{"ssd", "high-memory"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate validation logic
		isValid := requirements.MinCPUCores > 0 &&
			requirements.MinMemoryGB > 0 &&
			len(requirements.RequiredTags) > 0

		if !isValid {
			b.Fatalf("Requirements validation failed")
		}
	}
}

// BenchmarkClaimResult_Processing benchmarks ClaimResult processing
func BenchmarkClaimResult_Processing(b *testing.B) {
	host := &infrastructurev1beta1.PhysicalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-namespace",
		},
		Status: infrastructurev1beta1.PhysicalHostStatus{
			State: infrastructurev1beta1.StateAvailable,
			Ready: true,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result := &ClaimResult{
			Host:         host,
			ClaimSuccess: true,
			Retry:        false,
			RetryAfter:   0,
			Error:        nil,
		}

		// Simulate result processing
		if result.ClaimSuccess && result.Host != nil {
			// Success path
			continue
		}

		if result.Retry {
			// Retry logic
			time.Sleep(result.RetryAfter)
		}
	}
}

// BenchmarkHostRequirements_Matching benchmarks host matching logic
func BenchmarkHostRequirements_Matching(b *testing.B) {
	requirements := HostRequirements{
		MinCPUCores:   2,
		MinMemoryGB:   8,
		RequiredTags:  []string{"worker"},
		PreferredTags: []string{"ssd"},
	}

	// Create test hosts with varying specifications
	hosts := make([]*infrastructurev1beta1.PhysicalHost, 100)
	for i := 0; i < 100; i++ {
		hosts[i] = &infrastructurev1beta1.PhysicalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("host-%d", i),
				Namespace: "test-namespace",
				Labels: map[string]string{
					"cpu-cores": fmt.Sprintf("%d", 2+(i%8)),  // 2-10 cores
					"memory-gb": fmt.Sprintf("%d", 8+(i%32)), // 8-40 GB
					"node-type": "worker",
					"storage":   []string{"hdd", "ssd"}[i%2],
				},
			},
			Status: infrastructurev1beta1.PhysicalHostStatus{
				State: infrastructurev1beta1.StateAvailable,
				Ready: true,
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matchingHosts := 0
		for _, host := range hosts {
			// Simulate matching logic
			if host.Status.State == infrastructurev1beta1.StateAvailable &&
				host.Status.Ready {

				// Check required tags
				hasRequiredTags := true
				for _, requiredTag := range requirements.RequiredTags {
					if nodeType, exists := host.Labels["node-type"]; !exists || nodeType != requiredTag {
						hasRequiredTags = false
						break
					}
				}

				if hasRequiredTags {
					matchingHosts++
				}
			}
		}

		if matchingHosts == 0 {
			// No matching hosts found
			continue
		}
	}
}

// BenchmarkContextOperations benchmarks context operations
func BenchmarkContextOperations(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Simulate some work with context
		select {
		case <-ctx.Done():
			// Context cancelled
		default:
			// Normal operation
		}

		cancel()
	}
}

// BenchmarkStringFormatting benchmarks string formatting operations
func BenchmarkStringFormatting(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		machineName := fmt.Sprintf("machine-%d", i)
		hostName := fmt.Sprintf("host-%d", i)
		logMessage := fmt.Sprintf("Claiming host %s for machine %s", hostName, machineName)
		_ = logMessage // Use the string to avoid optimization
	}
}

// BenchmarkSliceOperations benchmarks slice operations common in coordination
func BenchmarkSliceOperations(b *testing.B) {
	tags := []string{"worker", "zone-a", "ssd", "high-memory", "gpu", "network-optimized"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate tag filtering
		filteredTags := make([]string, 0, len(tags))
		for _, tag := range tags {
			if len(tag) > 3 { // Simple filter
				filteredTags = append(filteredTags, tag)
			}
		}
		_ = filteredTags
	}
}

// BenchmarkMapOperations benchmarks map operations for labels and annotations
func BenchmarkMapOperations(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		labels := make(map[string]string)
		labels["node-type"] = "worker"
		labels["zone"] = fmt.Sprintf("zone-%d", i%5)
		labels["rack"] = fmt.Sprintf("rack-%d", i%10)
		labels["cpu-cores"] = fmt.Sprintf("%d", 2+(i%16))
		labels["memory-gb"] = fmt.Sprintf("%d", 8+(i%120))

		// Simulate label lookup
		if nodeType, exists := labels["node-type"]; exists && nodeType == "worker" {
			// Valid worker node
			continue
		}
	}
}
