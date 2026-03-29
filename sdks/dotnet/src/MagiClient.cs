using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Magi.Client;

public record MagiConfig(string BaseUrl, string? Token = null);

public record Memory
{
    [JsonPropertyName("id")] public string Id { get; init; } = "";
    [JsonPropertyName("content")] public string Content { get; init; } = "";
    [JsonPropertyName("summary")] public string? Summary { get; init; }
    [JsonPropertyName("type")] public string? Type { get; init; }
    [JsonPropertyName("speaker")] public string? Speaker { get; init; }
    [JsonPropertyName("area")] public string? Area { get; init; }
    [JsonPropertyName("tags")] public string[]? Tags { get; init; }
    [JsonPropertyName("createdAt")] public string? CreatedAt { get; init; }
}

public record RememberRequest
{
    [JsonPropertyName("content")] public required string Content { get; init; }
    [JsonPropertyName("project")] public required string Project { get; init; }
    [JsonPropertyName("type")] public string? Type { get; init; }
    [JsonPropertyName("summary")] public string? Summary { get; init; }
    [JsonPropertyName("speaker")] public string? Speaker { get; init; }
    [JsonPropertyName("area")] public string? Area { get; init; }
    [JsonPropertyName("sub_area")] public string? SubArea { get; init; }
    [JsonPropertyName("tags")] public string[]? Tags { get; init; }
}

public record RecallRequest
{
    [JsonPropertyName("query")] public required string Query { get; init; }
    [JsonPropertyName("limit")] public int Limit { get; init; } = 5;
    [JsonPropertyName("project")] public string? Project { get; init; }
}

public record RecallResult
{
    [JsonPropertyName("memory")] public Memory Memory { get; init; } = new();
    [JsonPropertyName("score")] public double Score { get; init; }
}

public record RecallResponse
{
    [JsonPropertyName("results")] public RecallResult[] Results { get; init; } = [];
}

public record RememberResponse
{
    [JsonPropertyName("id")] public string Id { get; init; } = "";
    [JsonPropertyName("ok")] public bool Ok { get; init; }
}

public record HealthResponse
{
    [JsonPropertyName("ok")] public bool Ok { get; init; }
    [JsonPropertyName("version")] public string Version { get; init; } = "";
}

/// <summary>
/// MAGI client — Multi-Agent Graph Intelligence.
/// Universal memory for AI agents.
/// </summary>
public class MagiClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly string _baseUrl;

    public MagiClient(MagiConfig config)
    {
        _baseUrl = config.BaseUrl.TrimEnd('/');
        _http = new HttpClient();
        if (config.Token is not null)
            _http.DefaultRequestHeaders.Add("Authorization", $"Bearer {config.Token}");
    }

    public async Task<RememberResponse> RememberAsync(RememberRequest req)
    {
        var res = await _http.PostAsJsonAsync($"{_baseUrl}/remember", req);
        res.EnsureSuccessStatusCode();
        return await res.Content.ReadFromJsonAsync<RememberResponse>() ?? new();
    }

    public async Task<RecallResponse> RecallAsync(RecallRequest req)
    {
        var res = await _http.PostAsJsonAsync($"{_baseUrl}/recall", req);
        res.EnsureSuccessStatusCode();
        return await res.Content.ReadFromJsonAsync<RecallResponse>() ?? new();
    }

    public async Task<Memory[]> ListAsync(Dictionary<string, string>? filters = null)
    {
        var qs = filters != null ? "?" + string.Join("&", filters.Select(kv => $"{kv.Key}={kv.Value}")) : "";
        var res = await _http.GetAsync($"{_baseUrl}/memories{qs}");
        res.EnsureSuccessStatusCode();
        return await res.Content.ReadFromJsonAsync<Memory[]>() ?? [];
    }

    public async Task ForgetAsync(string id)
    {
        var res = await _http.DeleteAsync($"{_baseUrl}/memories/{id}");
        res.EnsureSuccessStatusCode();
    }

    public async Task<HealthResponse> HealthAsync()
    {
        var res = await _http.GetAsync($"{_baseUrl}/health");
        return await res.Content.ReadFromJsonAsync<HealthResponse>() ?? new();
    }

    public void Dispose() => _http.Dispose();
}
