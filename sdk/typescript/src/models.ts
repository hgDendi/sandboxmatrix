export interface Sandbox {
  name: string;
  state: string;
  blueprint: string;
  runtimeId: string;
  ip?: string;
  createdAt?: string;
}

export interface ExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

export interface Snapshot {
  id: string;
  tag: string;
  createdAt?: string;
  size?: number;
}

export interface Matrix {
  name: string;
  state: string;
  members: string[];
}

export interface Session {
  id: string;
  sandbox: string;
  state: string;
  execCount: number;
}

export interface VersionInfo {
  version: string;
  commit: string;
  buildDate: string;
  goVersion: string;
  os: string;
  arch: string;
}
