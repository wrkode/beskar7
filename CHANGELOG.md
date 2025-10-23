# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]
- Future enhancements and planned features

## [v0.3.4-alpha] - 2025-10-23

### Major Features

#### Network Boot Support
- **PXE/iPXE Provisioning**: Full implementation of PXE and iPXE network boot modes
  - Added `SetBootSourcePXE` method to Redfish client interface
  - Implemented BMC configuration for network boot (PXE/UEFI)
  - Added comprehensive examples and documentation
  - Infrastructure prerequisites guide with full setup instructions
- **Provisioning Modes**: All four modes now fully documented and working
  - `PreBakedISO` - Pre-configured ISO boot
  - `RemoteConfig` - Generic ISO with remote configuration
  - `PXE` - Traditional network boot via TFTP
  - `iPXE` - Modern network boot via HTTP

#### Boot Mode Control
- **Boot Mode Field**: Added `bootMode` field to `Beskar7MachineSpec` API
  - Supports `UEFI` (recommended) and `Legacy` boot modes
  - Webhook validation for boot mode values
  - Updated all examples to include boot mode configuration
  - CRD manifests regenerated with new field

### Enhancements

#### Hardware Management
- **Hardware Requirements Matching**: Implemented label-based host selection
  - Added `RequiredLabels` and `PreferredLabels` to `HostRequirements`
  - Implemented label matching logic in `HostClaimCoordinator`
  - CPU/Memory requirements documented (pending HardwareDetails enhancement)
  - Comprehensive logging for host selection decisions

#### Network Discovery
- **Network Interface Traversal**: Enhanced network address detection
  - Implemented NetworkPorts traversal in Redfish client
  - Implemented NetworkDeviceFunctions traversal
  - Added comprehensive logging for network discovery
  - Documented standard Redfish schema limitations

### Bug Fixes

#### API & Validation
- **OS Family Cleanup**: Removed unsupported operating systems from API
  - Removed: `talos`, `ubuntu`, `rhel`, `centos`, `fedora`, `debian`, `opensuse`
  - Retained: `kairos` (recommended), `flatcar`, `LeapMicro`
  - Updated all tests to use supported OS families
  - Regenerated CRD manifests with correct enum values
  - Updated Helm chart CRDs

#### Test Suite
- **Test Coverage**: Fixed and unskipped all previously skipped tests
  - Fixed control plane endpoint detection tests (2 tests)
  - Fixed RemoteConfig validation test
  - Updated test to use LeapMicro instead of Talos
  - Added comprehensive condition checks in PhysicalHost tests
  - All controller tests now passing

### Documentation

#### Complete Documentation Overhaul
- **Comprehensive PXE/iPXE Guide**: 67-page infrastructure prerequisites document
  - Network infrastructure setup (VLANs, routing, topology)
  - DHCP server configuration (ISC DHCP, dnsmasq)
  - TFTP server setup for PXE
  - HTTP server setup for iPXE with nginx configuration
  - OS image hosting and management
  - Firewall rules and port requirements
  - Validation checklist and automated validation script
  - Complete troubleshooting guide

- **Quick Start Guides**:
  - `PXE_QUICK_START.md` - 5-minute testing guide
  - `PXE_TESTING_GUIDE.md` - Comprehensive testing procedures
  - Complete example YAML files for all provisioning modes

- **Examples**:
  - `pxe-simple-test.yaml` - Quick PXE/iPXE testing
  - `pxe-provisioning-example.yaml` - Full PXE cluster deployment
  - `ipxe-provisioning-example.yaml` - Full iPXE cluster deployment

#### Documentation Alignment
- **API Reference**: Updated to reflect only supported features
  - Accurate OS family documentation
  - Boot mode field documented
  - UserDataSecretRef status clarified (pending full integration)
  - Provisioning mode requirements clearly stated

