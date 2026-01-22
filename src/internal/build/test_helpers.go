package build

import "os"

// Test credential helpers - get from env or use defaults
// This prevents gosec from flagging hardcoded credentials in tests

func getTestToken() string {
	if token := os.Getenv("TEST_GIT_TOKEN"); token != "" {
		return token
	}
	return "test_token_placeholder_123" // #nosec G101 - test credential placeholder
}

func getTestPassword() string {
	if pass := os.Getenv("TEST_GIT_PASSWORD"); pass != "" {
		return pass
	}
	return "test_pass_placeholder" // #nosec G101 - test credential placeholder
}

func getTestAPIKey() string {
	if key := os.Getenv("TEST_API_KEY"); key != "" {
		return key
	}
	return "test_api_key_placeholder" // #nosec G101 - test credential placeholder
}

func getTestSecret() string {
	if secret := os.Getenv("TEST_SECRET"); secret != "" {
		return secret
	}
	return "test_secret_placeholder" // #nosec G101 - test credential placeholder
}

func getTestUsername() string {
	if user := os.Getenv("TEST_GIT_USERNAME"); user != "" {
		return user
	}
	return "testuser" // #nosec G101 - test credential placeholder
}

func getTestOAuthUser() string {
	return "oauth2" // This is a standard convention, not a credential
}
