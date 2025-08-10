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

## [v0.2.6] - 2025-08-10
- Initial public manifests bundle.
- CI: lint, tests, container build, CRD generation, Kind sanity checks.
- Core controllers and CRDs for `PhysicalHost`, `Beskar7Machine`, `Beskar7Cluster`.

[Unreleased]: https://github.com/wrkode/beskar7/compare/v0.2.6...HEAD
[v0.2.6]: https://github.com/wrkode/beskar7/releases/tag/v0.2.6

