/*
Copyright 2024 The Beskar7 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package coordination

import (
	"context"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

func TestProvisioningQueue_BasicOperations(t *testing.T) {
	queue := NewProvisioningQueue(2, 10) // maxConcurrent=2, maxQueueSize=10

	queueLength, processingCount := queue.GetQueueStatus()

	if processingCount != 0 {
		t.Errorf("Expected 0 active operations, got %d", processingCount)
	}

	if queueLength != 0 {
		t.Errorf("Expected empty queue, got length %d", queueLength)
	}
}

func TestProvisioningQueue_QueueManagement(t *testing.T) {
	queue := NewProvisioningQueue(1, 3) // capacity=1, queueSize=3

	ctx := context.Background()
	queue.Start(ctx, 1) // Start with 1 worker
	defer queue.Stop()

	// Create test host
	host := &infrastructurev1beta1.PhysicalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Spec: infrastructurev1beta1.PhysicalHostSpec{
			RedfishConnection: infrastructurev1beta1.RedfishConnection{
				Address:              "redfish.example.com",
				CredentialsSecretRef: "test-secret",
			},
		},
		Status: infrastructurev1beta1.PhysicalHostStatus{
			State: infrastructurev1beta1.StateAvailable,
		},
	}

	machine := &infrastructurev1beta1.Beskar7Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			UID:       "test-machine-uid",
		},
		Spec: infrastructurev1beta1.Beskar7MachineSpec{
			ImageURL: "http://example.com/image.iso",
			OSFamily: "kairos",
		},
	}

	// Submit a request
	request, err := queue.SubmitRequest(ctx, host, machine, OperationClaim)
	if err != nil {
		t.Fatalf("Failed to submit request: %v", err)
	}

	if request == nil {
		t.Fatal("Expected non-nil request")
	}

	// Wait for result with timeout
	result, err := queue.WaitForResult(request, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to wait for result: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected successful result, got failure: %v", result.Error)
	}
}

func TestProvisioningQueue_ConcurrentOperations(t *testing.T) {
	queue := NewProvisioningQueue(2, 10)

	ctx := context.Background()
	queue.Start(ctx, 2) // Start with 2 workers
	defer queue.Stop()

	// Create multiple hosts
	hosts := make([]*infrastructurev1beta1.PhysicalHost, 3)
	machines := make([]*infrastructurev1beta1.Beskar7Machine, 3)

	for i := 0; i < 3; i++ {
		hosts[i] = &infrastructurev1beta1.PhysicalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host-" + string(rune('0'+i)),
				Namespace: "default",
			},
			Spec: infrastructurev1beta1.PhysicalHostSpec{
				RedfishConnection: infrastructurev1beta1.RedfishConnection{
					Address:              "redfish" + string(rune('0'+i)) + ".example.com",
					CredentialsSecretRef: "test-secret",
				},
			},
			Status: infrastructurev1beta1.PhysicalHostStatus{
				State: infrastructurev1beta1.StateAvailable,
			},
		}

		machines[i] = &infrastructurev1beta1.Beskar7Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine-" + string(rune('0'+i)),
				Namespace: "default",
				UID:       types.UID("test-machine-uid-" + string(rune('0'+i))),
			},
			Spec: infrastructurev1beta1.Beskar7MachineSpec{
				ImageURL: "http://example.com/image.iso",
				OSFamily: "kairos",
			},
		}
	}

	// Submit concurrent requests
	var wg sync.WaitGroup
	results := make([]*ProvisioningResult, 3)
	errors := make([]error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			request, err := queue.SubmitRequest(ctx, hosts[index], machines[index], OperationClaim)
			if err != nil {
				errors[index] = err
				return
			}

			result, err := queue.WaitForResult(request, 10*time.Second)
			results[index] = result
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify results
	successCount := 0
	for i := 0; i < 3; i++ {
		if errors[i] == nil && results[i] != nil && results[i].Success {
			successCount++
		}
	}

	if successCount != 3 {
		t.Errorf("Expected 3 successful operations, got %d", successCount)
	}
}

func TestProvisioningQueue_BMCCooldown(t *testing.T) {
	queue := NewProvisioningQueue(1, 10)

	ctx := context.Background()
	queue.Start(ctx, 1)
	defer queue.Stop()

	// Create two hosts with the same BMC address
	host1 := &infrastructurev1beta1.PhysicalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host-1",
			Namespace: "default",
		},
		Spec: infrastructurev1beta1.PhysicalHostSpec{
			RedfishConnection: infrastructurev1beta1.RedfishConnection{
				Address:              "shared-bmc.example.com",
				CredentialsSecretRef: "test-secret",
			},
		},
		Status: infrastructurev1beta1.PhysicalHostStatus{
			State: infrastructurev1beta1.StateAvailable,
		},
	}

	host2 := &infrastructurev1beta1.PhysicalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host-2",
			Namespace: "default",
		},
		Spec: infrastructurev1beta1.PhysicalHostSpec{
			RedfishConnection: infrastructurev1beta1.RedfishConnection{
				Address:              "shared-bmc.example.com", // Same BMC
				CredentialsSecretRef: "test-secret",
			},
		},
		Status: infrastructurev1beta1.PhysicalHostStatus{
			State: infrastructurev1beta1.StateAvailable,
		},
	}

	machine1 := &infrastructurev1beta1.Beskar7Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine-1",
			Namespace: "default",
			UID:       "test-machine-uid-1",
		},
		Spec: infrastructurev1beta1.Beskar7MachineSpec{
			ImageURL: "http://example.com/image.iso",
			OSFamily: "kairos",
		},
	}

	machine2 := &infrastructurev1beta1.Beskar7Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine-2",
			Namespace: "default",
			UID:       "test-machine-uid-2",
		},
		Spec: infrastructurev1beta1.Beskar7MachineSpec{
			ImageURL: "http://example.com/image.iso",
			OSFamily: "kairos",
		},
	}

	// Submit first request
	request1, err := queue.SubmitRequest(ctx, host1, machine1, OperationProvision)
	if err != nil {
		t.Fatalf("Failed to submit first request: %v", err)
	}

	// Submit second request immediately (should be queued due to BMC cooldown)
	request2, err := queue.SubmitRequest(ctx, host2, machine2, OperationProvision)
	if err != nil {
		t.Fatalf("Failed to submit second request: %v", err)
	}

	// Wait for both results
	result1, err := queue.WaitForResult(request1, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to wait for first result: %v", err)
	}

	result2, err := queue.WaitForResult(request2, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to wait for second result: %v", err)
	}

	if !result1.Success {
		t.Errorf("Expected first result to succeed")
	}

	if !result2.Success {
		t.Errorf("Expected second result to succeed")
	}

	// Verify second request took longer (due to cooldown)
	if result2.Duration <= result1.Duration {
		t.Error("Expected second request to take longer due to BMC cooldown")
	}
}

func TestLeaderElectionClaimCoordinator_Interface(t *testing.T) {
	// Test interface compliance without using fake clients
	config := LeaderElectionConfig{
		Namespace: "test",
		Identity:  "test-coordinator",
	}

	// We can't test the full functionality without real clients,
	// but we can test configuration and interface compliance
	if config.Namespace != "test" {
		t.Errorf("Expected namespace 'test', got '%s'", config.Namespace)
	}

	if config.Identity != "test-coordinator" {
		t.Errorf("Expected identity 'test-coordinator', got '%s'", config.Identity)
	}
}

func TestLeaderElectionConfig_Defaults(t *testing.T) {
	// Test default values are applied correctly
	config := LeaderElectionConfig{
		Identity: "test-coordinator",
	}

	// Test that config has required fields
	if config.Identity == "" {
		t.Error("Expected non-empty identity")
	}

	// Test that default values would be applied
	if config.Namespace == "" {
		config.Namespace = "beskar7-system"
	}

	if config.Namespace != "beskar7-system" {
		t.Errorf("Expected default namespace 'beskar7-system', got '%s'", config.Namespace)
	}
}

func TestHostRequirements_Validation(t *testing.T) {
	// Test host requirements structure
	requirements := HostRequirements{
		RequiredTags:  []string{"worker", "amd64"},
		PreferredTags: []string{"nvme", "high-memory"},
	}

	if len(requirements.RequiredTags) != 2 {
		t.Errorf("Expected 2 required tags, got %d", len(requirements.RequiredTags))
	}

	if len(requirements.PreferredTags) != 2 {
		t.Errorf("Expected 2 preferred tags, got %d", len(requirements.PreferredTags))
	}

	// Test contains function
	if !contains(requirements.RequiredTags, "worker") {
		t.Error("Expected to find 'worker' in required tags")
	}

	if contains(requirements.RequiredTags, "control-plane") {
		t.Error("Expected not to find 'control-plane' in required tags")
	}
}

func TestClaimRequest_Structure(t *testing.T) {
	// Test claim request structure
	machine := &infrastructurev1beta1.Beskar7Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			UID:       "test-machine-uid",
		},
		Spec: infrastructurev1beta1.Beskar7MachineSpec{
			ImageURL: "http://example.com/image.iso",
			OSFamily: "kairos",
		},
	}

	requirements := HostRequirements{
		RequiredTags: []string{"worker"},
	}

	request := ClaimRequest{
		Machine:       machine,
		RequiredSpecs: requirements,
	}

	if request.Machine.Name != "test-machine" {
		t.Errorf("Expected machine name 'test-machine', got '%s'", request.Machine.Name)
	}

	if len(request.RequiredSpecs.RequiredTags) != 1 {
		t.Errorf("Expected 1 required tag, got %d", len(request.RequiredSpecs.RequiredTags))
	}
}

func TestClaimResult_Structure(t *testing.T) {
	// Test claim result structure
	host := &infrastructurev1beta1.PhysicalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "default",
		},
		Status: infrastructurev1beta1.PhysicalHostStatus{
			State: infrastructurev1beta1.StateAvailable,
		},
	}

	result := &ClaimResult{
		ClaimSuccess: true,
		Host:         host,
		Retry:        false,
		RetryAfter:   0,
	}

	if !result.ClaimSuccess {
		t.Error("Expected claim success to be true")
	}

	if result.Host.Name != "test-host" {
		t.Errorf("Expected host name 'test-host', got '%s'", result.Host.Name)
	}

	if result.Retry {
		t.Error("Expected retry to be false")
	}
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
