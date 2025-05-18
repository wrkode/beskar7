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

	// Environment configuration
	Environment *EnvironmentConfig

	// Feature flags
	Features map[string]bool
}

// RedfishConfig holds Redfish-specific configuration
type RedfishConfig struct {
	// DefaultScheme is the default URL scheme for Redfish endpoints
	DefaultScheme string
	// DefaultPort is the default port for Redfish endpoints
	DefaultPort string
	// DefaultTimeout is the default timeout for Redfish operations
	DefaultTimeout time.Duration

	// InsecureSkipVerify is a flag to skip SSL verification
	InsecureSkipVerify bool

	// Timeout is the timeout for Redfish operations
	Timeout time.Duration
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

	// DefaultBootSource is the default boot source
	DefaultBootSource string
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Redfish: RedfishConfig{
			DefaultScheme:      "https",
			DefaultPort:        "443",
			DefaultTimeout:     30 * time.Second,
			InsecureSkipVerify: false,
			Timeout:            30 * time.Second,
		},
		Controller: ControllerConfig{
			RequeueInterval:    15 * time.Second,
			RequeueAfterError:  5 * time.Minute,
			RequeueAfterNoHost: 1 * time.Minute,
		},
		Retry: RetryConfig{
			InitialInterval: 2 * time.Second,
			MaxInterval:     1 * time.Minute,
			Multiplier:      2.0,
			MaxAttempts:     3,
			MaxElapsedTime:  5 * time.Minute,
		},
		Boot: BootConfig{
			DefaultEFIBootloaderPath:         "\\EFI\\BOOT\\BOOTX64.EFI",
			DefaultBootSourceOverrideEnabled: "Once",
			DefaultBootSourceOverrideTarget:  "UefiTarget",
			DefaultBootSource:                "UEFI",
		},
		Environment: NewEnvironmentConfig(),
		Features:    make(map[string]bool),
	}
}

// LoadConfig loads configuration from environment variables and environment-specific settings
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	// Load environment-specific configuration
	if err := config.Environment.LoadEnvironmentConfig(); err != nil {
		return nil, err
	}

	// Load from environment variables
	LoadFromEnv(config)

	// Apply environment-specific overrides
	applyEnvironmentOverrides(config)

	return config, nil
}

// applyEnvironmentOverrides applies environment-specific configuration overrides
func applyEnvironmentOverrides(config *Config) {
	// Helper function to get environment override
	getOverride := func(key string) (string, bool) {
		return config.Environment.GetOverride(key)
	}

	// Apply Redfish overrides
	if value, ok := getOverride("REDFISH_SCHEME"); ok {
		config.Redfish.DefaultScheme = value
	}
	if value, ok := getOverride("REDFISH_PORT"); ok {
		config.Redfish.DefaultPort = value
	}
	if value, ok := getOverride("REDFISH_TIMEOUT"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Redfish.DefaultTimeout = duration
		}
	}

	// Apply Controller overrides
	if value, ok := getOverride("CONTROLLER_REQUEUE_INTERVAL"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Controller.RequeueInterval = duration
		}
	}
	if value, ok := getOverride("CONTROLLER_REQUEUE_AFTER_ERROR"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Controller.RequeueAfterError = duration
		}
	}
	if value, ok := getOverride("CONTROLLER_REQUEUE_AFTER_NO_HOST"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Controller.RequeueAfterNoHost = duration
		}
	}

	// Apply Retry overrides
	if value, ok := getOverride("RETRY_INITIAL_INTERVAL"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Retry.InitialInterval = duration
		}
	}
	if value, ok := getOverride("RETRY_MAX_INTERVAL"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Retry.MaxInterval = duration
		}
	}
	if value, ok := getOverride("RETRY_MULTIPLIER"); ok {
		if multiplier, err := parseFloat(value); err == nil {
			config.Retry.Multiplier = multiplier
		}
	}
	if value, ok := getOverride("RETRY_MAX_ATTEMPTS"); ok {
		if attempts, err := parseInt(value); err == nil {
			config.Retry.MaxAttempts = attempts
		}
	}
	if value, ok := getOverride("RETRY_MAX_ELAPSED_TIME"); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			config.Retry.MaxElapsedTime = duration
		}
	}

	// Apply Boot overrides
	if value, ok := getOverride("BOOT_DEFAULT_EFI_PATH"); ok {
		config.Boot.DefaultEFIBootloaderPath = value
	}
	if value, ok := getOverride("BOOT_DEFAULT_OVERRIDE_ENABLED"); ok {
		config.Boot.DefaultBootSourceOverrideEnabled = value
	}
	if value, ok := getOverride("BOOT_DEFAULT_OVERRIDE_TARGET"); ok {
		config.Boot.DefaultBootSourceOverrideTarget = value
	}

	// Apply feature flags
	if value, ok := getOverride("FEATURE_FLAG"); ok {
		config.Features[value] = true
	}
}
