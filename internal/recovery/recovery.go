package recovery

import (
	"context"
	"fmt"
	"time"

	"github.com/stmcginnis/gofish/redfish"
	"github.com/wrkode/beskar7/internal/errors"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	"go.uber.org/zap"
)

// RecoveryStrategy defines the interface for recovery strategies
type RecoveryStrategy interface {
	// AttemptRecovery attempts to recover from the error
	AttemptRecovery(ctx context.Context, client internalredfish.Client) error
	// IsApplicable checks if this strategy is applicable for the given error
	IsApplicable(err error) bool
}

// PowerStateRecovery handles recovery from power state related errors
type PowerStateRecovery struct {
	TargetState redfish.PowerState
	MaxAttempts int
}

// AttemptRecovery implements RecoveryStrategy
func (r *PowerStateRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	for i := 0; i < r.MaxAttempts; i++ {
		// Get current power state
		currentState, err := client.GetPowerState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get power state during recovery: %w", err)
		}

		// If already in target state, no need to change
		if currentState == r.TargetState {
			return nil
		}

		// Try to set power state
		if err := client.SetPowerState(ctx, r.TargetState); err != nil {
			// If it's the last attempt, return the error
			if i == r.MaxAttempts-1 {
				return fmt.Errorf("failed to set power state after %d attempts: %w", r.MaxAttempts, err)
			}
			// Wait before retrying
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}

		// Verify the state change
		time.Sleep(2 * time.Second)
		newState, err := client.GetPowerState(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify power state after recovery: %w", err)
		}

		if newState == r.TargetState {
			return nil
		}
	}

	return fmt.Errorf("failed to recover power state after %d attempts", r.MaxAttempts)
}

// IsApplicable implements RecoveryStrategy
func (r *PowerStateRecovery) IsApplicable(err error) bool {
	_, ok := err.(*errors.PowerStateError)
	return ok
}

// BootSourceRecovery handles recovery from boot source related errors
type BootSourceRecovery struct {
	BootSource  string
	MaxAttempts int
}

