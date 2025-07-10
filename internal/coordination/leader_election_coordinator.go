package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	internalmetrics "github.com/wrkode/beskar7/internal/metrics"
)

// LeaderElectionClaimCoordinator provides leader election-based coordination for host claims
type LeaderElectionClaimCoordinator struct {
	client.Client
	kubernetesClient kubernetes.Interface
	namespace        string
	identity         string
	leaseDuration    time.Duration
	renewDeadline    time.Duration
	retryPeriod      time.Duration

	// Leader election state
	mu               sync.RWMutex
	isLeader         bool
	leaderElector    *leaderelection.LeaderElector
	leadershipLostCh chan struct{}

	// Claim coordination state
	pendingClaims      map[string]*LeaderCoordinatedClaim // keyed by machine UID
	claimQueue         []*LeaderCoordinatedClaim
	processingInterval time.Duration
	stopCh             chan struct{}

	// Fallback coordinator for non-leader instances
	fallbackCoordinator *HostClaimCoordinator
}

// LeaderCoordinatedClaim represents a claim being coordinated by the leader
type LeaderCoordinatedClaim struct {
	ID              string
	Machine         *infrastructurev1beta1.Beskar7Machine
	Request         ClaimRequest
	ResultCh        chan *ClaimResult
	SubmittedAt     time.Time
	ProcessedAt     *time.Time
	RequesterNodeID string
	Priority        int
	RetryCount      int
}

// LeaderElectionConfig configures the leader election claim coordinator
type LeaderElectionConfig struct {
	Namespace          string
	Identity           string
	LeaseDuration      time.Duration
	RenewDeadline      time.Duration
	RetryPeriod        time.Duration
	ProcessingInterval time.Duration
}

// NewLeaderElectionClaimCoordinator creates a new leader election-based claim coordinator
func NewLeaderElectionClaimCoordinator(client client.Client, kubernetesClient kubernetes.Interface, config LeaderElectionConfig) *LeaderElectionClaimCoordinator {
	if config.LeaseDuration == 0 {
		config.LeaseDuration = 15 * time.Second
	}
	if config.RenewDeadline == 0 {
		config.RenewDeadline = 10 * time.Second
	}
	if config.RetryPeriod == 0 {
		config.RetryPeriod = 2 * time.Second
	}
	if config.ProcessingInterval == 0 {
		config.ProcessingInterval = 1 * time.Second
	}
	if config.Namespace == "" {
		config.Namespace = "beskar7-system"
	}

	return &LeaderElectionClaimCoordinator{
		Client:              client,
		kubernetesClient:    kubernetesClient,
		namespace:           config.Namespace,
		identity:            config.Identity,
		leaseDuration:       config.LeaseDuration,
		renewDeadline:       config.RenewDeadline,
		retryPeriod:         config.RetryPeriod,
		processingInterval:  config.ProcessingInterval,
		pendingClaims:       make(map[string]*LeaderCoordinatedClaim),
		claimQueue:          make([]*LeaderCoordinatedClaim, 0),
		leadershipLostCh:    make(chan struct{}),
		stopCh:              make(chan struct{}),
		fallbackCoordinator: NewHostClaimCoordinator(client),
	}
}

// Start initializes the leader election and starts the coordinator
func (lec *LeaderElectionClaimCoordinator) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues(
		"component", "LeaderElectionClaimCoordinator",
		"identity", lec.identity,
		"namespace", lec.namespace,
	)

	logger.Info("Starting leader election claim coordinator")

	// Setup leader election
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "beskar7-claim-coordinator-leader",
			Namespace: lec.namespace,
		},
		Client: lec.kubernetesClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: lec.identity,
		},
	}

	// Configure leader election
	leaderElectionConfig := leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: lec.leaseDuration,
		RenewDeadline: lec.renewDeadline,
		RetryPeriod:   lec.retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				lec.onStartedLeading(ctx)
			},
			OnStoppedLeading: func() {
				lec.onStoppedLeading()
			},
			OnNewLeader: func(identity string) {
				lec.onNewLeader(identity)
			},
		},
	}

	leaderElector, err := leaderelection.NewLeaderElector(leaderElectionConfig)
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	lec.leaderElector = leaderElector

	// Start leader election in a goroutine
	go func() {
		defer logger.Info("Leader election stopped")
		leaderElector.Run(ctx)
	}()

	logger.Info("Leader election claim coordinator started")
	return nil
}

// Stop gracefully shuts down the coordinator
func (lec *LeaderElectionClaimCoordinator) Stop() {
	close(lec.stopCh)

	lec.mu.Lock()
	if lec.isLeader {
		// Process any remaining claims before stepping down
		lec.processRemainingClaims()
	}
	lec.mu.Unlock()
}

// ClaimHost implements the ClaimCoordinator interface by delegating to ClaimHostWithLeaderElection
func (lec *LeaderElectionClaimCoordinator) ClaimHost(ctx context.Context, request ClaimRequest) (*ClaimResult, error) {
	return lec.ClaimHostWithLeaderElection(ctx, request)
}

