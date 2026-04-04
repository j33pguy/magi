/**
 * MAGI Client SDK
 * Multi-Agent Graph Intelligence — Universal memory for AI agents.
 */

// ── Error types ──────────────────────────────────────────────────────

export class MagiError extends Error {
  constructor(
    public readonly status: number,
    public readonly statusText: string,
    message: string,
  ) {
    super(message);
    this.name = 'MagiError';
  }
}

// ── Interfaces ───────────────────────────────────────────────────────

export interface MagiConfig {
  baseUrl: string;
  token?: string;
  /** Max retries on 5xx / network errors (default: 3, 0 = no retries). */
  maxRetries?: number;
  /** Initial backoff in ms (default: 200). Doubles each attempt. */
  retryBaseMs?: number;
}

export interface Memory {
  id: string;
  content: string;
  summary?: string;
  project?: string;
  type?: string;
  visibility?: string;
  source?: string;
  sourceFile?: string;
  parentId?: string;
  chunkIndex?: number;
  speaker?: string;
  area?: string;
  subArea?: string;
  createdAt?: string;
  updatedAt?: string;
  archivedAt?: string;
  tokenCount?: number;
  tags?: string[];
}

export interface RememberOptions {
  content: string;
  project: string;
  type?: string;
  summary?: string;
  speaker?: string;
  area?: string;
  sub_area?: string;
  tags?: string[];
}

export interface RecallOptions {
  query: string;
  limit?: number;
  project?: string;
  type?: string;
}

export interface RecallResult {
  memory: Memory;
  score: number;
}

export interface SearchParams {
  top_k?: number;
  recency_decay?: number;
  tags?: string;
  project?: string;
  type?: string;
}

export interface SearchResult {
  memory: Memory;
  rrfScore: number;
  vecRank: number;
  bm25Rank: number;
  distance: number;
  score: number;
  recencyWeight?: number;
  weightedScore?: number;
}

export interface Conversation {
  channel: string;
  session_key?: string;
  started_at?: string;
  ended_at?: string;
  turn_count?: number;
  topics?: string[];
  summary: string;
  decisions?: string[];
  action_items?: string[];
}

export interface ConversationSearchParams {
  query: string;
  limit?: number;
  channel?: string;
  min_relevance?: number;
  recency_decay?: number;
}

export interface ConversationSearchResult {
  results: SearchResult[];
  rewritten: boolean;
  rewritten_query?: string;
  attempts: number;
}

export interface Task {
  id: string;
  project?: string;
  queue?: string;
  title: string;
  summary?: string;
  description?: string;
  status?: string;
  priority?: string;
  created_by?: string;
  orchestrator?: string;
  worker?: string;
  parent_task_id?: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
  started_at?: string;
  completed_at?: string;
  failed_at?: string;
  blocked_at?: string;
}

export interface CreateTaskParams {
  title: string;
  project?: string;
  queue?: string;
  summary?: string;
  description?: string;
  status?: string;
  priority?: string;
  created_by?: string;
  orchestrator?: string;
  worker?: string;
  parent_task_id?: string;
  metadata?: Record<string, unknown>;
  actor_role?: string;
  actor_name?: string;
}

export interface UpdateTaskParams {
  project?: string;
  queue?: string;
  title?: string;
  summary?: string;
  description?: string;
  status?: string;
  priority?: string;
  created_by?: string;
  orchestrator?: string;
  worker?: string;
  parent_task_id?: string;
  metadata?: Record<string, unknown>;
  status_comment?: string;
  actor_role?: string;
  actor_name?: string;
}

export interface ListTasksParams {
  project?: string;
  queue?: string;
  status?: string;
  worker?: string;
  orchestrator?: string;
  limit?: number;
}

export interface TaskEvent {
  id: string;
  task_id: string;
  event_type: string;
  actor_role?: string;
  actor_name?: string;
  actor_user?: string;
  actor_machine?: string;
  actor_agent?: string;
  summary?: string;
  content?: string;
  status?: string;
  memory_id?: string;
  source?: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
}

export interface CreateTaskEventParams {
  event_type: string;
  actor_role?: string;
  actor_name?: string;
  actor_user?: string;
  actor_machine?: string;
  actor_agent?: string;
  summary?: string;
  content?: string;
  status?: string;
  memory_id?: string;
  source?: string;
  metadata?: Record<string, unknown>;
}

export interface MemoryLink {
  id: string;
  fromId: string;
  toId: string;
  relation: string;
  weight: number;
  auto: boolean;
  createdAt: string;
}

export interface RelatedMemory {
  memory: Memory;
  links: MemoryLink[];
}

export interface HistoryEntry {
  hash: string;
  author: string;
  timestamp: string;
  message: string;
}

export interface MemoryHistoryResult {
  id: string;
  entries: HistoryEntry[];
}

export interface MemoryDiffResult {
  id: string;
  from_commit: string;
  to_commit: string;
  from_content: string;
  to_content: string;
  diff: string;
}

export interface PipelineStats {
  queue_depth: number;
  batch_pending: number;
  workers: number;
  submitted: number;
  completed: number;
  failed: number;
}

// ── Client ───────────────────────────────────────────────────────────

export class Magi {
  private baseUrl: string;
  private headers: Record<string, string>;
  private maxRetries: number;
  private retryBaseMs: number;

  constructor(config: MagiConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
    this.headers = {
      'Content-Type': 'application/json',
      ...(config.token ? { Authorization: `Bearer ${config.token}` } : {}),
    };
    this.maxRetries = config.maxRetries ?? 3;
    this.retryBaseMs = config.retryBaseMs ?? 200;
  }

