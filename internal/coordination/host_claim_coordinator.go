package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// HostClaimCoordinator manages concurrent host claiming to prevent race conditions
type HostClaimCoordinator struct {
	client.Client
	maxRetries        int
	backoffFactor     time.Duration
	claimTimeout      time.Duration
	deterministicMode bool
}

// NewHostClaimCoordinator creates a new host claim coordinator
func NewHostClaimCoordinator(client client.Client) *HostClaimCoordinator {
	return &HostClaimCoordinator{
		Client:            client,
		maxRetries:        5,
		backoffFactor:     100 * time.Millisecond,
		claimTimeout:      30 * time.Second,
		deterministicMode: true,
	}
}

// ClaimRequest represents a request to claim a host
type ClaimRequest struct {
	Machine       *infrastructurev1beta1.Beskar7Machine
	ImageURL      string
	RequiredSpecs HostRequirements
}

// HostRequirements defines requirements for host selection
type HostRequirements struct {
	// Future: Add hardware requirements like CPU, memory, storage
	MinCPUCores   int
	MinMemoryGB   int
	RequiredTags  []string
	PreferredTags []string
}

// ClaimResult represents the result of a claim operation
type ClaimResult struct {
	Host         *infrastructurev1beta1.PhysicalHost
	ClaimSuccess bool
	Retry        bool
	RetryAfter   time.Duration
	Error        error
}

// ClaimHost attempts to claim an available host for the given machine
func (c *HostClaimCoordinator) ClaimHost(ctx context.Context, request ClaimRequest) (*ClaimResult, error) {
	logger := log.FromContext(ctx).WithValues(
		"machine", request.Machine.Name,
		"namespace", request.Machine.Namespace,
	)

	logger.Info("Starting host claim process")

	// Check if machine already has a claimed host
	if associatedHost := c.findAssociatedHost(ctx, request.Machine); associatedHost != nil {
		logger.Info("Machine already has associated host", "host", associatedHost.Name)
		return &ClaimResult{
			Host:         associatedHost,
			ClaimSuccess: true,
			Retry:        false,
		}, nil
	}

	// Get available hosts in deterministic order
	availableHosts, err := c.getAvailableHostsOrdered(ctx, request.Machine.Namespace, request.RequiredSpecs)
	if err != nil {
		return nil, fmt.Errorf("failed to get available hosts: %w", err)
	}

	if len(availableHosts) == 0 {
		logger.Info("No available hosts found")
		return &ClaimResult{
			ClaimSuccess: false,
			Retry:        true,
			RetryAfter:   1 * time.Minute,
		}, nil
	}

	// Start with a deterministic host, but fall back across the list to ensure progress
	startHost := c.selectHostDeterministic(availableHosts, request.Machine)

	// Find starting index
	startIndex := 0
	for i := range availableHosts {
		if availableHosts[i].Name == startHost.Name {
			startIndex = i
			break
		}
	}

	// Try each available host in round-robin order starting from startIndex
	for offset := 0; offset < len(availableHosts); offset++ {
		candidate := availableHosts[(startIndex+offset)%len(availableHosts)]
		logger.Info("Selected host for claiming", "host", candidate.Name)

		result, err := c.attemptAtomicClaim(ctx, candidate, request)
		if err != nil {
			// Hard failure trying to claim this host; move on to next
			logger.V(1).Info("Error attempting to claim host; trying next candidate", "host", candidate.Name, "error", err)
			continue
		}
		if result != nil && result.ClaimSuccess {
			return result, nil
		}
		// If not successful, continue to next candidate host
	}

	// If we reach here, none of the available hosts could be claimed now; advise retry
	return &ClaimResult{
		ClaimSuccess: false,
		Retry:        true,
		RetryAfter:   5 * time.Second,
		Error:        fmt.Errorf("no hosts could be claimed at this time"),
	}, nil
}

// findAssociatedHost finds a host already associated with the machine
func (c *HostClaimCoordinator) findAssociatedHost(ctx context.Context, machine *infrastructurev1beta1.Beskar7Machine) *infrastructurev1beta1.PhysicalHost {
	logger := log.FromContext(ctx)

	// First check via ProviderID
	if machine.Spec.ProviderID != nil && *machine.Spec.ProviderID != "" {
		if host := c.getHostByProviderID(ctx, *machine.Spec.ProviderID, machine.Namespace); host != nil {
			return host
		}
	}

	// Then check by ConsumerRef
	hostList := &infrastructurev1beta1.PhysicalHostList{}
	if err := c.List(ctx, hostList, client.InNamespace(machine.Namespace)); err != nil {
		logger.Error(err, "Failed to list hosts to find associated host")
		return nil
	}

	for i := range hostList.Items {
		host := &hostList.Items[i]
		if host.Spec.ConsumerRef != nil &&
			host.Spec.ConsumerRef.Name == machine.Name &&
			host.Spec.ConsumerRef.Namespace == machine.Namespace &&
			host.Spec.ConsumerRef.Kind == machine.Kind {
			return host
		}
	}

	return nil
}

