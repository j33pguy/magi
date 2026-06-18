package db

import "fmt"

func persistPreparedMemoryGeneric(store Store, input PersistPreparedMemoryInput) (*PersistPreparedMemoryResult, error) {
	if input.Memory == nil {
		return nil, fmt.Errorf("memory is required")
	}

	saved, err := store.SaveMemory(input.Memory)
	if err != nil {
		return nil, err
	}

	if input.Context != nil {
		ctxCopy := *input.Context
		ctxCopy.MemoryID = saved.ID
		if err := store.SaveMemoryContext(&ctxCopy); err != nil {
			return nil, err
		}
	}

	var tagWarning string
	if len(input.Tags) > 0 {
		if err := store.SetTags(saved.ID, input.Tags); err != nil {
			tagWarning = err.Error()
		}
	}

	return &PersistPreparedMemoryResult{Saved: saved, TagWarning: tagWarning}, nil
}
