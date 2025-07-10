package coordination

import (
	"context"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// ClaimCoordinator defines the interface for host claiming coordination
type ClaimCoordinator interface {
	// ClaimHost attempts to claim a host for the given machine
	ClaimHost(ctx context.Context, request ClaimRequest) (*ClaimResult, error)

	// ReleaseHost releases a host claimed by the given machine
	ReleaseHost(ctx context.Context, machine *infrastructurev1beta1.Beskar7Machine) error
}

// Ensure that HostClaimCoordinator implements ClaimCoordinator
var _ ClaimCoordinator = &HostClaimCoordinator{}

// LeaderElectionCapable is an optional interface that coordinators can implement
// to support leader election-based coordination
type LeaderElectionCapable interface {
	ClaimCoordinator
	ClaimHostWithLeaderElection(ctx context.Context, request ClaimRequest) (*ClaimResult, error)
}
