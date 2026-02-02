// Package sandbox provides container state type constants.
package sandbox

// ContainerState represents the possible states of a container.
type ContainerState string

const (
	// StateRunning indicates the container is running.
	StateRunning ContainerState = "Running"
	// StateStopped indicates the container is stopped.
	StateStopped ContainerState = "Stopped"
	// StateFrozen indicates the container is frozen/paused.
	StateFrozen ContainerState = "Frozen"
	// StateUnknown is used when the state cannot be determined.
	StateUnknown ContainerState = "Unknown"
)

// IsRunning returns true if the state represents a running container.
func (s ContainerState) IsRunning() bool {
	return s == StateRunning
}

// IsStopped returns true if the state represents a stopped container.
func (s ContainerState) IsStopped() bool {
	return s == StateStopped
}

// String returns the string representation of the state.
func (s ContainerState) String() string {
	return string(s)
}

// ParseContainerState converts a string to ContainerState.
// Returns StateUnknown for unrecognized values.
func ParseContainerState(s string) ContainerState {
	switch s {
	case "Running":
		return StateRunning
	case "Stopped":
		return StateStopped
	case "Frozen":
		return StateFrozen
	default:
		return StateUnknown
	}
}

// CloudInitState represents cloud-init status values.
type CloudInitState string

const (
	// CloudInitDone indicates cloud-init completed successfully.
	CloudInitDone CloudInitState = "done"
	// CloudInitRunning indicates cloud-init is still running.
	CloudInitRunning CloudInitState = "running"
	// CloudInitError indicates cloud-init failed.
	CloudInitError CloudInitState = "error"
	// CloudInitDisabled indicates cloud-init is disabled.
	CloudInitDisabled CloudInitState = "disabled"
)

// IsDone returns true if cloud-init has completed successfully.
func (s CloudInitState) IsDone() bool {
	return s == CloudInitDone
}

// IsFailed returns true if cloud-init failed or is disabled.
func (s CloudInitState) IsFailed() bool {
	return s == CloudInitError || s == CloudInitDisabled
}