// ClaimHostWithLeaderElection attempts to claim a host using leader election coordination
func (lec *LeaderElectionClaimCoordinator) ClaimHostWithLeaderElection(ctx context.Context, request ClaimRequest) (*ClaimResult, error) {
	logger := log.FromContext(ctx).WithValues(
		"machine", request.Machine.Name,
		"namespace", request.Machine.Namespace,
	)

	// Check if we should use leader election or fallback
	lec.mu.RLock()
	isLeader := lec.isLeader
	lec.mu.RUnlock()

	// If we're the leader, process the claim directly
	if isLeader {
		return lec.processClaimAsLeader(ctx, request)
	}

	// If leader election is enabled but we're not the leader, delegate to leader
	if lec.shouldUseLeaderElection(ctx) {
		return lec.delegateClaimToLeader(ctx, request)
	}

	// Fallback to optimistic locking approach
	logger.V(1).Info("Using fallback coordinator (no leader or leader election disabled)")
	return lec.fallbackCoordinator.ClaimHost(ctx, request)
}

// onStartedLeading is called when this instance becomes the leader
func (lec *LeaderElectionClaimCoordinator) onStartedLeading(ctx context.Context) {
	logger := log.FromContext(ctx).WithValues("component", "LeaderElectionClaimCoordinator")
	logger.Info("Became leader for claim coordination")

	lec.mu.Lock()
	lec.isLeader = true
	lec.mu.Unlock()

	// Record leader election metrics
	internalmetrics.RecordClaimCoordinatorLeaderElection(lec.namespace, "started_leading")

	// Start processing claims as leader
	go lec.processClaimsAsLeader(ctx)
}

// onStoppedLeading is called when this instance loses leadership
func (lec *LeaderElectionClaimCoordinator) onStoppedLeading() {
	logger := log.FromContext(context.Background()).WithValues("component", "LeaderElectionClaimCoordinator")
	logger.Info("Lost leadership for claim coordination")

	lec.mu.Lock()
	lec.isLeader = false
	lec.mu.Unlock()

	// Signal that leadership was lost
	select {
	case lec.leadershipLostCh <- struct{}{}:
	default:
	}

	// Record leader election metrics
	internalmetrics.RecordClaimCoordinatorLeaderElection(lec.namespace, "stopped_leading")
}

// onNewLeader is called when a new leader is elected
func (lec *LeaderElectionClaimCoordinator) onNewLeader(identity string) {
	logger := log.FromContext(context.Background()).WithValues(
		"component", "LeaderElectionClaimCoordinator",
		"newLeader", identity,
	)
	logger.Info("New leader elected for claim coordination")

	// Record leader election metrics
	internalmetrics.RecordClaimCoordinatorLeaderElection(lec.namespace, "new_leader")
}

// processClaimAsLeader processes a claim when this instance is the leader
func (lec *LeaderElectionClaimCoordinator) processClaimAsLeader(ctx context.Context, request ClaimRequest) (*ClaimResult, error) {
	logger := log.FromContext(ctx).WithValues(
		"component", "LeaderElectionClaimCoordinator",
		"machine", request.Machine.Name,
	)

	startTime := time.Now()
	logger.Info("Processing claim as leader")

	// Use the fallback coordinator's logic but with leader coordination benefits
	result, err := lec.fallbackCoordinator.ClaimHost(ctx, request)

	// Record metrics
	duration := time.Since(startTime)
	if err != nil {
		internalmetrics.RecordHostClaimDuration(request.Machine.Namespace, internalmetrics.ClaimOutcomeError, duration)
		internalmetrics.RecordClaimCoordinatorResult(request.Machine.Namespace, "error")
	} else if result.ClaimSuccess {
		internalmetrics.RecordHostClaimDuration(request.Machine.Namespace, internalmetrics.ClaimOutcomeSuccess, duration)
		internalmetrics.RecordClaimCoordinatorResult(request.Machine.Namespace, "success")
	} else {
		internalmetrics.RecordClaimCoordinatorResult(request.Machine.Namespace, "retry")
	}

	return result, err
}

// delegateClaimToLeader sends a claim request to the leader
func (lec *LeaderElectionClaimCoordinator) delegateClaimToLeader(ctx context.Context, request ClaimRequest) (*ClaimResult, error) {
	logger := log.FromContext(ctx).WithValues(
		"component", "LeaderElectionClaimCoordinator",
		"machine", request.Machine.Name,
	)

	logger.Info("Delegating claim to leader")

	// For now, fall back to optimistic locking since we need a distributed coordination mechanism
	// In a full implementation, this would use a shared queue or messaging system
	logger.Info("Leader delegation not yet implemented, falling back to optimistic locking")
	return lec.fallbackCoordinator.ClaimHost(ctx, request)
}