// AttemptRecovery implements RecoveryStrategy
func (r *BootSourceRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	for i := 0; i < r.MaxAttempts; i++ {
		// First try to eject any existing virtual media
		if err := client.EjectVirtualMedia(ctx); err != nil {
			// Log but continue, as this might not be critical
			fmt.Printf("Warning: failed to eject virtual media: %v\n", err)
		}

		// Wait a bit before setting new boot source
		time.Sleep(2 * time.Second)

		// Try to set the boot source
		if err := client.SetBootSourceISO(ctx, r.BootSource); err != nil {
			// If it's the last attempt, return the error
			if i == r.MaxAttempts-1 {
				return fmt.Errorf("failed to set boot source after %d attempts: %w", r.MaxAttempts, err)
			}
			// Wait before retrying
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to recover boot source after %d attempts", r.MaxAttempts)
}

// IsApplicable implements RecoveryStrategy
func (r *BootSourceRecovery) IsApplicable(err error) bool {
	_, ok := err.(*errors.BootSourceError)
	return ok
}

// ConnectionRecovery handles recovery from connection related errors
type ConnectionRecovery struct {
	MaxAttempts int
}

// AttemptRecovery implements RecoveryStrategy
func (r *ConnectionRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	for i := 0; i < r.MaxAttempts; i++ {
		// Try to get system info as a connection test
		_, err := client.GetSystemInfo(ctx)
		if err == nil {
			return nil
		}

		// If it's the last attempt, return the error
		if i == r.MaxAttempts-1 {
			return fmt.Errorf("failed to establish connection after %d attempts: %w", r.MaxAttempts, err)
		}

		// Wait with exponential backoff
		time.Sleep(time.Second * time.Duration(1<<uint(i)))
	}

	return fmt.Errorf("failed to recover connection after %d attempts", r.MaxAttempts)
}

// IsApplicable implements RecoveryStrategy
func (r *ConnectionRecovery) IsApplicable(err error) bool {
	_, ok := err.(*errors.RedfishConnectionError)
	return ok
}

// SystemInfoRecovery handles recovery from system info related errors
type SystemInfoRecovery struct {
	MaxAttempts int
}

// AttemptRecovery implements RecoveryStrategy
func (r *SystemInfoRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	for i := 0; i < r.MaxAttempts; i++ {
		// Try to get system info
		_, err := client.GetSystemInfo(ctx)
		if err == nil {
			return nil
		}

		// If it's the last attempt, return the error
		if i == r.MaxAttempts-1 {
			return fmt.Errorf("failed to get system info after %d attempts: %w", r.MaxAttempts, err)
		}

		// Wait with exponential backoff
		time.Sleep(time.Second * time.Duration(1<<uint(i)))
	}

	return fmt.Errorf("failed to recover system info after %d attempts", r.MaxAttempts)
}

// IsApplicable implements RecoveryStrategy
func (r *SystemInfoRecovery) IsApplicable(err error) bool {
	_, ok := err.(*errors.DiscoveryError)
	return ok
}

// VirtualMediaRecovery handles recovery from virtual media related errors
type VirtualMediaRecovery struct {
	MaxAttempts int
}

// AttemptRecovery implements RecoveryStrategy
func (r *VirtualMediaRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	for i := 0; i < r.MaxAttempts; i++ {
		// Try to eject virtual media
		if err := client.EjectVirtualMedia(ctx); err != nil {
			// If it's the last attempt, return the error
			if i == r.MaxAttempts-1 {
				return fmt.Errorf("failed to eject virtual media after %d attempts: %w", r.MaxAttempts, err)
			}
			// Wait before retrying
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}

		// Wait a bit to ensure the operation completed
		time.Sleep(2 * time.Second)

		// Try to get system info to verify connection is still good
		_, err := client.GetSystemInfo(ctx)
		if err != nil {
			return fmt.Errorf("connection lost after virtual media operation: %w", err)
		}

		return nil
	}

	return fmt.Errorf("failed to recover virtual media after %d attempts", r.MaxAttempts)
}

// IsApplicable implements RecoveryStrategy
func (r *VirtualMediaRecovery) IsApplicable(err error) bool {
	// Check if the error is related to virtual media operations
	if bootErr, ok := err.(*errors.BootSourceError); ok {
		return bootErr.BootSource != ""
	}
	return false
}

// RecoveryMetrics tracks recovery attempt metrics
type RecoveryMetrics struct {
	TotalAttempts        int64
	SuccessfulRecoveries int64
	FailedRecoveries     int64
	RecoveryDuration     time.Duration
}

// RecoveryConfig holds configuration options for recovery behavior
type RecoveryConfig struct {
	// MaxAttempts is the maximum number of recovery attempts for any strategy
	MaxAttempts int
	// InitialBackoff is the initial backoff duration between retries
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration between retries
	MaxBackoff time.Duration
	// BackoffMultiplier is the factor to multiply the backoff duration by after each attempt
	BackoffMultiplier float64
	// EnableMetrics enables collection of recovery metrics
	EnableMetrics bool
	// EnableLogging enables detailed logging of recovery attempts
	EnableLogging bool
}

// DefaultRecoveryConfig returns the default recovery configuration
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		MaxAttempts:       3,
		InitialBackoff:    time.Second,
		MaxBackoff:        5 * time.Minute,
		BackoffMultiplier: 2.0,
		EnableMetrics:     true,
		EnableLogging:     true,
	}
}

// RecoveryManager manages multiple recovery strategies
type RecoveryManager struct {
	strategies []RecoveryStrategy
	logger     *zap.Logger
	metrics    *RecoveryMetrics
	config     *RecoveryConfig
}

// CompositeRecovery combines multiple recovery strategies
type CompositeRecovery struct {
	Strategies  []RecoveryStrategy
	MaxAttempts int
}

// NewCompositeRecovery creates a new CompositeRecovery
func NewCompositeRecovery(strategies []RecoveryStrategy, maxAttempts int) *CompositeRecovery {
	return &CompositeRecovery{
		Strategies:  strategies,
		MaxAttempts: maxAttempts,
	}
}

// AttemptRecovery implements RecoveryStrategy
func (r *CompositeRecovery) AttemptRecovery(ctx context.Context, client internalredfish.Client) error {
	var lastErr error
	for i := 0; i < r.MaxAttempts; i++ {
		// Try each strategy in sequence
		for _, strategy := range r.Strategies {
			if err := strategy.AttemptRecovery(ctx, client); err != nil {
				lastErr = err
				continue
			}
			// If any strategy succeeds, return success
			return nil
		}

		// If all strategies failed and this is the last attempt, return the last error
		if i == r.MaxAttempts-1 {
			return fmt.Errorf("all recovery strategies failed after %d attempts: %w", r.MaxAttempts, lastErr)
		}

		// Wait with exponential backoff before retrying
		time.Sleep(time.Second * time.Duration(1<<uint(i)))
	}

	return fmt.Errorf("failed to recover after %d attempts: %w", r.MaxAttempts, lastErr)
}

