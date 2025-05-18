package config

import (
	"os"
	"strconv"
	"time"
)

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv(config *Config) {
	// Redfish configuration
	if scheme := os.Getenv("BESKAR7_REDFISH_SCHEME"); scheme != "" {
		config.Redfish.DefaultScheme = scheme
	}
	if port := os.Getenv("BESKAR7_REDFISH_PORT"); port != "" {
		config.Redfish.DefaultPort = port
	}
	if timeout := os.Getenv("BESKAR7_REDFISH_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			config.Redfish.DefaultTimeout = duration
		}
	}

	// Controller configuration
	if interval := os.Getenv("BESKAR7_CONTROLLER_REQUEUE_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval); err == nil {
			config.Controller.RequeueInterval = duration
		}
	}
	if afterError := os.Getenv("BESKAR7_CONTROLLER_REQUEUE_AFTER_ERROR"); afterError != "" {
		if duration, err := time.ParseDuration(afterError); err == nil {
			config.Controller.RequeueAfterError = duration
		}
	}
	if afterNoHost := os.Getenv("BESKAR7_CONTROLLER_REQUEUE_AFTER_NO_HOST"); afterNoHost != "" {
		if duration, err := time.ParseDuration(afterNoHost); err == nil {
			config.Controller.RequeueAfterNoHost = duration
		}
	}

	// Retry configuration
	if initialInterval := os.Getenv("BESKAR7_RETRY_INITIAL_INTERVAL"); initialInterval != "" {
		if duration, err := time.ParseDuration(initialInterval); err == nil {
			config.Retry.InitialInterval = duration
		}
	}
	if maxInterval := os.Getenv("BESKAR7_RETRY_MAX_INTERVAL"); maxInterval != "" {
		if duration, err := time.ParseDuration(maxInterval); err == nil {
			config.Retry.MaxInterval = duration
		}
	}
	if multiplier := os.Getenv("BESKAR7_RETRY_MULTIPLIER"); multiplier != "" {
		if value, err := parseFloat(multiplier); err == nil {
			config.Retry.Multiplier = value
		}
	}
	if maxAttempts := os.Getenv("BESKAR7_RETRY_MAX_ATTEMPTS"); maxAttempts != "" {
		if value, err := parseInt(maxAttempts); err == nil {
			config.Retry.MaxAttempts = value
		}
	}
	if maxElapsedTime := os.Getenv("BESKAR7_RETRY_MAX_ELAPSED_TIME"); maxElapsedTime != "" {
		if duration, err := time.ParseDuration(maxElapsedTime); err == nil {
			config.Retry.MaxElapsedTime = duration
		}
	}

	// Boot configuration
	if bootloaderPath := os.Getenv("BESKAR7_BOOT_DEFAULT_EFI_PATH"); bootloaderPath != "" {
		config.Boot.DefaultEFIBootloaderPath = bootloaderPath
	}
	if overrideEnabled := os.Getenv("BESKAR7_BOOT_DEFAULT_OVERRIDE_ENABLED"); overrideEnabled != "" {
		config.Boot.DefaultBootSourceOverrideEnabled = overrideEnabled
	}
	if overrideTarget := os.Getenv("BESKAR7_BOOT_DEFAULT_OVERRIDE_TARGET"); overrideTarget != "" {
		config.Boot.DefaultBootSourceOverrideTarget = overrideTarget
	}
}

// parseFloat parses a string into a float64
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// parseInt parses a string into an int
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
