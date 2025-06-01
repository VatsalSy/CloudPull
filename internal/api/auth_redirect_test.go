package api

import (
	"testing"
)

func TestExtractRedirectURL(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    string
		wantErr bool
	}{
		{
			name: "installed app credentials",
			json: `{
				"installed": {
					"client_id": "test.apps.googleusercontent.com",
					"redirect_uris": ["http://localhost", "urn:ietf:wg:oauth:2.0:oob"]
				}
			}`,
			want:    "http://localhost",
			wantErr: false,
		},
		{
			name: "web app credentials",
			json: `{
				"web": {
					"client_id": "test.apps.googleusercontent.com",
					"redirect_uris": ["http://localhost:8080/callback"]
				}
			}`,
			want:    "http://localhost:8080/callback",
			wantErr: false,
		},
		{
			name: "custom redirect URI",
			json: `{
				"installed": {
					"client_id": "test.apps.googleusercontent.com",
					"redirect_uris": ["http://localhost:3000/auth"]
				}
			}`,
			want:    "http://localhost:3000/auth",
			wantErr: false,
		},
		{
			name: "no redirect URIs",
			json: `{
				"installed": {
					"client_id": "test.apps.googleusercontent.com"
				}
			}`,
			want:    "",
			wantErr: true,
		},
		{
			name: "empty redirect URIs array",
			json: `{
				"installed": {
					"client_id": "test.apps.googleusercontent.com",
					"redirect_uris": []
				}
			}`,
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			json:    `{"invalid": json}`,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRedirectURL([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractRedirectURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractRedirectURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
