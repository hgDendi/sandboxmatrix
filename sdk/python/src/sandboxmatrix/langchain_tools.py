"""LangChain tool adapters for sandboxMatrix.

Provides LangChain-compatible tools for sandbox operations including
shell execution, file I/O, and multi-language code interpretation.

Usage::

    from sandboxmatrix.langchain_tools import create_sandbox_tools

    tools = create_sandbox_tools(sandbox_name="my-sandbox")
    # Use with any LangChain agent
    agent = create_react_agent(llm, tools)

Requires the ``langchain`` extra::

    pip install sandboxmatrix[langchain]
"""

from typing import Optional, Type

try:
    from langchain_core.tools import BaseTool
    from pydantic import BaseModel, Field, ConfigDict
except ImportError:
    raise ImportError(
        "langchain_core and pydantic are required for LangChain integration. "
        "Install with: pip install sandboxmatrix[langchain]"
    )

from .http_client import HTTPClient


# ---------------------------------------------------------------------------
# Input schemas
# ---------------------------------------------------------------------------

class SandboxExecInput(BaseModel):
    """Input for sandbox shell execution."""
    command: str = Field(description="Shell command to execute in the sandbox")


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

class SandboxExecTool(BaseTool):
    """Execute a shell command inside an isolated sandbox."""

    name: str = "sandbox_exec"
    description: str = (
        "Execute a shell command in an isolated sandbox environment. "
        "Returns stdout, stderr, and exit code."
    )
    args_schema: Type[BaseModel] = SandboxExecInput

    client: HTTPClient
    sandbox_name: str

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, command: str) -> str:
        result = self.client.exec(self.sandbox_name, command)
        output = result.stdout
        if result.stderr:
            output += f"\n[stderr]\n{result.stderr}"
        if result.exit_code != 0:
            output += f"\n[exit code: {result.exit_code}]"
        return output


class SandboxWriteFileTool(BaseTool):
    """Write content to a file in the sandbox."""

    name: str = "sandbox_write_file"
    description: str = "Write content to a file in the sandbox."
    args_schema: Type[BaseModel] = SandboxWriteFileInput

    client: HTTPClient
    sandbox_name: str

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, path: str, content: str) -> str:
        self.client.write_file(self.sandbox_name, path, content)
        return f"File written to {path}"


class SandboxReadFileTool(BaseTool):
    """Read content from a file in the sandbox."""

    name: str = "sandbox_read_file"
    description: str = "Read content from a file in the sandbox."
    args_schema: Type[BaseModel] = SandboxReadFileInput

    client: HTTPClient
    sandbox_name: str

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, path: str) -> str:
        data = self.client.read_file(self.sandbox_name, path)
        return data.decode("utf-8", errors="replace")


class CodeInterpreterTool(BaseTool):
    """Execute code in the sandbox and get the output."""

    name: str = "code_interpreter"
    description: str = (
        "Execute code in the sandbox and get the output. "
        "Supports Python, JavaScript, Bash, Go, and Rust."
    )
    args_schema: Type[BaseModel] = CodeInterpreterInput

    client: HTTPClient
    sandbox_name: str

    model_config = ConfigDict(arbitrary_types_allowed=True)

    def _run(self, language: str, code: str) -> str:
        result = self.client.interpret(self.sandbox_name, language, code)
        output = result.stdout
        if result.stderr:
            output += f"\n[stderr]\n{result.stderr}"
        if result.exit_code != 0:
            output += f"\n[exit code: {result.exit_code}]"
        return output


# ---------------------------------------------------------------------------
# Factory
# ---------------------------------------------------------------------------

def create_sandbox_tools(
    sandbox_name: str,
    base_url: str = "http://localhost:8080",
    token: Optional[str] = None,
) -> list[BaseTool]:
    """Create a list of LangChain tools for sandbox operations.

    Args:
        sandbox_name: Name of the sandbox to operate on (must already exist).
        base_url: sandboxMatrix API server URL.
        token: Optional Bearer token for authentication.

    Returns:
        List of LangChain tools ready for use with any agent.
    """
    client = HTTPClient(base_url=base_url, token=token)
    return [
        SandboxExecTool(client=client, sandbox_name=sandbox_name),
        SandboxWriteFileTool(client=client, sandbox_name=sandbox_name),
        SandboxReadFileTool(client=client, sandbox_name=sandbox_name),
        CodeInterpreterTool(client=client, sandbox_name=sandbox_name),
    ]