  // ── Internal helpers ───────────────────────────────────────────────

  private async request<T>(
    method: string,
    path: string,
    opts?: { body?: unknown; headers?: Record<string, string> },
  ): Promise<T> {
    let lastError: Error | undefined;
    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      if (attempt > 0) {
        const delay = this.retryBaseMs * 2 ** (attempt - 1);
        await new Promise((r) => setTimeout(r, delay));
      }
      try {
        const res = await fetch(`${this.baseUrl}${path}`, {
          method,
          headers: opts?.headers ?? this.headers,
          body: opts?.body !== undefined ? JSON.stringify(opts.body) : undefined,
        });
        if (res.status >= 500 && attempt < this.maxRetries) {
          lastError = new MagiError(res.status, res.statusText, await res.text());
          continue;
        }
        if (!res.ok) {
          throw new MagiError(res.status, res.statusText, await res.text());
        }
        const text = await res.text();
        return text ? (JSON.parse(text) as T) : (undefined as unknown as T);
      } catch (err) {
        if (err instanceof MagiError) throw err;
        if (attempt < this.maxRetries) {
          lastError = err as Error;
          continue;
        }
        throw err;
      }
    }
    throw lastError;
  }

  private qs(params?: Record<string, string | number | boolean | undefined>): string {
    if (!params) return '';
    const entries = Object.entries(params).filter(
      (e): e is [string, string | number | boolean] => e[1] !== undefined,
    );
    if (entries.length === 0) return '';
    return '?' + new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString();
  }

  // ── Memory CRUD ────────────────────────────────────────────────────

  async remember(opts: RememberOptions): Promise<{ id: string; ok: boolean }> {
    return this.request('POST', '/remember', { body: opts });
  }

  async recall(opts: RecallOptions): Promise<{ results: RecallResult[] }> {
    return this.request('POST', '/recall', { body: opts });
  }

  async list(params?: Record<string, string>): Promise<Memory[]> {
    return this.request('GET', `/memories${this.qs(params)}`);
  }

  async forget(id: string): Promise<void> {
    return this.request('DELETE', `/memories/${id}`);
  }

  async update(id: string, patch: Partial<Memory>): Promise<{ id: string; status: string }> {
    return this.request('PATCH', `/memories/${id}`, { body: patch });
  }

  // ── Search ─────────────────────────────────────────────────────────

  async search(query: string, params?: SearchParams): Promise<SearchResult[]> {
    return this.request('GET', `/search${this.qs({ q: query, ...params })}`);
  }

  // ── Knowledge graph ────────────────────────────────────────────────

  async link(
    fromId: string,
    toId: string,
    relation: string,
    weight = 1.0,
  ): Promise<{ id: string; ok: boolean }> {
    return this.request('POST', '/links', {
      body: { from_id: fromId, to_id: toId, relation, weight },
    });
  }

  async unlink(linkId: string): Promise<void> {
    return this.request('DELETE', `/links/${linkId}`);
  }

  async getRelated(memoryId: string): Promise<RelatedMemory[]> {
    return this.request('GET', `/memories/${memoryId}/related`);
  }

  // ── Conversations ──────────────────────────────────────────────────

  async storeConversation(data: Conversation): Promise<{ id: string; ok: boolean }> {
    return this.request('POST', '/conversations', { body: data });
  }

  async listConversations(params?: {
    limit?: number;
    channel?: string;
    since?: string;
  }): Promise<Memory[]> {
    return this.request('GET', `/conversations${this.qs(params)}`);
  }

  async searchConversations(data: ConversationSearchParams): Promise<ConversationSearchResult> {
    return this.request('POST', '/conversations/search', { body: data });
  }

  async getConversation(id: string): Promise<Memory> {
    return this.request('GET', `/conversations/${id}`);
  }

  // ── Tasks ──────────────────────────────────────────────────────────

  async createTask(data: CreateTaskParams): Promise<Task> {
    return this.request('POST', '/tasks', { body: data });
  }

  async listTasks(params?: ListTasksParams): Promise<Task[]> {
    return this.request('GET', `/tasks${this.qs(params as Record<string, string | number | undefined>)}`);
  }

  async getTask(id: string): Promise<Task> {
    return this.request('GET', `/tasks/${id}`);
  }

  async updateTask(id: string, patch: UpdateTaskParams): Promise<Task> {
    return this.request('PATCH', `/tasks/${id}`, { body: patch });
  }

  async createTaskEvent(taskId: string, data: CreateTaskEventParams): Promise<TaskEvent> {
    return this.request('POST', `/tasks/${taskId}/events`, { body: data });
  }

  async listTaskEvents(taskId: string, limit?: number): Promise<TaskEvent[]> {
    return this.request('GET', `/tasks/${taskId}/events${this.qs({ limit })}`);
  }

  // ── History / diff ─────────────────────────────────────────────────

  async memoryHistory(id: string): Promise<MemoryHistoryResult> {
    return this.request('GET', `/memory/${id}/history`);
  }

  async memoryDiff(id: string, from: string, to: string): Promise<MemoryDiffResult> {
    return this.request('GET', `/memory/${id}/diff${this.qs({ from, to })}`);
  }

  // ── Pipeline ───────────────────────────────────────────────────────

  async pipelineStats(): Promise<PipelineStats> {
    return this.request('GET', '/pipeline/stats');
  }

  // ── Health ─────────────────────────────────────────────────────────

  async health(): Promise<{ ok: boolean; version: string }> {
    return this.request('GET', '/health');
  }
}

export default Magi;