// getHostByProviderID retrieves a host by its provider ID
func (c *HostClaimCoordinator) getHostByProviderID(ctx context.Context, providerID, namespace string) *infrastructurev1beta1.PhysicalHost {
	logger := log.FromContext(ctx)

	// Parse provider ID format: beskar7://namespace/hostname
	if len(providerID) < 11 || providerID[:11] != "beskar7://" {
		return nil
	}

	parts := providerID[11:] // Remove "beskar7://"
	nsHostParts := splitOnce(parts, "/")
	if len(nsHostParts) != 2 {
		return nil
	}

	hostNamespace, hostName := nsHostParts[0], nsHostParts[1]
	if hostNamespace != namespace {
		return nil
	}

	host := &infrastructurev1beta1.PhysicalHost{}
	key := types.NamespacedName{Namespace: hostNamespace, Name: hostName}
	if err := c.Get(ctx, key, host); err != nil {
		logger.V(1).Info("Host not found by provider ID", "providerID", providerID, "error", err)
		return nil
	}

	return host
}

// getAvailableHostsOrdered returns available hosts in deterministic order
func (c *HostClaimCoordinator) getAvailableHostsOrdered(ctx context.Context, namespace string, requirements HostRequirements) ([]*infrastructurev1beta1.PhysicalHost, error) {
	hostList := &infrastructurev1beta1.PhysicalHostList{}
	if err := c.List(ctx, hostList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	var availableHosts []*infrastructurev1beta1.PhysicalHost
	for i := range hostList.Items {
		host := &hostList.Items[i]
		if c.isHostAvailable(host) && c.meetsRequirements(host, requirements) {
			availableHosts = append(availableHosts, host)
		}
	}

	// Sort hosts deterministically by name for consistent selection
	sort.Slice(availableHosts, func(i, j int) bool {
		return availableHosts[i].Name < availableHosts[j].Name
	})

	return availableHosts, nil
}

// isHostAvailable checks if a host is available for claiming
func (c *HostClaimCoordinator) isHostAvailable(host *infrastructurev1beta1.PhysicalHost) bool {
	return host.Spec.ConsumerRef == nil &&
		host.Status.State == infrastructurev1beta1.StateAvailable &&
		host.Status.Ready
}

// meetsRequirements checks if a host meets the specified requirements
func (c *HostClaimCoordinator) meetsRequirements(host *infrastructurev1beta1.PhysicalHost, requirements HostRequirements) bool {
	// For now, all available hosts meet requirements
	// Future: Implement hardware requirement checking
	return true
}

// selectHostDeterministic selects a host using deterministic algorithm
func (c *HostClaimCoordinator) selectHostDeterministic(hosts []*infrastructurev1beta1.PhysicalHost, machine *infrastructurev1beta1.Beskar7Machine) *infrastructurev1beta1.PhysicalHost {
	if !c.deterministicMode || len(hosts) == 1 {
		return hosts[0]
	}

	// Create deterministic hash based on machine name and current day
	// This ensures different machines get different hosts but same machine
	// consistently gets the same host on retries within the same day
	today := time.Now().Format("2006-01-02")
	hashInput := fmt.Sprintf("%s-%s-%s", machine.Namespace, machine.Name, today)
	hash := sha256.Sum256([]byte(hashInput))
	hashStr := hex.EncodeToString(hash[:])

	// Convert first 8 characters of hash to index
	var hashValue uint64
	for _, char := range hashStr[:8] {
		hashValue = hashValue*16 + uint64(hexCharToInt(char))
	}

	selectedIndex := int(hashValue % uint64(len(hosts)))
	return hosts[selectedIndex]
}

// attemptAtomicClaim attempts to atomically claim a host with retries
func (c *HostClaimCoordinator) attemptAtomicClaim(ctx context.Context, host *infrastructurev1beta1.PhysicalHost, request ClaimRequest) (*ClaimResult, error) {
	logger := log.FromContext(ctx).WithValues("host", host.Name, "machine", request.Machine.Name)

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * c.backoffFactor
			logger.V(1).Info("Retrying claim after backoff", "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		// Get the latest version of the host
		latestHost := &infrastructurev1beta1.PhysicalHost{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(host), latestHost); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Host no longer exists")
				return &ClaimResult{
					ClaimSuccess: false,
					Retry:        true,
					RetryAfter:   5 * time.Second,
				}, nil
			}
			return nil, fmt.Errorf("failed to get latest host state: %w", err)
		}

		// Check if host is still available
		if !c.isHostAvailable(latestHost) {
			logger.Info("Host no longer available",
				"consumerRef", latestHost.Spec.ConsumerRef,
				"state", latestHost.Status.State,
				"ready", latestHost.Status.Ready)
			return &ClaimResult{
				ClaimSuccess: false,
				Retry:        true,
				RetryAfter:   5 * time.Second,
			}, nil
		}

		// Attempt atomic claim
		claimedHost := latestHost.DeepCopy()
		claimedHost.Spec.ConsumerRef = &corev1.ObjectReference{
			APIVersion: request.Machine.APIVersion,
			Kind:       request.Machine.Kind,
			Name:       request.Machine.Name,
			Namespace:  request.Machine.Namespace,
			UID:        request.Machine.UID,
		}
		claimedHost.Spec.BootISOSource = &request.ImageURL

		// Add claim timestamp annotation for debugging
		if claimedHost.Annotations == nil {
			claimedHost.Annotations = make(map[string]string)
		}
		claimedHost.Annotations["beskar7.io/claimed-at"] = time.Now().Format(time.RFC3339)
		claimedHost.Annotations["beskar7.io/claimed-by"] = fmt.Sprintf("%s/%s", request.Machine.Namespace, request.Machine.Name)

		// Try to update the host
		if err := c.Update(ctx, claimedHost); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("Optimistic lock conflict during claim", "attempt", attempt+1)
				continue // Retry with backoff
			}
			return nil, fmt.Errorf("failed to update host during claim: %w", err)
		}

		// Update status to reflect claimed state
		latestForStatus := &infrastructurev1beta1.PhysicalHost{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(claimedHost), latestForStatus); err == nil {
			latestForStatus.Status.State = infrastructurev1beta1.StateClaimed
			// Keep Ready true while claimed to allow subsequent operations
			latestForStatus.Status.Ready = true
			if err := c.Status().Update(ctx, latestForStatus); err != nil {
				logger.V(1).Info("Failed to update host status after claim", "error", err)
			}
		} else {
			logger.V(1).Info("Failed to re-fetch host for status update after claim", "error", err)
		}

		logger.Info("Successfully claimed host atomically", "attempt", attempt+1)
		return &ClaimResult{
			Host:         claimedHost,
			ClaimSuccess: true,
			Retry:        false,
		}, nil
	}

	logger.Info("Failed to claim host after maximum retries", "maxRetries", c.maxRetries)
	return &ClaimResult{
		ClaimSuccess: false,
		Retry:        true,
		RetryAfter:   30 * time.Second,
		Error:        fmt.Errorf("failed to claim host after %d attempts due to conflicts", c.maxRetries),
	}, nil
}

