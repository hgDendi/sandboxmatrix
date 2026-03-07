class SandboxMatrixError(Exception):
    """Base exception for sandboxMatrix SDK."""


class SandboxNotFoundError(SandboxMatrixError):
    """Sandbox does not exist."""


class SandboxNotRunningError(SandboxMatrixError):
    """Sandbox is not in running state."""


class BlueprintError(SandboxMatrixError):
    """Blueprint validation or parsing error."""


class CLIError(SandboxMatrixError):
    """Error executing the smx CLI binary."""

    def __init__(self, message: str, exit_code: int = -1, stderr: str = ""):
        super().__init__(message)
        self.exit_code = exit_code
        self.stderr = stderr
