package node

import "testing"

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	cap := &Capability{Type: TypeWriter, PoolSize: 4, Mode: ModeEmbedded}
	r.Register(cap)

	got := r.Get(TypeWriter)
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	if got.PoolSize != 4 {
		t.Errorf("pool size = %d, want 4", got.PoolSize)
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	if got := r.Get(TypeReader); got != nil {
		t.Errorf("expected nil for unregistered type, got %+v", got)
	}
}

func TestRegistryHas(t *testing.T) {
	r := NewRegistry()
	r.Register(&Capability{Type: TypeIndex, PoolSize: 1, Mode: ModeEmbedded})

	if !r.Has(TypeIndex) {
		t.Error("expected Has(TypeIndex) = true")
	}
	if r.Has(TypeCoordinator) {
		t.Error("expected Has(TypeCoordinator) = false")
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(&Capability{Type: TypeWriter, PoolSize: 4, Mode: ModeEmbedded})
	r.Unregister(TypeWriter)

	if r.Has(TypeWriter) {
		t.Error("expected writer to be unregistered")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Capability{Type: TypeWriter, PoolSize: 4, Mode: ModeEmbedded})
	r.Register(&Capability{Type: TypeReader, PoolSize: 8, Mode: ModeEmbedded})

	caps := r.List()
	if len(caps) != 2 {
		t.Errorf("list length = %d, want 2", len(caps))
	}
}
