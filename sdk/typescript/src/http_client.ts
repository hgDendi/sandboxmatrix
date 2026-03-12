import type {
  Sandbox, ExecResult, Matrix, Session, VersionInfo,
  FileInfo, PortMapping, InterpretResult, BuildResult,
} from "./models";
import { SandboxMatrixError, SandboxNotFoundError } from "./errors";

export interface HTTPClientOptions {
  baseURL?: string;
  token?: string;
}

export class HTTPClient {
  private baseURL: string;
  private token?: string;

  constructor(options: HTTPClientOptions = {}) {
    this.baseURL = (options.baseURL ?? "http://localhost:8080").replace(/\/+$/, "");
    this.token = options.token;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const url = `${this.baseURL}/api/v1${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }

    const res = await fetch(url, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    const data = await res.json();
    if (!res.ok) {
      const msg = data?.error ?? JSON.stringify(data);
      if (res.status === 404) throw new SandboxNotFoundError(msg);
      throw new SandboxMatrixError(`API error (${res.status}): ${msg}`);
    }
    return data as T;
  }

  // Health & Version
  async health(): Promise<{ status: string }> {
    return this.request("GET", "/health");
  }

  async version(): Promise<VersionInfo> {
    return this.request("GET", "/version");
  }

  // Sandbox operations
  async createSandbox(name: string, blueprint: string, workspace?: string): Promise<Sandbox> {
    const body: Record<string, string> = { name, blueprint };
    if (workspace) body.workspace = workspace;
    const data = await this.request<any>("POST", "/sandboxes", body);
    return this.parseSandbox(data);
  }

  async getSandbox(name: string): Promise<Sandbox> {
    const data = await this.request<any>("GET", `/sandboxes/${encodeURIComponent(name)}`);
    return this.parseSandbox(data);
  }

  async listSandboxes(): Promise<Sandbox[]> {
    const data = await this.request<any[]>("GET", "/sandboxes");
    return data.map((s) => this.parseSandbox(s));
  }

  async startSandbox(name: string): Promise<void> {
    await this.request("POST", `/sandboxes/${encodeURIComponent(name)}/start`);
  }

  async stopSandbox(name: string): Promise<void> {
    await this.request("POST", `/sandboxes/${encodeURIComponent(name)}/stop`);
  }

  async destroySandbox(name: string): Promise<void> {
    await this.request("DELETE", `/sandboxes/${encodeURIComponent(name)}`);
  }

  async exec(name: string, command: string | string[]): Promise<ExecResult> {
    const cmd = typeof command === "string" ? ["sh", "-c", command] : command;
    return this.request("POST", `/sandboxes/${encodeURIComponent(name)}/exec`, { command: cmd });
  }

  async stats(name: string): Promise<Record<string, number>> {
    return this.request("GET", `/sandboxes/${encodeURIComponent(name)}/stats`);
  }

  // Snapshot operations
  async createSnapshot(name: string, tag?: string): Promise<{ snapshotId: string; tag: string }> {
    return this.request("POST", `/sandboxes/${encodeURIComponent(name)}/snapshots`, tag ? { tag } : {});
  }

  async listSnapshots(name: string): Promise<any[]> {
    return this.request("GET", `/sandboxes/${encodeURIComponent(name)}/snapshots`);
  }

  // Matrix operations
  async createMatrix(name: string, members: { name: string; blueprint: string }[]): Promise<any> {
    return this.request("POST", "/matrices", { name, members });
  }

  async getMatrix(name: string): Promise<any> {
    return this.request("GET", `/matrices/${encodeURIComponent(name)}`);
  }

  async listMatrices(): Promise<any[]> {
    return this.request("GET", "/matrices");
  }

  async startMatrix(name: string): Promise<void> {
    await this.request("POST", `/matrices/${encodeURIComponent(name)}/start`);
  }

  async stopMatrix(name: string): Promise<void> {
    await this.request("POST", `/matrices/${encodeURIComponent(name)}/stop`);
  }

  async destroyMatrix(name: string): Promise<void> {
    await this.request("DELETE", `/matrices/${encodeURIComponent(name)}`);
  }

  // Session operations
  async startSession(sandbox: string): Promise<any> {
    return this.request("POST", "/sessions", { sandbox });
  }

  async listSessions(sandbox?: string): Promise<any[]> {
    const path = sandbox ? `/sessions?sandbox=${encodeURIComponent(sandbox)}` : "/sessions";
    return this.request("GET", path);
  }

  async endSession(sessionId: string): Promise<void> {
    await this.request("POST", `/sessions/${encodeURIComponent(sessionId)}/end`);
  }

  private async requestRaw(method: string, path: string): Promise<string> {
    const url = `${this.baseURL}/api/v1${path}`;
    const headers: Record<string, string> = {};
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }

    const res = await fetch(url, { method, headers });
    if (!res.ok) {
      let msg: string;
      try {
        const data = await res.json();
        msg = data?.error ?? JSON.stringify(data);
      } catch {
        msg = await res.text();
      }
      if (res.status === 404) throw new SandboxNotFoundError(msg);
      throw new SandboxMatrixError(`API error (${res.status}): ${msg}`);
    }
    return res.text();
  }

  // File operations
  async writeFile(name: string, path: string, content: string): Promise<void> {
    const encoded = btoa(content);
    await this.request("PUT", `/sandboxes/${encodeURIComponent(name)}/files`, {
      path,
      content: encoded,
    });
  }

  async readFile(name: string, path: string): Promise<string> {
    return this.requestRaw(
      "GET",
      `/sandboxes/${encodeURIComponent(name)}/files?path=${encodeURIComponent(path)}`,
    );
  }

  async listFiles(name: string, path: string = "/"): Promise<FileInfo[]> {
    return this.request(
      "GET",
      `/sandboxes/${encodeURIComponent(name)}/files/list?path=${encodeURIComponent(path)}`,
    );
  }

  // Port operations
  async listPorts(name: string): Promise<PortMapping[]> {
    return this.request("GET", `/sandboxes/${encodeURIComponent(name)}/ports`);
  }

  // Code interpreter
  async interpret(
    name: string,
    language: string,
    code: string,
    timeout: number = 30,
  ): Promise<InterpretResult> {
    return this.request("POST", `/sandboxes/${encodeURIComponent(name)}/interpret`, {
      language,
      code,
      timeout,
    });
  }

  // Image operations
  async buildImage(blueprint: string): Promise<BuildResult> {
    return this.request("POST", "/images/build", { blueprint });
  }

  async listImages(): Promise<any[]> {
    return this.request("GET", "/images");
  }

  private parseSandbox(data: any): Sandbox {
    const metadata = data.metadata ?? {};
    const spec = data.spec ?? {};
    const status = data.status ?? {};
    return {
      name: metadata.name ?? "",
      state: status.state ?? "Unknown",
      blueprint: spec.blueprintRef ?? "",
      runtimeId: status.runtimeID ?? "",
      ip: status.ip,
      createdAt: metadata.createdAt,
    };
  }
}
