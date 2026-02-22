package output

import (
	"testing"
	"time"

	"github.com/Azure/ARO-Tools/pipelines/pkg/release/client"
)

func TestParseFormat(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    Format
		wantErr bool
	}{
		{
			name: "json format",
			args: args{s: "json"},
			want: FormatJSON,
		},
		{
			name: "yaml format",
			args: args{s: "yaml"},
			want: FormatYAML,
		},
		{
			name: "human format",
			args: args{s: "human"},
			want: FormatHuman,
		},
		{
			name:    "invalid format",
			args:    args{s: "xml"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatOutput(t *testing.T) {
	type args struct {
		deployments       []*client.ReleaseDeployment
		outputFormat      Format
		loc               *time.Location
		includeComponents bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "json empty slice",
			args: args{
				deployments:       []*client.ReleaseDeployment{},
				outputFormat:      FormatJSON,
				loc:               nil,
				includeComponents: false,
			},
			want:    "[]",
			wantErr: false,
		},
		{
			name: "human invalid timestamp",
			args: args{
				deployments: []*client.ReleaseDeployment{
					{
						Metadata: client.ReleaseMetadata{
							Timestamp: "not-a-time",
						},
					},
				},
				outputFormat:      FormatHuman,
				loc:               nil,
				includeComponents: false,
			},
			want:    "Found 1 deployment(s):\n\n1. Deployment to  was unknown ago (unknown (not-a-time))\n   Upstream Revision: \n   Revision: \n   Branch: \n\n",
			wantErr: false,
		},
		{
			name: "invalid format",
			args: args{
				deployments:       []*client.ReleaseDeployment{},
				outputFormat:      Format("invalid"),
				loc:               nil,
				includeComponents: false,
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatOutput(tt.args.deployments, tt.args.outputFormat, tt.args.loc, tt.args.includeComponents)
			if (err != nil) != tt.wantErr {
				t.Errorf("FormatOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FormatOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}
