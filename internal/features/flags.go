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

// Feature flags for experimental features
const (
	// EnableAdvancedRecovery enables advanced error recovery mechanisms
	EnableAdvancedRecovery Feature = "EnableAdvancedRecovery"

	// EnableMetricsExport enables detailed metrics export
	EnableMetricsExport Feature = "EnableMetricsExport"

	// EnableCustomBootSource enables custom boot source configuration
	EnableCustomBootSource Feature = "EnableCustomBootSource"

	// EnableVendorSpecificFeatures enables vendor-specific features
	EnableVendorSpecificFeatures Feature = "EnableVendorSpecificFeatures"
)

// RegisterDefaultFeatures registers all feature flags with their default states
func RegisterDefaultFeatures() {
	// Register features with their default states
	RegisterFeature(EnableAdvancedRecovery, false)
	RegisterFeature(EnableMetricsExport, false)
	RegisterFeature(EnableCustomBootSource, false)
	RegisterFeature(EnableVendorSpecificFeatures, false)
}
