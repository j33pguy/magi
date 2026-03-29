/**
 * MAGI Client SDK
 * Multi-Agent Graph Intelligence — Universal memory for AI agents.
 */

export interface MagiConfig {
  baseUrl: string;
  token?: string;
}

export interface Memory {
  id: string;
  content: string;
  summary?: string;
  type?: string;
  speaker?: string;
  area?: string;
  subArea?: string;
  tags?: string[];
  createdAt?: string;
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

export class Magi {
  private baseUrl: string;
  private headers: Record<string, string>;

  constructor(config: MagiConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
    this.headers = {
      'Content-Type': 'application/json',
      ...(config.token ? { Authorization: `Bearer ${config.token}` } : {}),
    };
  }

  async remember(opts: RememberOptions): Promise<{ id: string; ok: boolean }> {
    const res = await fetch(`${this.baseUrl}/remember`, {
      method: 'POST', headers: this.headers, body: JSON.stringify(opts),
    });
    if (!res.ok) throw new Error(`MAGI remember failed: ${res.status}`);
    return res.json();
  }

  async recall(opts: RecallOptions): Promise<{ results: RecallResult[] }> {
    const res = await fetch(`${this.baseUrl}/recall`, {
      method: 'POST', headers: this.headers, body: JSON.stringify(opts),
    });
    if (!res.ok) throw new Error(`MAGI recall failed: ${res.status}`);
    return res.json();
  }

  async list(params?: Record<string, string>): Promise<Memory[]> {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    const res = await fetch(`${this.baseUrl}/memories${qs}`, { headers: this.headers });
    if (!res.ok) throw new Error(`MAGI list failed: ${res.status}`);
    return res.json();
  }

  async forget(id: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/memories/${id}`, {
      method: 'DELETE', headers: this.headers,
    });
    if (!res.ok) throw new Error(`MAGI forget failed: ${res.status}`);
  }

  async update(id: string, patch: Partial<Memory>): Promise<{ id: string; status: string }> {
    const res = await fetch(`${this.baseUrl}/memories/${id}`, {
      method: 'PATCH', headers: this.headers, body: JSON.stringify(patch),
    });
    if (!res.ok) throw new Error(`MAGI update failed: ${res.status}`);
    return res.json();
  }

  async link(fromId: string, toId: string, relation: string, weight = 1.0): Promise<any> {
    const res = await fetch(`${this.baseUrl}/links`, {
      method: 'POST', headers: this.headers,
      body: JSON.stringify({ from_id: fromId, to_id: toId, relation, weight }),
    });
    if (!res.ok) throw new Error(`MAGI link failed: ${res.status}`);
    return res.json();
  }

  async health(): Promise<{ ok: boolean; version: string }> {
    const res = await fetch(`${this.baseUrl}/health`);
    return res.json();
  }
}

export default Magi;
