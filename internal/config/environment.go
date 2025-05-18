package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment represents a deployment environment (e.g., dev, staging, prod)
type Environment string

const (
	// EnvironmentDevelopment represents the development environment
	EnvironmentDevelopment Environment = "development"
	// EnvironmentStaging represents the staging environment
	EnvironmentStaging Environment = "staging"
	// EnvironmentProduction represents the production environment
	EnvironmentProduction Environment = "production"
)

// EnvironmentConfig holds environment-specific configuration
type EnvironmentConfig struct {
	// Environment is the current deployment environment
	Environment Environment
	// ConfigPath is the path to environment-specific configuration files
	ConfigPath string
	// Overrides contains environment-specific configuration overrides
	Overrides map[string]string
}

// NewEnvironmentConfig creates a new EnvironmentConfig instance
func NewEnvironmentConfig() *EnvironmentConfig {
	env := getEnvironment()
	configPath := getConfigPath(env)

	return &EnvironmentConfig{
		Environment: env,
		ConfigPath:  configPath,
		Overrides:   make(map[string]string),
	}
}

// LoadEnvironmentConfig loads environment-specific configuration
func (ec *EnvironmentConfig) LoadEnvironmentConfig() error {
	// Load from environment variables first
	ec.loadFromEnv()

	// Then load from config files if they exist
	if err := ec.loadFromFiles(); err != nil {
		return fmt.Errorf("failed to load environment config: %w", err)
	}

	return nil
}

// GetOverride returns the environment-specific override for a configuration key
func (ec *EnvironmentConfig) GetOverride(key string) (string, bool) {
	value, exists := ec.Overrides[key]
	return value, exists
}

// getEnvironment determines the current environment
func getEnvironment() Environment {
	env := strings.ToLower(os.Getenv("BESKAR7_ENVIRONMENT"))
	switch Environment(env) {
	case EnvironmentStaging:
		return EnvironmentStaging
	case EnvironmentProduction:
		return EnvironmentProduction
	default:
		return EnvironmentDevelopment
	}
}

// getConfigPath returns the path to environment-specific configuration files
func getConfigPath(env Environment) string {
	// First check if a custom path is set
	if customPath := os.Getenv("BESKAR7_CONFIG_PATH"); customPath != "" {
		return filepath.Join(customPath, string(env))
	}

	// Default paths
	switch env {
	case EnvironmentDevelopment:
		return "config/environments/development"
	case EnvironmentStaging:
		return "config/environments/staging"
	case EnvironmentProduction:
		return "config/environments/production"
	default:
		return "config/environments/development"
	}
}

// loadFromEnv loads environment-specific configuration from environment variables
func (ec *EnvironmentConfig) loadFromEnv() {
	// Look for environment-specific variables with the pattern BESKAR7_<ENV>_<KEY>
	envPrefix := fmt.Sprintf("BESKAR7_%s_", strings.ToUpper(string(ec.Environment)))
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := pair[0], pair[1]
		if strings.HasPrefix(key, envPrefix) {
			// Remove the environment prefix to get the actual config key
			configKey := strings.TrimPrefix(key, envPrefix)
			ec.Overrides[configKey] = value
		}
	}
}

// loadFromFiles loads environment-specific configuration from files
func (ec *EnvironmentConfig) loadFromFiles() error {
	// Check if the config directory exists
	if _, err := os.Stat(ec.ConfigPath); os.IsNotExist(err) {
		return nil // No config files to load
	}

	// Load all .env files in the config directory
	return filepath.Walk(ec.ConfigPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".env") {
			return nil
		}

		// Read and parse the .env file
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read config file %s: %w", path, err)
		}

		// Parse the file content
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			pair := strings.SplitN(line, "=", 2)
			if len(pair) != 2 {
				continue
			}
			key, value := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
			ec.Overrides[key] = value
		}

		return nil
	})
}
