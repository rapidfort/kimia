# Contributing to Kimia

Thank you for your interest in contributing to Kimia! We welcome contributions from the community.

## How to Contribute

We appreciate various types of contributions:

- **Bug fixes**: Help us identify and fix issues
- **New features**: Propose and implement new functionality
- **Documentation**: Improve or expand documentation
- **Performance improvements**: Optimize existing code

## Development Setup

### Prerequisites

- Git
- Go 1.19 or higher
- Docker, Podman, Buildah, or comparable container engine (for testing container functionality)
- Kubernetes cluster and kubectl (for testing Kubernetes functionality)

### Building from Source

```bash
# Build the project
make build
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage
```

## Pull Request Process

1. Fork the repository
2. Create a branch
3. Implement your changes on the branch and ensure that the tests pass
4. Open a Pull Request on GitHub with
   - Clear title describing the change
   - Detailed description of what changed and why
   - Reference to any related issues (e.g., "Fixes #123")
   - Screenshots or examples if applicable
5. Address review feedback promptly and respectfully

## Reporting Issues

When reporting issues, please include:

- **Clear title** describing the problem
- **Detailed description** of the issue
- **Steps to reproduce** the problem
- **Expected behavior** vs actual behavior
- **Environment details**: OS, Go version, container engine and version, Kubernetes distribution and version
- **Error messages** or logs if applicable

## Questions?

- Open an issue for questions about contributing
- Check existing issues and pull requests first
- Be patient and respectful when seeking help

## License

By contributing to Kimia, you agree that your contributions will be licensed under the same license as the project.

---

Thank you for contributing to Kimia! 🎉