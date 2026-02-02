package sandbox

import "testing"

func TestContainerState(t *testing.T) {
	tests := []struct {
		state     ContainerState
		isRunning bool
		isStopped bool
	}{
		{StateRunning, true, false},
		{StateStopped, false, true},
		{StateFrozen, false, false},
		{StateUnknown, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsRunning(); got != tt.isRunning {
				t.Errorf("IsRunning() = %v, want %v", got, tt.isRunning)
			}
			if got := tt.state.IsStopped(); got != tt.isStopped {
				t.Errorf("IsStopped() = %v, want %v", got, tt.isStopped)
			}
		})
	}
}

func TestParseContainerState(t *testing.T) {
	tests := []struct {
		input string
		want  ContainerState
	}{
		{"Running", StateRunning},
		{"Stopped", StateStopped},
		{"Frozen", StateFrozen},
		{"Unknown", StateUnknown},
		{"invalid", StateUnknown},
		{"", StateUnknown},
		{"running", StateUnknown}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseContainerState(tt.input); got != tt.want {
				t.Errorf("ParseContainerState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCloudInitState(t *testing.T) {
	tests := []struct {
		state    CloudInitState
		isDone   bool
		isFailed bool
	}{
		{CloudInitDone, true, false},
		{CloudInitRunning, false, false},
		{CloudInitError, false, true},
		{CloudInitDisabled, false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsDone(); got != tt.isDone {
				t.Errorf("IsDone() = %v, want %v", got, tt.isDone)
			}
			if got := tt.state.IsFailed(); got != tt.isFailed {
				t.Errorf("IsFailed() = %v, want %v", got, tt.isFailed)
			}
		})
	}
}
