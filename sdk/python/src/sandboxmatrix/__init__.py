"""sandboxMatrix Python SDK - AI sandbox orchestrator client."""

from .client import SandboxMatrixClient
from .http_client import HTTPClient
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
    "HTTPClient",
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