// processClaimsAsLeader processes claims in the queue as the leader
func (lec *LeaderElectionClaimCoordinator) processClaimsAsLeader(ctx context.Context) {
	logger := log.FromContext(ctx).WithValues("component", "LeaderElectionClaimCoordinator")
	logger.Info("Starting claim processing as leader")

	ticker := time.NewTicker(lec.processingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Context cancelled, stopping claim processing")
			return
		case <-lec.leadershipLostCh:
			logger.Info("Leadership lost, stopping claim processing")
			return
		case <-lec.stopCh:
			logger.Info("Stop signal received, stopping claim processing")
			return
		case <-ticker.C:
			lec.processClaimBatch(ctx)
		}
	}
}

// processClaimBatch processes a batch of pending claims
func (lec *LeaderElectionClaimCoordinator) processClaimBatch(ctx context.Context) {
	lec.mu.Lock()
	defer lec.mu.Unlock()

	if !lec.isLeader || len(lec.claimQueue) == 0 {
		return
	}

	logger := log.FromContext(ctx).WithValues(
		"component", "LeaderElectionClaimCoordinator",
		"queueLength", len(lec.claimQueue),
	)

	// Process claims in priority order
	claimsToProcess := make([]*LeaderCoordinatedClaim, len(lec.claimQueue))
	copy(claimsToProcess, lec.claimQueue)
	lec.claimQueue = lec.claimQueue[:0] // Clear the queue

	logger.V(1).Info("Processing claim batch", "claimCount", len(claimsToProcess))

	for _, claim := range claimsToProcess {
		lec.processIndividualClaim(ctx, claim)
	}
}

// processIndividualClaim processes a single claim
func (lec *LeaderElectionClaimCoordinator) processIndividualClaim(ctx context.Context, claim *LeaderCoordinatedClaim) {
	logger := log.FromContext(ctx).WithValues(
		"claimID", claim.ID,
		"machine", claim.Machine.Name,
	)

	startTime := time.Now()
	result, err := lec.fallbackCoordinator.ClaimHost(ctx, claim.Request)

	now := time.Now()
	claim.ProcessedAt = &now

	if err != nil {
		result = &ClaimResult{
			ClaimSuccess: false,
			Error:        err,
			Retry:        true,
			RetryAfter:   30 * time.Second,
		}
		logger.Error(err, "Failed to process claim")
	}

	// Send result back
	select {
	case claim.ResultCh <- result:
		logger.V(1).Info("Claim result sent", "success", result.ClaimSuccess)
	default:
		logger.Info("Result channel full or closed, claim result dropped")
	}

	// Record metrics
	duration := time.Since(startTime)
	if result.ClaimSuccess {
		internalmetrics.RecordClaimCoordinatorProcessing(claim.Machine.Namespace, "success", duration)
	} else {
		internalmetrics.RecordClaimCoordinatorProcessing(claim.Machine.Namespace, "failed", duration)
	}
}

// processRemainingClaims processes any claims still in the queue before stepping down
func (lec *LeaderElectionClaimCoordinator) processRemainingClaims() {
	logger := log.FromContext(context.Background()).WithValues("component", "LeaderElectionClaimCoordinator")

	if len(lec.claimQueue) > 0 {
		logger.Info("Processing remaining claims before stepping down", "remainingClaims", len(lec.claimQueue))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		lec.processClaimBatch(ctx)
	}
}

// shouldUseLeaderElection determines if leader election should be used for coordination
func (lec *LeaderElectionClaimCoordinator) shouldUseLeaderElection(ctx context.Context) bool {
	// Check if there are multiple controller instances (indicating high contention scenario)
	// This could be determined by checking the number of leases or active controllers

	// For now, always try to use leader election if it's configured
	// In practice, this could be made configurable or dynamic based on load
	return true
}

// GetLeadershipStatus returns the current leadership status
func (lec *LeaderElectionClaimCoordinator) GetLeadershipStatus() (isLeader bool, leaderIdentity string) {
	lec.mu.RLock()
	defer lec.mu.RUnlock()

	// In a real implementation, we'd track the current leader identity
	return lec.isLeader, lec.identity
}

// GetClaimQueueStatus returns the current status of the claim queue
func (lec *LeaderElectionClaimCoordinator) GetClaimQueueStatus() (queueLength int, pendingClaims int) {
	lec.mu.RLock()
	defer lec.mu.RUnlock()

	return len(lec.claimQueue), len(lec.pendingClaims)
}

// ReleaseHost releases a host claimed by the given machine
func (lec *LeaderElectionClaimCoordinator) ReleaseHost(ctx context.Context, machine *infrastructurev1beta1.Beskar7Machine) error {
	logger := log.FromContext(ctx).WithValues(
		"machine", machine.Name,
		"namespace", machine.Namespace,
	)

	logger.V(1).Info("Releasing host through leader election coordinator")

	// Delegate to the fallback coordinator for release operations
	// Host release is typically less contentious than claiming
	return lec.fallbackCoordinator.ReleaseHost(ctx, machine)
}
