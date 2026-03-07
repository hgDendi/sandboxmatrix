import { execFileSync } from "node:child_process";
import type { Sandbox, ExecResult, Matrix, VersionInfo } from "./models";
import { CLIError } from "./errors";

export interface ClientOptions {
  binary?: string;
}

export class SandboxMatrixClient {
  private binary: string;

  constructor(options: ClientOptions = {}) {
    this.binary = options.binary ?? this.findBinary();
  }

  private findBinary(): string {
    // Check common locations
    const candidates = ["smx", "/usr/local/bin/smx"];
    for (const candidate of candidates) {
      try {
        execFileSync(candidate, ["version"], { stdio: "pipe" });
        return candidate;
      } catch {
        continue;
      }
    }
    throw new CLIError("smx binary not found", -1, "");
  }

  private run(...args: string[]): { stdout: string; stderr: string } {
    try {
      const stdout = execFileSync(this.binary, args, {
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
      });
      return { stdout, stderr: "" };
    } catch (error: any) {
      if (error.status !== undefined) {
        throw new CLIError(
          `smx ${args.join(" ")} failed: ${error.stderr?.toString() ?? ""}`,
          error.status,
          error.stderr?.toString() ?? ""
        );
      }
      throw error;
    }
  }

  private runUnchecked(...args: string[]): { stdout: string; stderr: string; exitCode: number } {
    try {
      const stdout = execFileSync(this.binary, args, {
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
      });
      return { stdout, stderr: "", exitCode: 0 };
    } catch (error: any) {
      return {
        stdout: error.stdout?.toString() ?? "",
        stderr: error.stderr?.toString() ?? "",
        exitCode: error.status ?? -1,
      };
    }
  }

  // Version
  version(): VersionInfo {
    const { stdout } = this.run("version", "--json");
    return JSON.parse(stdout);
  }

  // Sandbox operations
  createSandbox(name: string, blueprint: string, workspace?: string): Sandbox {
    const args = ["sandbox", "create", "-b", blueprint, "-n", name];
    if (workspace) args.push("-w", workspace);
    this.run(...args);
    return this.getSandbox(name);
  }

  getSandbox(name: string): Sandbox {
    const { stdout } = this.run("sandbox", "inspect", name);
    const data: Record<string, string> = {};
    for (const line of stdout.trim().split("\n")) {
      const idx = line.indexOf(":");
      if (idx > 0) {
        data[line.slice(0, idx).trim().toLowerCase()] = line.slice(idx + 1).trim();
      }
    }
    return {
      name: data["name"] ?? name,
      state: data["state"] ?? "Unknown",
      blueprint: data["blueprint"] ?? "",
      runtimeId: data["runtime id"] ?? "",
      ip: data["ip"],
      createdAt: data["created"],
    };
  }

  listSandboxes(): Sandbox[] {
    const { stdout } = this.run("sandbox", "list");
    if (stdout.includes("No sandboxes found")) return [];
    const lines = stdout.trim().split("\n").slice(1);
    return lines.map((line) => {
      const parts = line.trim().split(/\s{2,}/);
      return {
        name: parts[0] ?? "",
        state: parts[1] ?? "",
        blueprint: parts[2] ?? "",
        runtimeId: parts[3] ?? "",
      };
    });
  }

  exec(name: string, command: string | string[]): ExecResult {
    const cmdArgs = typeof command === "string"
      ? ["sandbox", "exec", name, "--", "sh", "-c", command]
      : ["sandbox", "exec", name, "--", ...command];
    const result = this.runUnchecked(...cmdArgs);
    return {
      exitCode: result.exitCode,
      stdout: result.stdout,
      stderr: result.stderr,
    };
  }

  stopSandbox(name: string): void {
    this.run("sandbox", "stop", name);
  }

  startSandbox(name: string): void {
    this.run("sandbox", "start", name);
  }

  destroySandbox(name: string): void {
    this.run("sandbox", "destroy", name);
  }

  // Snapshot
  snapshot(name: string, tag?: string): string {
    const args = ["sandbox", "snapshot", name];
    if (tag) args.push("--tag", tag);
    const { stdout } = this.run(...args);
    const match = stdout.match(/Snapshot created: (.+)/);
    return match?.[1]?.trim() ?? "";
  }

  restore(name: string, snapshotId: string, newName: string): Sandbox {
    this.run("sandbox", "restore", name, "--snapshot", snapshotId, "--name", newName);
    return this.getSandbox(newName);
  }

  // Matrix
  createMatrix(name: string, members: Record<string, string>): void {
    const args = ["matrix", "create", name];
    for (const [memberName, blueprint] of Object.entries(members)) {
      args.push("--member", `${memberName}:${blueprint}`);
    }
    this.run(...args);
  }

  listMatrices(): Matrix[] {
    const { stdout } = this.run("matrix", "list");
    if (stdout.includes("No matrices found")) return [];
    const lines = stdout.trim().split("\n").slice(1);
    return lines.map((line) => {
      const parts = line.trim().split(/\s{2,}/);
      return {
        name: parts[0] ?? "",
        state: parts[1] ?? "",
        members: (parts[2] ?? "").split(","),
      };
    });
  }

  destroyMatrix(name: string): void {
    this.run("matrix", "destroy", name);
  }

  // Session
  startSession(sandboxName: string): string {
    const { stdout } = this.run("session", "start", sandboxName);
    const match = stdout.match(/"([^"]+)"/);
    return match?.[1] ?? stdout.trim();
  }

  endSession(sessionId: string): void {
    this.run("session", "end", sessionId);
  }
}
