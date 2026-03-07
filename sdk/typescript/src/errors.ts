export class SandboxMatrixError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SandboxMatrixError";
  }
}

export class CLIError extends SandboxMatrixError {
  public readonly exitCode: number;
  public readonly stderr: string;

  constructor(message: string, exitCode: number, stderr: string) {
    super(message);
    this.name = "CLIError";
    this.exitCode = exitCode;
    this.stderr = stderr;
  }
}

export class SandboxNotFoundError extends SandboxMatrixError {
  constructor(name: string) {
    super(`Sandbox "${name}" not found`);
    this.name = "SandboxNotFoundError";
  }
}
