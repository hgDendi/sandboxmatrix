"""sandboxMatrix Python SDK - AI sandbox orchestrator client."""

from .client import SandboxMatrixClient
from .models import Sandbox, ExecResult, Snapshot, Matrix, Session
from .exceptions import (
    SandboxMatrixError,
    SandboxNotFoundError,
    SandboxNotRunningError,
    BlueprintError,
    CLIError,
)

__version__ = "0.1.0"
__all__ = [
    "SandboxMatrixClient",
    "Sandbox",
    "ExecResult",
    "Snapshot",
    "Matrix",
    "Session",
    "SandboxMatrixError",
    "SandboxNotFoundError",
    "SandboxNotRunningError",
    "BlueprintError",
    "CLIError",
]
