package syncagent

import "time"

// ConflictResult indicates which version should be kept after resolution.
type ConflictResult int

const (
	KeepLocal  ConflictResult = iota // Keep the local version.
	KeepRemote                       // Keep the remote version.
	KeepBoth                         // Both changed and manual resolution is needed.
)

// RemoteState represents a memory fetched from the server for conflict comparison.
type RemoteState struct {
	SHA256    string
	UpdatedAt time.Time
}

// DetectConflict returns true when both the local file and the remote memory
// have been modified since the last sync checkpoint.
func DetectConflict(localHash string, lastSyncHash string, remote RemoteState) bool {
	localChanged := localHash != lastSyncHash
	remoteChanged := remote.SHA256 != lastSyncHash
	// If both changed but converged to the same content, no conflict.
	if localHash == remote.SHA256 {
		return false
	}
	return localChanged && remoteChanged
}

// ResolveConflict decides which version to keep based on the configured strategy.
//
// For last-write-wins and newest, the localModTime is compared against
// remote.UpdatedAt. For manual, KeepBoth is returned so the caller can
// surface the conflict to the user.
func ResolveConflict(strategy ConflictStrategy, localModTime time.Time, remote RemoteState) ConflictResult {
	switch strategy {
	case ConflictLastWriteWins, ConflictNewest:
		if localModTime.After(remote.UpdatedAt) {
			return KeepLocal
		}
		return KeepRemote
	case ConflictManual:
		return KeepBoth
	default:
		// Unknown strategy falls back to last-write-wins behavior.
		if localModTime.After(remote.UpdatedAt) {
			return KeepLocal
		}
		return KeepRemote
	}
}
