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

export interface FileInfo {
  name: string;
  path: string;
  size: number;
  isDir: boolean;
  modTime?: string;
}

export interface PortMapping {
  sandboxName: string;
  containerPort: number;
  hostPort: number;
  protocol: string;
}

export interface InterpretResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  duration: string;
  error?: string;
  files?: any[];
}

export interface BuildResult {
  imageId: string;
  imageTag: string;
  blueprint: string;
  cached: boolean;
}
