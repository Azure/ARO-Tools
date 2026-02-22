package client

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func TestReleaseDeployment_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    *ReleaseDeployment
		wantErr bool
	}{
		{
			name:    "single region",
			fixture: "testdata/inputs/release_single_region.yaml",
			want: &ReleaseDeployment{
				Metadata: ReleaseMetadata{
					UpstreamRevision: "bbbbbbbbbbbb",
					Revision:         "aaaaaaaaaaaa",
					Branch:           "main",
					Timestamp:        "2025-09-21T00:38:14Z",
					PullRequestID:    11111111,
					ServiceGroup:     "Microsoft.Azure.ARO.HCP.Global",
					ServiceGroupBase: "Microsoft.Azure.ARO.HCP",
				},
				Target: DeploymentTarget{
					Cloud:         "public",
					Environment:   "int",
					RegionConfigs: []string{"region"},
				},
				Components: make(Components),
			},
			wantErr: false,
		},
		{
			name:    "multiple regions",
			fixture: "testdata/inputs/release_multiple_regions.yaml",
			want: &ReleaseDeployment{
				Metadata: ReleaseMetadata{
					UpstreamRevision: "bbbbbbbbbbbb",
					Revision:         "aaaaaaaaaaaa",
					Branch:           "main",
					Timestamp:        "2025-11-05T10:00:00Z",
					PullRequestID:    12345678,
					ServiceGroup:     "Microsoft.Azure.ARO.HCP.Global",
					ServiceGroupBase: "Microsoft.Azure.ARO.HCP",
				},
				Target: DeploymentTarget{
					Cloud:         "public",
					Environment:   "stg",
					RegionConfigs: []string{"region1", "region2", "region3"},
				},
				Components: make(Components),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.fixture)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", tt.fixture, err)
			}

			var got ReleaseDeployment
			err = yaml.Unmarshal(data, &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(tt.want, &got); diff != "" {
				t.Errorf("UnmarshalYAML() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
