package timeparse

import (
	"reflect"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Duration
		wantErr bool
	}{
		{
			name:    "standard hours",
			args:    args{s: "2h"},
			want:    2 * time.Hour,
			wantErr: false,
		},
		{
			name:    "standard minutes",
			args:    args{s: "30m"},
			want:    30 * time.Minute,
			wantErr: false,
		},
		{
			name:    "mixed standard duration",
			args:    args{s: "1h30m"},
			want:    90 * time.Minute,
			wantErr: false,
		},
		{
			name:    "days suffix",
			args:    args{s: "1d"},
			want:    24 * time.Hour,
			wantErr: false,
		},
		{
			name:    "weeks suffix",
			args:    args{s: "2w"},
			want:    14 * 24 * time.Hour,
			wantErr: false,
		},
		{
			name:    "invalid unit",
			args:    args{s: "5x"},
			want:    0,
			wantErr: true,
		},
		{
			name:    "not a duration",
			args:    args{s: "abc"},
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimeToUTC(t *testing.T) {
	type args struct {
		timeStr string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "valid RFC3339 UTC",
			args: args{
				timeStr: "2025-11-02T15:30:00Z",
			},
			want:    time.Date(2025, 11, 2, 15, 30, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "valid RFC3339 with offset",
			args: args{
				timeStr: "2025-11-02T10:30:00-05:00",
			},
			// 10:30 -05:00 == 15:30 UTC
			want:    time.Date(2025, 11, 2, 15, 30, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "duration 1d relative",
			args: args{
				timeStr: "1d",
			},
			want:    time.Date(2024, 12, 31, 12, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "duration 2w relative",
			args: args{
				timeStr: "2w",
			},
			want:    time.Date(2024, 12, 18, 12, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "invalid input",
			args: args{
				timeStr: "not-a-time",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixedNow := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
			parser := NewParser(fakeClock{now: fixedNow})

			got, err := parser.ToUTC(tt.args.timeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimeToUTC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTimeToUTC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatRelativeTime(t *testing.T) {
	type args struct {
		d time.Duration
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "less than a minute",
			args: args{
				d: 30 * time.Second,
			},
			want: "less than a minute",
		},
		{
			name: "one minute",
			args: args{
				d: 1 * time.Minute,
			},
			want: "1 minute",
		},
		{
			name: "multiple minutes",
			args: args{
				d: 5 * time.Minute,
			},
			want: "5 minutes",
		},
		{
			name: "one hour",
			args: args{
				d: 1 * time.Hour,
			},
			want: "1 hour",
		},
		{
			name: "multiple hours",
			args: args{
				d: 3 * time.Hour,
			},
			want: "3 hours",
		},
		{
			name: "one day",
			args: args{
				d: 24 * time.Hour,
			},
			want: "1 day",
		},
		{
			name: "multiple days",
			args: args{
				d: 48 * time.Hour,
			},
			want: "2 days",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatRelativeTime(tt.args.d); got != tt.want {
				t.Errorf("FormatRelativeTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time {
	return f.now
}
