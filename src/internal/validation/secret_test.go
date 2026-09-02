package validation

import "testing"

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
