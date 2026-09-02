package validation

import (
	"strings"
	"testing"
)

func TestValidateBuildSecretSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		// Valid forms
		{"file source", "id=npmrc,src=/run/secrets/npmrc", false},
		{"env source", "id=npm_token,env=NPM_TOKEN", false},
		{"shorthand id only", "NPM_TOKEN", false},
		{"id only via key", "id=npmrc", false},
		{"dotted id", "id=artifactory.npmrc,src=/run/secrets/npmrc", false},
		{"hyphenated id", "id=artifactory-token,src=/run/secrets/token", false},
		{"explicit type file", "id=npmrc,src=/run/secrets/npmrc,type=file", false},
		{"explicit type env", "id=tok,env=NPM_TOKEN,type=env", false},
		{"key order independent", "src=/run/secrets/npmrc,id=npmrc", false},

		// Invalid forms
		{"empty", "", true},
		{"missing id", "src=/run/secrets/npmrc", true},
		{"unknown key", "id=npmrc,dest=/tmp/x", true},
		{"src and env together", "id=npmrc,src=/run/secrets/npmrc,env=NPM_TOKEN", true},
		{"duplicate key", "id=npmrc,id=other", true},
		{"empty value", "id=npmrc,src=", true},
		{"id starts with digit", "id=1npmrc,src=/run/secrets/npmrc", true},
		{"path traversal in src", "id=npmrc,src=/run/secrets/../../etc/shadow", true},
		{"spec over length limit", "id=npmrc,src=/run/secrets/" + strings.Repeat("a", 1024), true},
		{"command substitution", "id=npmrc,src=$(whoami)", true},
		{"shell metacharacter", "id=npmrc,src=/tmp/x;rm -rf /", true},
		{"null byte", "id=npmrc,src=/tmp/\x00", true},
		{"bad env var name", "id=npmrc,env=lowercase", true},
		{"invalid type", "id=npmrc,src=/run/secrets/npmrc,type=socket", true},
		{"type env with src", "id=npmrc,src=/run/secrets/npmrc,type=env", true},
		{"type file with env", "id=npmrc,env=NPM_TOKEN,type=file", true},
		{"shorthand after pairs", "id=npmrc,bare", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuildSecretSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBuildSecretSpec(%q) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBuildKitCacheSpecBackends(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{"registry", "type=registry,ref=registry.io/cache:latest,mode=max", false},
		{"inline", "type=inline", false},
		{"local", "type=local,dest=/tmp/cache", false},
		{"s3", "type=s3,region=us-east-1,bucket=my-cache", false},
		{"s3 with prefix and mode", "type=s3,region=us-east-1,bucket=my-cache,prefix=build/,mode=max", false},
		{"azblob", "type=azblob,account_url=https://x.blob.core.windows.net,name=cache", false},
		{"gha", "type=gha", false},
		{"unknown backend", "type=gcs,bucket=my-cache", true},
		{"missing type", "bucket=my-cache", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuildKitCacheSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBuildKitCacheSpec(%q) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
			}
		})
	}
}

func TestBuildSecretID(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want string
	}{
		{"id first", "id=npmrc,src=/run/secrets/npmrc", "npmrc"},
		{"id last", "src=/run/secrets/npmrc,id=npmrc", "npmrc"},
		{"env source", "id=npm_token,env=NPM_TOKEN", "npm_token"},
		{"shorthand", "NPM_TOKEN", "NPM_TOKEN"},
		{"dotted id", "id=artifactory.npmrc,src=/run/secrets/npmrc", "artifactory.npmrc"},
		{"no id present", "src=/run/secrets/npmrc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildSecretID(tt.spec); got != tt.want {
				t.Errorf("BuildSecretID(%q) = %q, want %q", tt.spec, got, tt.want)
			}
		})
	}
}

// Two --secret flags sharing an id are rejected by the caller, which compares
// the ids this function returns. Guard the property that makes that work.
func TestBuildSecretIDDetectsDuplicates(t *testing.T) {
	specs := []string{"id=npmrc,src=/run/secrets/npmrc", "id=npmrc,env=NPM_TOKEN"}
	if BuildSecretID(specs[0]) != BuildSecretID(specs[1]) {
		t.Errorf("expected both specs to yield the same id, got %q and %q",
			BuildSecretID(specs[0]), BuildSecretID(specs[1]))
	}
}
