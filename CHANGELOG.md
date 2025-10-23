# Changelog

All notable changes to Smithy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- New features here
- Added --image-download-retry flag for controlling retry attempts when pulling base images during build

### Changed
- Changes in existing functionality

### Fixed
- Bug fixes

## [1.0.10] - 2025-10-21

### Added
- Added --reproducible option

### Changed

### Fixed

## [1.0.9] - 2025-10-13

### Added

### Changed
- Added examples (community nginx & redis)

### Fixed

## [1.0.8] - 2025-10-12

### Added

### Changed
- Minor cleanup

### Fixed

## [1.0.7] - 2025-10-12

### Added

### Changed
- buildah version tagged with -rf for patch

### Fixed
- Arm64 build

## [1.0.4] - 2025-10-12

### Fixed
- GHCR authentication in release pipeline
- Version tag format (removed 'v' prefix from image tags)
- Multi-arch build workflow simplification

## [1.0.2] - 2025-10-12

### Added
- Multi-arch support (amd64, arm64)
- Comprehensive documentation
- GitHub Actions workflow

### Fixed
- Cache permission issues
- Git authentication in builds

## [1.0.1] - 2025-10-11

### Added
- Initial public release
- Basic build functionality

