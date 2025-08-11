# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]
- Throttle BMC operations via `ProvisioningQueue` permits in `PhysicalHostReconciler`.
- Vendor-specific BIOS scheduling: Dell iDRAC ApplyTime = OnReset after BIOS updates.
- Boot Options fallback: select boot via UEFI BootNext when UEFI Target override fails.
- Clarified Supermicro vendor documentation to reflect implemented fallbacks.
- Added explicit PreBakedISO guidance for supported OS families.
- Introduced `NewClientWithHTTPClient` shim for emulation tests.

## [v0.2.7] - 2025-08-11
### ✨ Features
- **Security Scanning**: Re-enabled Trivy security scanning for public repository with SARIF upload to GitHub Security tab
- **Enhanced CI/CD**: Improved E2E validation with local image building and comprehensive debugging
- **Webhook Integration**: Complete PhysicalHost webhook validation and mutation support

### 🐛 Bug Fixes
- **Critical**: Fixed PhysicalHost finalizer removal bug that caused indefinite deletion hanging
- **CI**: Fixed container image availability in E2E tests by building locally and setting `imagePullPolicy: Never`
- **CI**: Corrected deployment name mismatch in E2E validation (`beskar7-controller-manager` → `controller-manager`)
- **CI**: Fixed kind cluster name mismatch in image loading (`kind` → `beskar7-test`)
- **Linting**: Resolved variable shadowing in `internal/redfish/gofish_client.go`
- **Linting**: Fixed `gosimple` S1021 error in controller tests
- **Manifests**: Fixed `${VERSION}` placeholder substitution in Kubernetes manifests

### 🔧 Improvements
- **Code Quality**: Introduced constants for hardcoded strings (security levels, API versions, URL schemes)
- **Error Handling**: Enhanced webhook error diagnostics and CI failure debugging
- **Testing**: Added comprehensive E2E test timeout protection and cleanup validation
- **Dependencies**: Updated CI job dependencies to ensure proper build order
- **Documentation**: Improved inline code documentation and error messages

### 🛠️ Infrastructure
- **CI Pipeline**: Complete overhaul of GitHub Actions workflow with proper job dependencies
- **Container Build**: Optimized Docker build process with proper caching and multi-platform support
- **Manifest Generation**: Automated version substitution in release manifests
- **Quality Gates**: Comprehensive linting, testing, and security scanning integration

### 📦 Dependencies
- **golangci-lint**: Improved compatibility with v1.64.8 in CI environment
- **cert-manager**: Enhanced certificate management in E2E validation
- **kind**: Better integration with local Kubernetes testing

## [v0.2.6] - 2025-08-10
- Initial public manifests bundle.
- CI: lint, tests, container build, CRD generation, Kind sanity checks.
- Core controllers and CRDs for `PhysicalHost`, `Beskar7Machine`, `Beskar7Cluster`.

[Unreleased]: https://github.com/wrkode/beskar7/compare/v0.2.7...HEAD
[v0.2.7]: https://github.com/wrkode/beskar7/releases/tag/v0.2.7
[v0.2.6]: https://github.com/wrkode/beskar7/releases/tag/v0.2.6

