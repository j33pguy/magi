package db

// PersistPreparedMemoryInput captures a fully prepared memory write ready for atomic persistence.
type PersistPreparedMemoryInput struct {
	Memory  *Memory
	Tags    []string
	Context *MemoryContextRecord
}

// PersistPreparedMemoryResult captures the saved memory and any non-fatal tag warning.
type PersistPreparedMemoryResult struct {
	Saved      *Memory
	TagWarning string
}
