package config

import (
	"time"
)

// Config holds all configurable parameters for the Beskar7 controller
type Config struct {
	// Redfish configuration
	Redfish RedfishConfig

	// Controller configuration
	Controller ControllerConfig

	// Retry configuration
	Retry RetryConfig

	// Boot configuration
	Boot BootConfig
}

// RedfishConfig holds Redfish-specific configuration
type RedfishConfig struct {
	// DefaultScheme is the default URL scheme for Redfish endpoints
	DefaultScheme string
	// DefaultPort is the default port for Redfish endpoints
	DefaultPort string
	// DefaultTimeout is the default timeout for Redfish operations
	DefaultTimeout time.Duration
}

// ControllerConfig holds controller-specific configuration
type ControllerConfig struct {
	// RequeueInterval is the default interval for requeuing reconciliation
	RequeueInterval time.Duration
	// RequeueAfterError is the interval for requeuing after an error
	RequeueAfterError time.Duration
	// RequeueAfterNoHost is the interval for requeuing when no host is found
	RequeueAfterNoHost time.Duration
}

// RetryConfig holds retry-specific configuration
type RetryConfig struct {
	// InitialInterval is the initial retry interval
	InitialInterval time.Duration
	// MaxInterval is the maximum retry interval
	MaxInterval time.Duration
	// Multiplier is the factor to multiply the interval by for each retry
	Multiplier float64
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int
	// MaxElapsedTime is the maximum total time to retry
	MaxElapsedTime time.Duration
}

// BootConfig holds boot-specific configuration
type BootConfig struct {
	// DefaultEFIBootloaderPath is the default path to the EFI bootloader
	DefaultEFIBootloaderPath string
	// DefaultBootSourceOverrideEnabled is the default boot source override setting
	DefaultBootSourceOverrideEnabled string
	// DefaultBootSourceOverrideTarget is the default boot source override target
	DefaultBootSourceOverrideTarget string
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Redfish: RedfishConfig{
			DefaultScheme:  "https",
			DefaultPort:    "443",
			DefaultTimeout: 30 * time.Second,
		},
		Controller: ControllerConfig{
			RequeueInterval:    15 * time.Second,
			RequeueAfterError:  5 * time.Minute,
			RequeueAfterNoHost: 1 * time.Minute,
		},
		Retry: RetryConfig{
			InitialInterval: 1 * time.Second,
			MaxInterval:     5 * time.Minute,
			Multiplier:      2.0,
			MaxAttempts:     5,
			MaxElapsedTime:  15 * time.Minute,
		},
		Boot: BootConfig{
			DefaultEFIBootloaderPath:         "\\EFI\\BOOT\\BOOTX64.EFI",
			DefaultBootSourceOverrideEnabled: "Once",
			DefaultBootSourceOverrideTarget:  "UefiTarget",
		},
	}
}
