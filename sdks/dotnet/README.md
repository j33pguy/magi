# Magi.Client

.NET client SDK for [MAGI](https://github.com/j33pguy/magi).

## Install

```bash
dotnet add package Magi.Client
```

## Usage

```csharp
using Magi.Client;

var magi = new MagiClient(new("http://localhost:8302", Token: "your-token"));

await magi.RememberAsync(new() { Content = "v3 API deprecates /users", Project = "myapp", Type = "decision", Speaker = "grok" });

var results = await magi.RecallAsync(new() { Query = "API changes", Limit = 5 });

foreach (var r in results.Results)
    Console.WriteLine($"{r.Score:F2} — {r.Memory.Content}");
```

## Available Methods

| Method | Description |
|--------|-------------|
| `RememberAsync(RememberRequest)` | Store a memory |
| `RecallAsync(RecallRequest)` | Semantic search for memories |
| `ListAsync(Dictionary<string, string>?)` | List and filter memories |
| `ForgetAsync(string id)` | Archive (soft-delete) a memory |
| `HealthAsync()` | Check server health |
