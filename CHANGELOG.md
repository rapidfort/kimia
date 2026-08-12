# Changelog

All notable changes to Kimia will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Fixed

### Removed

## [1.0.26] - 2026-08-12

### Added
- Dependency audit tooling for intentional pin updates:
  - `make apk` / `scripts/make-apk.sh` — Alpine OS package pins vs latest
  - `make go-deps` / `scripts/make-go.sh` — Go toolchain (`GO_IMAGE`) digests
  - `make tools` / `scripts/make-tools.sh` — BuildKit, Cosign, credential helper versions
  - `make deps` — run all three audits

### Changed
- Bumped runtime base to Alpine **3.24.1** (digest-pinned) for both BuildKit and Buildah images.
- Bumped Go builder image to **golang:1.26.5-alpine3.24** (digest-pinned) for both Dockerfiles.
- Refreshed pinned `apk` package versions for Alpine 3.24 (e.g. bash, git, crun, shadow, ca-certificates; Buildah stack including buildah 1.44.x, netavark, aardvark-dns).
- Updated AWS ECR credential helper to **0.12.0** and Google docker-credential-gcr to **2.2.1**.
- Pin base images and OS/application packages by digest/version in Dockerfiles (reproducible, intentional upgrades).
- Pin GitHub Actions and security scanner action versions in CI workflows (`scan-image.yml`, `scan-repository.yml`, CodeQL-related fixes).

### Fixed
- CI workflow pinning issues for repository/image scanners and CodeQL configuration.

### Removed

## [1.0.25] - 2026-02-18

### Added
- Added `--buildah-opt` to pass arguments directly to Buildah.
- Added `--export-cache` and `--import-cache` flags for BuildKit advanced caching.

### Fixed
- Temporary build directories are now cleaned up on failed builds.
- Fixed bug where digest file was not created when `--no-push` is set.

## [1.0.24] - 2026-02-18

### Changed
- Updated RapidFort CI/CD workflow configuration and permissions.

### Fixed
- Trivy Action version pinning in CI.

## [1.0.23] - 2026-02-18

### Added
- Added --digest-file export handling for BuildKit output.
- Added --reproducible flag support for BuildKit provenance attestations.

### Changed

### Fixed
- Buildah --tar-path export now works without registry auth configuration
- Remediations and updates for security scanner findings

### Removed

## [1.0.22] - 2025-12-02

### Added
- Dependency verification stage with hash checking
- Support for arguments in image sources
- Attestation and signing support with multiple modes

### Changed
- Improved Git repository handling in provenance
- Enhanced registry authentication cleanup

### Fixed
- Environment configuration checks for building images
- git:// protocol issue in Kimia
- --context-sub-path handling for Git URLs (now uses #:subdir syntax)
- --insecure-registry flag to work properly with BuildKit
- Git branch and revision handling for Buildah

### Removed
- Redundant --skip-tls-verify flag (consolidated with --insecure-registry)

## [1.0.13] - 2025-10-23

### Fixed
- Fix --context-sub-path arg parsing when the context sub-path is an empty string "" for Kubernetes manifest files

## [1.0.12] - 2025-10-23

### Added
- Added --image-download-retry flag for controlling retry attempts when pulling base images during build

## [1.0.11] - 2025-10-22

### Fixed
- Fix --context-sub-path arg parsing when the context sub-path is an empty string ""

## [1.0.10] - 2025-10-21

### Added
- Added --reproducible option

## [1.0.9] - 2025-10-13

### Changed
- Added examples (community nginx & redis)

## [1.0.8] - 2025-10-12

### Changed
- Minor cleanup

## [1.0.7] - 2025-10-12

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
