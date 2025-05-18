# Beskar7 Project Roadmap

This document outlines the current status and future plans for the Beskar7 project. Items are organized by category and priority.

## Completed Items

### Core Functionality
- [x] Basic `PhysicalHost` reconciliation (Redfish connection, status update)
- [x] Basic `Beskar7Machine` reconciliation (Host claiming, status monitoring based on host)
- [x] `Beskar7Machine` deletion/finalizer handling (releasing the `PhysicalHost`)
- [x] BDD Testing setup (`envtest`, Ginkgo/Gomega)
- [x] Basic UserData handling (`Beskar7Machine` spec changes for OS-specific remote config)
- [x] Implement `PhysicalHost` Deprovisioning (Power off, eject media on delete)
- [x] Initial `SetBootParameters` implementation in Redfish client (UEFI target attempt)
- [x] Basic `Beskar7Cluster` reconciliation (handles finalizer and `ControlPlaneEndpointReady` based on spec)
- [x] Refine Status Reporting (CAPI Conditions for Beskar7Machine, PhysicalHost, Beskar7Cluster types and basic `Status.Ready` logic)

## In Progress

### Core Functionality
- [ ] **`SetBootParameters` Full Implementation:** Robustly handle setting boot parameters via Redfish across various BMCs, investigating `UefiTargetBootSourceOverride`, BIOS attributes, and other vendor-specific mechanisms
- [ ] **`Beskar7Cluster` Enhancements:**
  - [ ] Derive `ControlPlaneEndpoint` in `Status` from control plane `Beskar7Machine`s
  - [ ] Add IP address information to `Beskar7MachineStatus`

## Architecture and Design Improvements

### State Management
- [x] Implement state machine pattern for clearer state transitions
- [x] Add more detailed state tracking in the PhysicalHost status
- [x] Implement retry mechanisms with exponential backoff for transient failures

### Error Handling
- [x] Implement more granular error types and error wrapping
- [x] Add error recovery mechanisms for common failure scenarios
  - [x] Implement individual recovery strategies (PowerState, BootSource, Connection, SystemInfo, VirtualMedia)
  - [x] Add composite recovery strategies (Provisioning, Discovery)
  - [x] Add configuration options for recovery behavior
  - [x] Implement metrics and logging for recovery attempts
- [ ] Add comprehensive test coverage for error recovery system
  - [ ] Unit tests for individual recovery strategies
  - [ ] Integration tests for recovery system
  - [ ] Test coverage for error scenarios
  - [ ] Mock testing infrastructure
- [ ] Enhance error recovery system
  - [ ] Add Prometheus metrics export for recovery operations
  - [ ] Implement dynamic recovery strategy registration
  - [ ] Add recovery state persistence
  - [ ] Support strategy prioritization
  - [ ] Add recovery continuation after controller restart
- [ ] Improve error reporting and monitoring
  - [ ] Add structured error logging
  - [ ] Implement error aggregation
  - [ ] Create error dashboards
  - [ ] Set up error alerting
- [ ] Add error prevention mechanisms
  - [ ] Implement pre-operation validation
  - [ ] Add health checks
  - [ ] Create preventive maintenance routines
  - [ ] Add early warning system
- [ ] Improve error messages with more context

### Configuration
- [x] Move hardcoded values to configurable parameters
- [x] Add support for environment-specific configurations
- [ ] Implement feature flags for experimental features

## Testing Improvements

### Test Coverage
- [ ] Add more integration tests
- [ ] Implement chaos testing scenarios
- [ ] Add performance benchmarks
- [ ] Add more edge case tests

### Test Infrastructure
- [ ] Add test fixtures for common scenarios
- [ ] Implement test helpers for common operations
- [ ] Add test documentation

## Documentation Improvements

### Code Documentation
- [ ] Add more detailed godoc comments
- [ ] Document error handling strategies
- [ ] Add architecture diagrams

### User Documentation
- [ ] Add troubleshooting guides
- [ ] Improve API documentation
- [ ] Add deployment guides for different environments

## Security Improvements

### Authentication
- [ ] Implement more secure credential handling
- [ ] Add support for different authentication methods
- [ ] Add audit logging

### Authorization
- [ ] Implement more granular RBAC rules
- [ ] Add support for custom authorization policies
- [ ] Add security context documentation

## Performance Improvements

### Resource Management
- [ ] Implement resource quotas
- [ ] Add resource usage monitoring
- [ ] Optimize reconciliation loops

### Caching
- [ ] Implement caching for frequently accessed resources
- [ ] Add cache invalidation strategies
- [ ] Optimize API calls

## System Compatibility

### Path Dependencies
- [ ] Replace hardcoded paths with configurable ones
- [ ] Use path.Join() for all path operations
- [ ] Add OS-specific path handling

### Environment Dependencies
- [ ] Add environment detection and adaptation
- [ ] Document environment requirements
- [ ] Add environment-specific test cases

## New Features

### Monitoring and Observability
- [ ] Add metrics collection
- [ ] Implement health checks
- [ ] Add tracing support

### Management Features
- [ ] Add support for bulk operations
- [ ] Implement backup and restore
- [ ] Add migration tools

## Code Quality Improvements

### Code Organization
- [ ] Split large functions into smaller ones
- [ ] Add more interfaces for better testing
- [ ] Improve code reusability

### Dependency Management
- [ ] Update dependencies regularly
- [ ] Add dependency scanning
- [ ] Document dependency requirements

## CI/CD Improvements

### Build Process
- [ ] Add multi-architecture support
- [ ] Implement automated versioning
- [ ] Add build caching

### Deployment
- [ ] Add deployment automation
- [ ] Implement rollback mechanisms
- [ ] Add deployment verification

## Community and Collaboration

### Contributor Experience
- [ ] Improve contribution guidelines
- [ ] Add development environment setup guide
- [ ] Implement PR templates

### Community Engagement
- [ ] Add community documentation
- [ ] Implement feature request process
- [ ] Add user feedback mechanisms 