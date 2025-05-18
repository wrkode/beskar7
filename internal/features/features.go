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

package features

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Feature represents a feature flag
type Feature string

// FeatureState represents the state of a feature flag
type FeatureState struct {
	Enabled bool
	Default bool
}

var (
	// FeatureMap stores all registered features and their states
	featureMap = make(map[Feature]FeatureState)
	// featureMutex protects concurrent access to featureMap
	featureMutex sync.RWMutex
)

// RegisterFeature registers a new feature flag with a default state
func RegisterFeature(name Feature, defaultState bool) {
	featureMutex.Lock()
	defer featureMutex.Unlock()

	if _, exists := featureMap[name]; exists {
		panic(fmt.Sprintf("feature %q already registered", name))
	}

	featureMap[name] = FeatureState{
		Enabled: defaultState,
		Default: defaultState,
	}
}

// IsEnabled checks if a feature is enabled
func IsEnabled(name Feature) bool {
	featureMutex.RLock()
	defer featureMutex.RUnlock()

	state, exists := featureMap[name]
	if !exists {
		return false
	}
	return state.Enabled
}

// SetEnabled sets the state of a feature flag
func SetEnabled(name Feature, enabled bool) error {
	featureMutex.Lock()
	defer featureMutex.Unlock()

	state, exists := featureMap[name]
	if !exists {
		return fmt.Errorf("feature %q not registered", name)
	}

	state.Enabled = enabled
	featureMap[name] = state
	return nil
}

// ResetToDefault resets a feature flag to its default state
func ResetToDefault(name Feature) error {
	featureMutex.Lock()
	defer featureMutex.Unlock()

	state, exists := featureMap[name]
	if !exists {
		return fmt.Errorf("feature %q not registered", name)
	}

	state.Enabled = state.Default
	featureMap[name] = state
	return nil
}

// ResetAllToDefault resets all feature flags to their default states
func ResetAllToDefault() {
	featureMutex.Lock()
	defer featureMutex.Unlock()

	for name, state := range featureMap {
		state.Enabled = state.Default
		featureMap[name] = state
	}
}

// LoadFromEnv loads feature flag states from environment variables
// Environment variables should be in the format BESKAR7_FEATURE_<FEATURE_NAME>
// Values can be "true", "false", "1", "0"
func LoadFromEnv() {
	featureMutex.Lock()
	defer featureMutex.Unlock()

	for name, state := range featureMap {
		envName := fmt.Sprintf("BESKAR7_FEATURE_%s", strings.ToUpper(string(name)))
		if value, exists := os.LookupEnv(envName); exists {
			enabled := value == "true" || value == "1"
			state.Enabled = enabled
			featureMap[name] = state
		}
	}
}

// ListFeatures returns a map of all registered features and their states
func ListFeatures() map[Feature]FeatureState {
	featureMutex.RLock()
	defer featureMutex.RUnlock()

	result := make(map[Feature]FeatureState)
	for name, state := range featureMap {
		result[name] = state
	}
	return result
}
