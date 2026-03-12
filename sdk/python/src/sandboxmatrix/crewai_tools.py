"""CrewAI tool adapters for sandboxMatrix.

Provides CrewAI-compatible tools for sandbox operations including
shell execution, file I/O, and multi-language code interpretation.

Usage::

    from sandboxmatrix.crewai_tools import SandboxExecTool, CodeInterpreterTool

    exec_tool = SandboxExecTool(sandbox_name="my-sandbox")
    interp_tool = CodeInterpreterTool(sandbox_name="my-sandbox")
    agent = Agent(tools=[exec_tool, interp_tool], ...)

Requires the ``crewai`` extra::

    pip install sandboxmatrix[crewai]
"""

from typing import Optional, Type

try:
    from crewai.tools import BaseTool as CrewAIBaseTool
    from pydantic import BaseModel, Field, ConfigDict
except ImportError:
    raise ImportError(
        "crewai is required for CrewAI integration. "
        "Install with: pip install sandboxmatrix[crewai]"
    )

from .http_client import HTTPClient


# ---------------------------------------------------------------------------
# Input schemas
# ---------------------------------------------------------------------------

class SandboxExecInput(BaseModel):
    """Input for sandbox shell execution."""
    command: str = Field(description="Shell command to execute")


class SandboxWriteFileInput(BaseModel):
    """Input for writing a file in the sandbox."""
    path: str = Field(description="Absolute file path inside the sandbox (e.g., /workspace/main.py)")
    content: str = Field(description="File content to write")


class SandboxReadFileInput(BaseModel):
    """Input for reading a file from the sandbox."""
    path: str = Field(description="Absolute file path inside the sandbox to read")


class CodeInterpreterInput(BaseModel):
    """Input for multi-language code execution."""
    language: str = Field(description="Programming language: python, javascript, bash, go, rust")
    code: str = Field(description="Code to execute")


# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------

class SandboxExecTool(CrewAIBaseTool):
    """Execute a shell command in an isolated sandbox environment."""

    name: str = "Sandbox Execute"
    description: str = (
        "Execute a shell command in an isolated sandbox environment. "
        "Returns stdout, stderr, and exit code."
    )
    args_schema: Type[BaseModel] = SandboxExecInput

    sandbox_name: str
    base_url: str = "http://localhost:8080"
    token: Optional[str] = None

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, command: str) -> str:
        client = HTTPClient(base_url=self.base_url, token=self.token)
        result = client.exec(self.sandbox_name, command)
        output = result.stdout
        if result.stderr:
            output += f"\n[stderr]\n{result.stderr}"
        if result.exit_code != 0:
            output += f"\n[exit code: {result.exit_code}]"
        return output


class SandboxWriteFileTool(CrewAIBaseTool):
    """Write content to a file in the sandbox."""

    name: str = "Sandbox Write File"
    description: str = "Write content to a file in the sandbox."
    args_schema: Type[BaseModel] = SandboxWriteFileInput

    sandbox_name: str
    base_url: str = "http://localhost:8080"
    token: Optional[str] = None

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, path: str, content: str) -> str:
        client = HTTPClient(base_url=self.base_url, token=self.token)
        client.write_file(self.sandbox_name, path, content)
        return f"File written to {path}"


class SandboxReadFileTool(CrewAIBaseTool):
    """Read content from a file in the sandbox."""

    name: str = "Sandbox Read File"
    description: str = "Read content from a file in the sandbox."
    args_schema: Type[BaseModel] = SandboxReadFileInput

    sandbox_name: str
    base_url: str = "http://localhost:8080"
    token: Optional[str] = None

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, path: str) -> str:
        client = HTTPClient(base_url=self.base_url, token=self.token)
        data = client.read_file(self.sandbox_name, path)
        return data.decode("utf-8", errors="replace")


class CodeInterpreterTool(CrewAIBaseTool):
    """Execute code in an isolated sandbox with multi-language support."""

    name: str = "Code Interpreter"
    description: str = (
        "Execute code in an isolated sandbox. "
        "Supports Python, JavaScript, Bash, Go, and Rust."
    )
    args_schema: Type[BaseModel] = CodeInterpreterInput

    sandbox_name: str
    base_url: str = "http://localhost:8080"
    token: Optional[str] = None

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, language: str, code: str) -> str:
        client = HTTPClient(base_url=self.base_url, token=self.token)
        result = client.interpret(self.sandbox_name, language, code)
        output = result.stdout
        if result.stderr:
            output += f"\n[stderr]\n{result.stderr}"
        if result.exit_code != 0:
            output += f"\n[exit code: {result.exit_code}]"
        return output
