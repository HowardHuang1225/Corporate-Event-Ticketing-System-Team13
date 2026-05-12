package event

import (
	"errors"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/service/apperror"
)

func TestStatusAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		event model.Event
		want  string
	}{
		{
			name: "published remains published before deadline",
			event: model.Event{
				Status:        "published",
				ApplyDeadline: now.Add(time.Hour),
			},
			want: "published",
		},
		{
			name: "published becomes closed after deadline",
			event: model.Event{
				Status:        "published",
				ApplyDeadline: now.Add(-time.Hour),
			},
			want: "closed",
		},
		{
			name: "closed becomes ended after end time",
			event: model.Event{
				Status:  "closed",
				EndTime: now.Add(-time.Hour),
			},
			want: "ended",
		},
		{
			name: "draft remains draft",
			event: model.Event{
				Status: "draft",
			},
			want: "draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusAt(tt.event, now); got != tt.want {
				t.Fatalf("StatusAt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTimeline(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		publishTime time.Time
		startTime   time.Time
		deadline    time.Time
		endTime     time.Time
		wantErr     bool
	}{
		{
			name:        "allows publish time equal to start time",
			publishTime: now,
			startTime:   now,
			deadline:    now.Add(time.Hour),
			endTime:     now.Add(2 * time.Hour),
		},
		{
			name:        "allows apply deadline equal to end time",
			publishTime: now,
			startTime:   now.Add(time.Hour),
			deadline:    now.Add(2 * time.Hour),
			endTime:     now.Add(2 * time.Hour),
		},
		{
			name:        "rejects publish time after start time",
			publishTime: now.Add(2 * time.Hour),
			startTime:   now.Add(time.Hour),
			deadline:    now.Add(3 * time.Hour),
			endTime:     now.Add(4 * time.Hour),
			wantErr:     true,
		},
		{
			name:        "rejects missing publish time",
			publishTime: time.Time{},
			startTime:   now.Add(time.Hour),
			deadline:    now.Add(2 * time.Hour),
			endTime:     now.Add(3 * time.Hour),
			wantErr:     true,
		},
		{
			name:        "rejects apply deadline before start time",
			publishTime: now,
			startTime:   now.Add(2 * time.Hour),
			deadline:    now.Add(time.Hour),
			endTime:     now.Add(3 * time.Hour),
			wantErr:     true,
		},
		{
			name:        "rejects start time equal to apply deadline",
			publishTime: now,
			startTime:   now.Add(time.Hour),
			deadline:    now.Add(time.Hour),
			endTime:     now.Add(2 * time.Hour),
			wantErr:     true,
		},
		{
			name:        "rejects apply deadline after end time",
			publishTime: now,
			startTime:   now.Add(time.Hour),
			deadline:    now.Add(3 * time.Hour),
			endTime:     now.Add(2 * time.Hour),
			wantErr:     true,
		},
		{
			name:        "rejects end time before start time and apply deadline",
			publishTime: now,
			startTime:   now.Add(2 * time.Hour),
			deadline:    now.Add(3 * time.Hour),
			endTime:     now.Add(time.Hour),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("checking validateTimeline case: %s", tt.name)
			err := validateTimeline(tt.publishTime, tt.startTime, tt.deadline, tt.endTime)
			if tt.wantErr {
				if err == nil {
					t.Fatal("validateTimeline() expected error")
				}
				var appErr *apperror.Error
				if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
					t.Fatalf("validateTimeline() error = %v, want VALIDATION_ERROR", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateTimeline() error = %v", err)
			}
		})
	}
}
