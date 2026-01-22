# Test Credential Sanitization

## Overview

All hardcoded test credentials have been sanitized to prevent gosec (Go security linter) from flagging them as security issues. This follows security best practices for test code.

## Implementation

### 1. Central Helper Functions

Created `test_helpers.go` with centralized credential helpers:

```go
func getTestToken() string
func getTestPassword() string
func getTestAPIKey() string
func getTestSecret() string
func getTestUsername() string
func getTestOAuthUser() string
```

### 2. Environment Variable Support

Each helper function checks environment variables first:
- `TEST_GIT_TOKEN` - for test tokens
- `TEST_GIT_PASSWORD` - for test passwords
- `TEST_API_KEY` - for test API keys
- `TEST_SECRET` - for test secrets
- `TEST_GIT_USERNAME` - for test usernames

### 3. Safe Defaults

If environment variables are not set, functions return placeholder values with `#nosec G101` comments to suppress gosec warnings:

```go
return "test_token_placeholder_123" // #nosec G101 - test credential placeholder
```

## Benefits

1. **Security Compliance**: No hardcoded credentials flagged by gosec
2. **Flexibility**: Can use real credentials in CI/CD by setting env vars
3. **Consistency**: All tests use the same credential values
4. **Maintainability**: Single place to update test credentials
5. **Documentation**: Clear intent that these are test placeholders

## Usage in Tests

### Before (❌ Flagged by gosec):
```go
token := "ghp_secret_token_123"
password := "mypassword"
```

### After (✅ Clean):
```go
token := getTestToken()
password := getTestPassword()
```

## Running Tests with Custom Credentials

For local testing or CI/CD:

```bash
export TEST_GIT_TOKEN="your_test_token"
export TEST_GIT_PASSWORD="your_test_password"
export TEST_API_KEY="your_test_api_key"
go test ./...
```

## Files Modified

1. `test_helpers.go` - New file with credential helpers
2. `context_test.go` - Updated to use helpers
3. `builder_test.go` - Updated to use helpers

## Tests Updated

### context_test.go:
- TestExpandEnvInURL
- TestAddGitToken
- TestFormatGitURLForBuildKit_WithToken
- TestMaskToken
- BenchmarkAddGitToken
- BenchmarkMaskToken

### builder_test.go:
- TestSanitizeCommandArgs
- TestSanitizeCommandArgs_AllBranches
- TestSanitizeCommandArgs_EdgeCases
- TestSignImageWithCosign_WithPasswordEnv
- BenchmarkSanitizeCommandArgs

## Verification

All tests pass with sanitized credentials:

```bash
$ go test -v
PASS
ok      github.com/rapidfort/kimia/internal/build       0.311s
```

## Security Notes

- The `#nosec G101` comment is intentional and documented
- Actual secrets should NEVER be committed to the repository
- In production/CI, use proper secret management systems
- Test credentials are clearly marked as placeholders