// IsApplicable implements RecoveryStrategy
func (r *CompositeRecovery) IsApplicable(err error) bool {
	// Check if any of the strategies are applicable
	for _, strategy := range r.Strategies {
		if strategy.IsApplicable(err) {
			return true
		}
	}
	return false
}

// NewRecoveryManager creates a new RecoveryManager
func NewRecoveryManager(logger *zap.Logger, config *RecoveryConfig) *RecoveryManager {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	// Create individual strategies with configured max attempts
	powerStateRecovery := &PowerStateRecovery{MaxAttempts: config.MaxAttempts}
	bootSourceRecovery := &BootSourceRecovery{MaxAttempts: config.MaxAttempts}
	connectionRecovery := &ConnectionRecovery{MaxAttempts: config.MaxAttempts}
	systemInfoRecovery := &SystemInfoRecovery{MaxAttempts: config.MaxAttempts}
	virtualMediaRecovery := &VirtualMediaRecovery{MaxAttempts: config.MaxAttempts}

	// Create composite strategies
	provisioningRecovery := NewCompositeRecovery([]RecoveryStrategy{
		powerStateRecovery,
		bootSourceRecovery,
		virtualMediaRecovery,
	}, config.MaxAttempts)

	discoveryRecovery := NewCompositeRecovery([]RecoveryStrategy{
		connectionRecovery,
		systemInfoRecovery,
		powerStateRecovery,
	}, config.MaxAttempts)

	return &RecoveryManager{
		strategies: []RecoveryStrategy{
			powerStateRecovery,
			bootSourceRecovery,
			connectionRecovery,
			systemInfoRecovery,
			virtualMediaRecovery,
			provisioningRecovery,
			discoveryRecovery,
		},
		logger:  logger,
		metrics: &RecoveryMetrics{},
		config:  config,
	}
}

// AddStrategy adds a new recovery strategy
func (m *RecoveryManager) AddStrategy(strategy RecoveryStrategy) {
	m.strategies = append(m.strategies, strategy)
}

// AttemptRecovery attempts to recover from an error using applicable strategies
func (m *RecoveryManager) AttemptRecovery(ctx context.Context, client internalredfish.Client, err error) error {
	startTime := time.Now()
	if m.config.EnableMetrics {
		m.metrics.TotalAttempts++
	}

	if m.config.EnableLogging {
		logger := m.logger.With(
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.Error(err),
		)
		logger.Info("Attempting error recovery")
	}

	backoff := m.config.InitialBackoff
	for i := 0; i < m.config.MaxAttempts; i++ {
		for _, strategy := range m.strategies {
			if strategy.IsApplicable(err) {
				if m.config.EnableLogging {
					m.logger.Info("Found applicable recovery strategy",
						zap.String("strategy", fmt.Sprintf("%T", strategy)))
				}

				if recoveryErr := strategy.AttemptRecovery(ctx, client); recoveryErr != nil {
					if m.config.EnableMetrics {
						m.metrics.FailedRecoveries++
					}
					if m.config.EnableLogging {
						m.logger.Error("Recovery attempt failed",
							zap.Error(recoveryErr),
							zap.Duration("duration", time.Since(startTime)))
					}
					return fmt.Errorf("recovery failed: %w", recoveryErr)
				}

				if m.config.EnableMetrics {
					m.metrics.SuccessfulRecoveries++
					m.metrics.RecoveryDuration += time.Since(startTime)
				}
				if m.config.EnableLogging {
					m.logger.Info("Recovery successful",
						zap.Duration("duration", time.Since(startTime)))
				}
				return nil
			}
		}

		// If this is the last attempt, return error
		if i == m.config.MaxAttempts-1 {
			if m.config.EnableMetrics {
				m.metrics.FailedRecoveries++
			}
			if m.config.EnableLogging {
				m.logger.Error("No applicable recovery strategy found",
					zap.Duration("duration", time.Since(startTime)))
			}
			return fmt.Errorf("no applicable recovery strategy found for error: %w", err)
		}

		// Wait with configured backoff
		time.Sleep(backoff)
		backoff = time.Duration(float64(backoff) * m.config.BackoffMultiplier)
		if backoff > m.config.MaxBackoff {
			backoff = m.config.MaxBackoff
		}
	}

	return fmt.Errorf("failed to recover after %d attempts: %w", m.config.MaxAttempts, err)
}

// GetMetrics returns the current recovery metrics
func (m *RecoveryManager) GetMetrics() *RecoveryMetrics {
	return m.metrics
}