// ReleaseHost releases a host claimed by the specified machine
func (c *HostClaimCoordinator) ReleaseHost(ctx context.Context, machine *infrastructurev1beta1.Beskar7Machine) error {
	logger := log.FromContext(ctx).WithValues("machine", machine.Name, "namespace", machine.Namespace)

	host := c.findAssociatedHost(ctx, machine)
	if host == nil {
		logger.Info("No associated host found to release")
		return nil
	}

	logger.Info("Releasing host", "host", host.Name)

	// Verify ownership before releasing
	if host.Spec.ConsumerRef == nil ||
		host.Spec.ConsumerRef.Name != machine.Name ||
		host.Spec.ConsumerRef.Namespace != machine.Namespace {
		logger.Info("Host not owned by this machine, skipping release")
		return nil
	}

	// Clear the claim
	host.Spec.ConsumerRef = nil
	host.Spec.BootISOSource = nil

	// Add release timestamp annotation
	if host.Annotations == nil {
		host.Annotations = make(map[string]string)
	}
	host.Annotations["beskar7.io/released-at"] = time.Now().Format(time.RFC3339)

	// Update the host
	if err := c.Update(ctx, host); err != nil {
		return fmt.Errorf("failed to release host %s: %w", host.Name, err)
	}

	// Update status to Available
	latestForStatus := &infrastructurev1beta1.PhysicalHost{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(host), latestForStatus); err == nil {
		latestForStatus.Status.State = infrastructurev1beta1.StateAvailable
		latestForStatus.Status.Ready = true
		if err := c.Status().Update(ctx, latestForStatus); err != nil {
			logger.V(1).Info("Failed to update host status after release", "error", err)
		}
	} else {
		logger.V(1).Info("Failed to re-fetch host for status update after release", "error", err)
	}

	logger.Info("Successfully released host")
	return nil
}

// Helper functions

func splitOnce(s, sep string) []string {
	parts := make([]string, 0, 2)
	if idx := findString(s, sep); idx >= 0 {
		parts = append(parts, s[:idx], s[idx+len(sep):])
	} else {
		parts = append(parts, s)
	}
	return parts
}

func findString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func hexCharToInt(char rune) int {
	switch {
	case char >= '0' && char <= '9':
		return int(char - '0')
	case char >= 'a' && char <= 'f':
		return int(char - 'a' + 10)
	case char >= 'A' && char <= 'F':
		return int(char - 'A' + 10)
	default:
		return 0
	}
}
