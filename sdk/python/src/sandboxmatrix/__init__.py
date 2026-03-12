"""sandboxMatrix Python SDK - AI sandbox orchestrator client."""

from .client import SandboxMatrixClient
from .http_client import HTTPClient
from .models import (
    Sandbox, ExecResult, Snapshot, Matrix, Session,
    FileInfo, PortMapping, InterpretResult, BuildResult,
)
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
    "FileInfo",
    "PortMapping",
    "InterpretResult",
    "BuildResult",
    "SandboxMatrixError",
    "SandboxNotFoundError",
    "SandboxNotRunningError",
    "BlueprintError",
    "CLIError",
]

# AI framework integrations (optional)
try:
    from .langchain_tools import create_sandbox_tools
    __all__.append("create_sandbox_tools")
except ImportError:
    pass
