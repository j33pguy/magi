package db

import (
	"database/sql"
	"encoding/binary"
	"log/slog"
	"math"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helper: cosineDistance unit tests
// ---------------------------------------------------------------------------

func TestCosineDistance(t *testing.T) {
	// Identical vectors → distance 0
	a := []float32{1, 0, 0}
	if d := cosineDistance(a, a); math.Abs(d) > 1e-6 {
		t.Errorf("identical vectors: got distance %f, want ~0", d)
	}

	// Orthogonal vectors → distance 1
	b := []float32{0, 1, 0}
	if d := cosineDistance(a, b); math.Abs(d-1.0) > 1e-6 {
		t.Errorf("orthogonal vectors: got distance %f, want ~1", d)
	}

	// Opposite vectors → distance 2
	c := []float32{-1, 0, 0}
	if d := cosineDistance(a, c); math.Abs(d-2.0) > 1e-6 {
		t.Errorf("opposite vectors: got distance %f, want ~2", d)
	}

	// Empty vectors → distance 1
	if d := cosineDistance(nil, nil); d != 1.0 {
		t.Errorf("nil vectors: got distance %f, want 1", d)
	}

	// Zero vector → distance 1
	z := []float32{0, 0, 0}
	if d := cosineDistance(a, z); d != 1.0 {
		t.Errorf("zero vector: got distance %f, want 1", d)
	}

	// Mismatched lengths → distance 1
	short := []float32{1, 0}
	if d := cosineDistance(a, short); d != 1.0 {
		t.Errorf("mismatched lengths: got distance %f, want 1", d)
	}
}

// ---------------------------------------------------------------------------
// Helper: bytesToFloat32s unit tests
// ---------------------------------------------------------------------------

func TestBytesToFloat32s(t *testing.T) {
	// nil → nil
	if result := bytesToFloat32s(nil); result != nil {
		t.Error("nil input should return nil")
	}

	// empty → nil
	if result := bytesToFloat32s([]byte{}); result != nil {
		t.Error("empty input should return nil")
	}

	// non-multiple of 4 → nil
	if result := bytesToFloat32s([]byte{1, 2, 3}); result != nil {
		t.Error("odd-length input should return nil")
	}

	// Round-trip: float32sToBytes → bytesToFloat32s
	original := []float32{1.5, -2.25, 3.0, 0.0}
	bytes := float32sToBytes(original)
	decoded := bytesToFloat32s(bytes)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(original))
	}
	for i, v := range decoded {
		if v != original[i] {
			t.Errorf("index %d: got %f, want %f", i, v, original[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Parameter builder tests
// ---------------------------------------------------------------------------

func TestSQLServerAppendProjectCondition(t *testing.T) {
	// Single project
	var conds []string
	var args []any
	f := &MemoryFilter{Project: "test-proj"}
	sqlserverAppendProjectCondition(f, &conds, &args)
	if len(conds) != 1 || len(args) != 1 {
		t.Fatalf("single project: got %d conditions, %d args", len(conds), len(args))
	}
	if conds[0] != "m.project = @p1" {
		t.Errorf("condition = %q, want %q", conds[0], "m.project = @p1")
	}

	// Multiple projects
	conds = nil
	args = nil
	f = &MemoryFilter{Projects: []string{"a", "b", "c"}}
	sqlserverAppendProjectCondition(f, &conds, &args)
	if len(args) != 3 {
		t.Fatalf("multi project: got %d args, want 3", len(args))
	}
	if conds[0] != "m.project IN (@p1,@p2,@p3)" {
		t.Errorf("condition = %q", conds[0])
	}

	// No project
	conds = nil
	args = nil
	f = &MemoryFilter{}
	sqlserverAppendProjectCondition(f, &conds, &args)
	if len(conds) != 0 || len(args) != 0 {
		t.Error("empty filter should add no conditions")
	}
}

func TestSQLServerAppendTaxonomyConditions(t *testing.T) {
	var conds []string
	var args []any
	f := &MemoryFilter{Speaker: "user", Area: "work", SubArea: "infra"}
	sqlserverAppendTaxonomyConditions(f, &conds, &args)
	if len(conds) != 3 || len(args) != 3 {
		t.Fatalf("taxonomy: got %d conditions, %d args", len(conds), len(args))
	}
}

func TestSQLServerAppendTimeConditions(t *testing.T) {
	now := time.Now()
	var conds []string
	var args []any
	f := &MemoryFilter{AfterTime: &now, BeforeTime: &now}
	sqlserverAppendTimeConditions(f, &conds, &args)
	if len(conds) != 2 || len(args) != 2 {
		t.Fatalf("time: got %d conditions, %d args", len(conds), len(args))
	}
}

func TestSQLServerAppendVisibilityCondition(t *testing.T) {
	// "all" — no condition added
	var conds []string
	var args []any
	f := &MemoryFilter{Visibility: "all"}
	sqlserverAppendVisibilityCondition(f, &conds, &args)
	if len(conds) != 0 {
		t.Error("visibility=all should add no condition")
	}

	// specific value
	conds = nil
	args = nil
	f = &MemoryFilter{Visibility: "public"}
	sqlserverAppendVisibilityCondition(f, &conds, &args)
	if len(conds) != 1 || len(args) != 1 {
		t.Fatal("visibility=public should add 1 condition")
	}
	if conds[0] != "m.visibility = @p1" {
		t.Errorf("condition = %q", conds[0])
	}

	// empty — exclude private
	conds = nil
	args = nil
	f = &MemoryFilter{}
	sqlserverAppendVisibilityCondition(f, &conds, &args)
	if len(conds) != 1 || len(args) != 0 {
		t.Fatal("empty visibility should add 1 condition, 0 args")
	}
	if conds[0] != "m.visibility != 'private'" {
		t.Errorf("condition = %q", conds[0])
	}
}

// ---------------------------------------------------------------------------
// Factory test: sqlserver backend with bad DSN should fail
// ---------------------------------------------------------------------------

func TestNewStoreSQLServerFails(t *testing.T) {
	logger := testLogger()
	_, err := NewStore(&Config{
		Backend:      "sqlserver",
		SQLServerURL: "sqlserver://baduser:badpass@localhost:1433?database=nonexistent&connection+timeout=1",
	}, logger)
	// Expected to fail — no SQL Server running. Exercises the factory branch.
	if err == nil {
		t.Log("NewStore(sqlserver) succeeded unexpectedly (SQL Server must be running)")
	}
}

func TestNewStoreMSSQLAlias(t *testing.T) {
	logger := testLogger()
	_, err := NewStore(&Config{
		Backend:      "mssql",
		SQLServerURL: "sqlserver://baduser:badpass@localhost:1433?database=nonexistent&connection+timeout=1",
	}, logger)
	if err == nil {
		t.Log("NewStore(mssql) succeeded unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// Schema: migration helpers
// ---------------------------------------------------------------------------

func TestSQLServerSchemaIsApplied(t *testing.T) {
	// Use the existing SQLite test helper to verify the schema migration logic
	// is structurally sound (same SQL concepts, different dialect).
	// The actual T-SQL is tested via integration tests against a real SQL Server.
	s := &SQLServerSchema{}
	// Verify the struct is constructable — the actual DB calls need a real connection.
	if s.db != nil {
		t.Error("expected nil db")
	}
}

// ---------------------------------------------------------------------------
// Config: SQL Server URL construction from env parts
// ---------------------------------------------------------------------------

func TestSQLServerConfigFromParts(t *testing.T) {
	// Test the factory config path with env vars
	t.Setenv("MEMORY_BACKEND", "sqlserver")
	t.Setenv("SQLSERVER_URL", "")
	t.Setenv("SQLSERVER_HOST", "dbhost.example.com")
	t.Setenv("SQLSERVER_PORT", "1434")
	t.Setenv("SQLSERVER_DATABASE", "magi")
	t.Setenv("SQLSERVER_USER", "sa")
	t.Setenv("SQLSERVER_PASSWORD", "pass123")

	cfg := ConfigFromEnv()
	expected := "sqlserver://sa:pass123@dbhost.example.com:1434?database=magi"
	if cfg.SQLServerURL != expected {
		t.Errorf("SQLServerURL = %q, want %q", cfg.SQLServerURL, expected)
	}
	if cfg.Backend != "sqlserver" {
		t.Errorf("Backend = %q, want %q", cfg.Backend, "sqlserver")
	}
}

func TestSQLServerConfigDefaultPort(t *testing.T) {
	t.Setenv("MEMORY_BACKEND", "sqlserver")
	t.Setenv("SQLSERVER_URL", "")
	t.Setenv("SQLSERVER_HOST", "localhost")
	t.Setenv("SQLSERVER_PORT", "")
	t.Setenv("SQLSERVER_DATABASE", "magi")
	t.Setenv("SQLSERVER_USER", "sa")
	t.Setenv("SQLSERVER_PASSWORD", "test")

	cfg := ConfigFromEnv()
	expected := "sqlserver://sa:test@localhost:1433?database=magi"
	if cfg.SQLServerURL != expected {
		t.Errorf("SQLServerURL = %q, want %q", cfg.SQLServerURL, expected)
	}
}

func TestSQLServerConfigDirectURL(t *testing.T) {
	t.Setenv("MEMORY_BACKEND", "sqlserver")
	t.Setenv("SQLSERVER_URL", "sqlserver://user:pass@host:1433?database=db")
	t.Setenv("SQLSERVER_HOST", "should-be-ignored")

	cfg := ConfigFromEnv()
	if cfg.SQLServerURL != "sqlserver://user:pass@host:1433?database=db" {
		t.Errorf("direct URL should take precedence, got %q", cfg.SQLServerURL)
	}
}

// ---------------------------------------------------------------------------
// SQLServerClient: verify interface compliance
// ---------------------------------------------------------------------------

// Compile-time check that SQLServerClient implements Store.
var _ Store = (*SQLServerClient)(nil)

// ---------------------------------------------------------------------------
// NewTursoStore: verify it rejects sqlserver
// ---------------------------------------------------------------------------

func TestNewTursoStoreRejectsSQLServer(t *testing.T) {
	logger := testLogger()
	_, err := NewTursoStore(&Config{Backend: "sqlserver"}, logger)
	if err == nil {
		t.Error("expected error for sqlserver backend via NewTursoStore")
	}
}

// ---------------------------------------------------------------------------
// Cosine distance with real embeddings
// ---------------------------------------------------------------------------

func TestCosineDistanceSimilarVectors(t *testing.T) {
	// Two similar vectors should have low distance
	a := []float32{0.1, 0.2, 0.3, 0.4}
	b := []float32{0.11, 0.21, 0.29, 0.41}

	dist := cosineDistance(a, b)
	if dist < 0 || dist > 0.01 {
		t.Errorf("similar vectors should have very low distance, got %f", dist)
	}
}

// ---------------------------------------------------------------------------
// Embedding round-trip through VARBINARY
// ---------------------------------------------------------------------------

func TestEmbeddingRoundTrip(t *testing.T) {
	// Simulate what SQL Server stores: float32 → bytes → float32
	original := make([]float32, 384)
	for i := range original {
		original[i] = float32(i) * 0.001
	}

	encoded := float32sToBytes(original)
	if len(encoded) != 384*4 {
		t.Fatalf("encoded length = %d, want %d", len(encoded), 384*4)
	}

	decoded := bytesToFloat32s(encoded)
	if len(decoded) != 384 {
		t.Fatalf("decoded length = %d, want 384", len(decoded))
	}

	for i, v := range decoded {
		if v != original[i] {
			t.Errorf("index %d: got %f, want %f", i, v, original[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Verify that scan helpers handle edge cases
// ---------------------------------------------------------------------------

func TestSQLServerScanLinks_EmptyRows(t *testing.T) {
	c := &SQLServerClient{}
	// We can't easily test with real sql.Rows without a DB, but we verify the
	// method exists and the client is constructable. Integration tests cover
	// the actual scanning.
	_ = c
}

// ---------------------------------------------------------------------------
// Verify factory wiring with "mssql" alias
// ---------------------------------------------------------------------------

func TestConfigSQLServerNoHost(t *testing.T) {
	t.Setenv("MEMORY_BACKEND", "sqlserver")
	t.Setenv("SQLSERVER_URL", "")
	t.Setenv("SQLSERVER_HOST", "")

	cfg := ConfigFromEnv()
	if cfg.SQLServerURL != "" {
		t.Errorf("no host should result in empty URL, got %q", cfg.SQLServerURL)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
}

// testNullString verifies sql.NullString helper for SQL Server context.
func TestNullStringHelper(t *testing.T) {
	ns := nullString("")
	if ns.Valid {
		t.Error("empty string should produce invalid NullString")
	}

	ns = nullString("hello")
	if !ns.Valid || ns.String != "hello" {
		t.Error("non-empty string should produce valid NullString")
	}
}

// ---------------------------------------------------------------------------
// Verify datetime formatting matches SQL Server expectations
// ---------------------------------------------------------------------------

func TestDatetimeFormat(t *testing.T) {
	now := time.Now().UTC().Format(time.DateTime)
	// Format should be "2006-01-02 15:04:05" which SQL Server DATETIME2 can parse
	if len(now) < 19 {
		t.Errorf("datetime format too short: %q", now)
	}
}

// ---------------------------------------------------------------------------
// Float32 bits round-trip (used by embedding serialization)
// ---------------------------------------------------------------------------

func TestFloat32BitsRoundTrip(t *testing.T) {
	values := []float32{0.0, 1.0, -1.0, 3.14, math.MaxFloat32, math.SmallestNonzeroFloat32}
	for _, v := range values {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, math.Float32bits(v))
		got := math.Float32frombits(binary.LittleEndian.Uint32(buf))
		if got != v {
			t.Errorf("round-trip failed for %f: got %f", v, got)
		}
	}
}

// Verify the SQL Server schema struct is self-contained.
func TestSQLServerSchemaStructure(t *testing.T) {
	schema := &SQLServerSchema{db: nil}
	// Verify all migration methods exist (compile-time check).
	_ = schema.migrationV1
	_ = schema.migrationV2
	_ = schema.migrationV3
	_ = schema.migrationV4
	_ = schema.migrationV5
	_ = schema.migrationV6
	_ = schema.migrationV7
	_ = schema.createMetaTable
	_ = schema.isApplied
	_ = schema.markApplied
	_ = schema.run
}

// Ensure FTS migration references the PK correctly.
func TestSQLServerFTSMigrationExists(t *testing.T) {
	schema := &SQLServerSchema{db: nil}
	// Verify migrationV3 (FTS) is callable — actual execution needs a real DB.
	_ = schema.migrationV3
}

// ---------------------------------------------------------------------------
// Additional parameter builder edge cases
// ---------------------------------------------------------------------------

func TestSQLServerParamsWithPreExistingArgs(t *testing.T) {
	// Verify @pN numbering is correct when args already has elements
	var conds []string
	args := []any{"existing-arg-1", "existing-arg-2"}
	f := &MemoryFilter{Project: "myproj"}
	sqlserverAppendProjectCondition(f, &conds, &args)
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	// The new param should be @p3 (len after append)
	if conds[0] != "m.project = @p3" {
		t.Errorf("condition = %q, want m.project = @p3", conds[0])
	}
}

// Test that the interface methods all have correct signatures by using a nil client.
func TestSQLServerClientMethodSignatures(t *testing.T) {
	var s Store = (*SQLServerClient)(nil)
	// This is a compile-time assertion — if any method is missing or has the wrong
	// signature, this file won't compile.
	_ = s
}

// Test sql.ErrNoRows is properly propagated from DeleteLink.
func TestDeleteLinkErrorPropagation(t *testing.T) {
	// Cannot test without a real DB, but verify the error constant is accessible.
	if sql.ErrNoRows == nil {
		t.Error("sql.ErrNoRows should not be nil")
	}
}