- **README Updates**: Complete rewrite of key sections
  - Added "Supported Features" section
  - All four provisioning modes documented with examples
  - Hardware and OS compatibility tables
  - Quick reference section for common tasks
  - Recent updates section

- **Hardware Compatibility**: Updated matrix
  - Only supported OS families listed
  - Clear note about unsupported traditional distributions
  - OS-specific configuration requirements documented

### Internal Improvements

#### Code Quality
- **Removed TODO Comments**: Cleaned up implementation TODOs
  - Removed generic kubebuilder scaffold comments
  - Removed completed TODO markers
  - Converted remaining TODOs to tracked issues

#### API Cleanup
- **Type Definitions**: Streamlined and validated
  - Removed references to unsupported OS families
  - Added proper validation annotations
  - Updated webhook validation logic

#### Testing Infrastructure
- **Mock Clients**: Enhanced test mocks
  - Added `SetBootSourcePXE` to mock client
  - Updated test assertions for new functionality
  - Improved test coverage across all controllers

### Breaking Changes

 **OS Family Support**: The following OS families have been removed from the API enum:
- `talos` - Removed (open for community contribution)
- `ubuntu`, `rhel`, `centos`, `fedora`, `debian`, `opensuse` - Removed (never fully implemented)

**Migration Path**:
- Update any `Beskar7Machine` resources using removed OS families
- Use `kairos` (recommended), `flatcar`, or `LeapMicro` instead
- See documentation for OS-specific configuration requirements

 **API Changes**: 
- Added optional `bootMode` field to `Beskar7MachineSpec`
  - Defaults to `UEFI` if not specified
  - No action required for existing resources (backward compatible)

### New Files

#### Examples
- `examples/pxe-simple-test.yaml`
- `examples/pxe-provisioning-example.yaml`
- `examples/ipxe-provisioning-example.yaml`
- `examples/pxe-ipxe-prerequisites.md` (1165 lines)
- `examples/PXE_QUICK_START.md`
- `examples/PXE_TESTING_GUIDE.md`

#### Documentation
- Updated all documentation files for accuracy
- Added PXE/iPXE infrastructure guides
- Enhanced troubleshooting documentation

### Technical Details

#### API Changes
```go
// Added to Beskar7MachineSpec
BootMode string `json:"bootMode,omitempty"` // UEFI or Legacy
```

#### New Redfish Client Methods
```go
SetBootSourcePXE(ctx context.Context) error
```

#### Enhanced Coordination
```go
// HostRequirements - Added fields
RequiredLabels  map[string]string
PreferredLabels map[string]string
```

### Statistics

- **Code Changes**: 25+ files modified
- **Documentation**: 11 files updated, 3 comprehensive guides added
- **Examples**: 4 new complete examples
- **Tests**: 3 previously skipped tests fixed and unskipped
- **TODO Items**: 13 resolved
- **API Enum Cleanup**: 7 unsupported OS families removed
- **Lines of Documentation**: 1500+ new lines

### Project Status

**Alpha Release**: This release significantly improves the project's maturity:
- All provisioning modes implemented and documented
- Complete alignment between API, code, and documentation
- Comprehensive testing and examples
- Clear feature support documentation

### Notes

This release represents a comprehensive audit and cleanup of the entire codebase:
- Resolved all TODO comments from code audit
- Fixed all API/documentation misalignments
- Implemented missing critical features (PXE/iPXE, boot mode)
- Enhanced test coverage and quality
- Created comprehensive infrastructure guides

For detailed implementation information, see the examples directory and documentation.

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

[Unreleased]: https://github.com/wrkode/beskar7/compare/v0.3.4-alpha...HEAD
[v0.3.4-alpha]: https://github.com/wrkode/beskar7/compare/v0.2.7...v0.3.4-alpha
[v0.2.7]: https://github.com/wrkode/beskar7/releases/tag/v0.2.7
[v0.2.6]: https://github.com/wrkode/beskar7/releases/tag/v0.2.6

