package config

import "testing"

func TestGetByPath(t *testing.T) {
	tests := []struct {
		name  string
		vars  Configuration
		path  string
		want  any
		found bool
	}{
		{
			name: "simple",
			vars: Configuration{
				"key": "value",
			},
			path:  "key",
			want:  "value",
			found: true,
		},
		{
			name: "nested",
			vars: Configuration{
				"key": Configuration{
					"key": "value",
				},
			},
			path:  "key.key",
			want:  "value",
			found: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := tt.vars.GetByPath(tt.path)
			if got != tt.want {
				t.Errorf("Configuration.GetByPath() got = %v, want %v", got, tt.want)
			}
			if found != tt.found {
				t.Errorf("Configuration.GetByPath() found = %v, want %v", found, tt.found)
			}
		})
	}
}
