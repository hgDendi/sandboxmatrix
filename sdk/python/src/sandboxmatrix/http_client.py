"""HTTP client for sandboxMatrix REST API."""

import json
from urllib.request import Request, urlopen
from urllib.error import HTTPError
from .models import Sandbox, ExecResult, Matrix, Session
from .exceptions import SandboxMatrixError, SandboxNotFoundError


class HTTPClient:
    """HTTP client for sandboxMatrix REST API server.

    Connects directly to the REST API server instead of wrapping the CLI binary.
    Start the server with: smx server start --addr :8080

    Args:
        base_url: Base URL of the API server (default: http://localhost:8080)
        token: Optional Bearer token for RBAC authentication
    """

    def __init__(self, base_url: str = "http://localhost:8080", token: str | None = None):
        self.base_url = base_url.rstrip("/")
        self.token = token

    def _request(self, method: str, path: str, body: dict | None = None) -> dict | list:
        url = f"{self.base_url}/api/v1{path}"
        data = json.dumps(body).encode() if body else None
        req = Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", f"Bearer {self.token}")

        try:
            with urlopen(req) as resp:
                return json.loads(resp.read().decode())
        except HTTPError as e:
            body_text = e.read().decode() if e.fp else ""
            try:
                err_data = json.loads(body_text)
                msg = err_data.get("error", body_text)
            except json.JSONDecodeError:
                msg = body_text
            if e.code == 404:
                raise SandboxNotFoundError(msg) from e
            raise SandboxMatrixError(f"API error ({e.code}): {msg}") from e

    def health(self) -> dict:
        return self._request("GET", "/health")

    def version(self) -> dict:
        return self._request("GET", "/version")

    # Sandbox operations
    def create_sandbox(self, name: str, blueprint: str, workspace: str | None = None) -> Sandbox:
        body = {"name": name, "blueprint": blueprint}
        if workspace:
            body["workspace"] = workspace
        data = self._request("POST", "/sandboxes", body)
        return self._parse_sandbox(data)

    def get_sandbox(self, name: str) -> Sandbox:
        data = self._request("GET", f"/sandboxes/{name}")
        return self._parse_sandbox(data)

    def list_sandboxes(self) -> list[Sandbox]:
        data = self._request("GET", "/sandboxes")
        return [self._parse_sandbox(s) for s in data]

    def start_sandbox(self, name: str) -> None:
        self._request("POST", f"/sandboxes/{name}/start")

    def stop_sandbox(self, name: str) -> None:
        self._request("POST", f"/sandboxes/{name}/stop")

    def destroy_sandbox(self, name: str) -> None:
        self._request("DELETE", f"/sandboxes/{name}")

    def exec(self, name: str, command: str | list[str]) -> ExecResult:
        if isinstance(command, str):
            cmd = ["sh", "-c", command]
        else:
            cmd = command
        data = self._request("POST", f"/sandboxes/{name}/exec", {"command": cmd})
        return ExecResult(
            exit_code=data.get("exitCode", -1),
            stdout=data.get("stdout", ""),
            stderr=data.get("stderr", ""),
        )

    def stats(self, name: str) -> dict:
        return self._request("GET", f"/sandboxes/{name}/stats")

    # Snapshot operations
    def create_snapshot(self, name: str, tag: str | None = None) -> dict:
        body = {}
        if tag:
            body["tag"] = tag
        return self._request("POST", f"/sandboxes/{name}/snapshots", body)

    def list_snapshots(self, name: str) -> list:
        return self._request("GET", f"/sandboxes/{name}/snapshots")

    # Matrix operations
    def create_matrix(self, name: str, members: list[dict]) -> dict:
        return self._request("POST", "/matrices", {"name": name, "members": members})

    def get_matrix(self, name: str) -> dict:
        return self._request("GET", f"/matrices/{name}")

    def list_matrices(self) -> list:
        return self._request("GET", "/matrices")

    def start_matrix(self, name: str) -> None:
        self._request("POST", f"/matrices/{name}/start")

    def stop_matrix(self, name: str) -> None:
        self._request("POST", f"/matrices/{name}/stop")

    def destroy_matrix(self, name: str) -> None:
        self._request("DELETE", f"/matrices/{name}")

    # Session operations
    def start_session(self, sandbox: str) -> dict:
        return self._request("POST", "/sessions", {"sandbox": sandbox})

    def list_sessions(self, sandbox: str | None = None) -> list:
        path = "/sessions"
        if sandbox:
            path += f"?sandbox={sandbox}"
        return self._request("GET", path)

    def end_session(self, session_id: str) -> None:
        self._request("POST", f"/sessions/{session_id}/end")

    @staticmethod
    def _parse_sandbox(data: dict) -> Sandbox:
        metadata = data.get("metadata", {})
        spec = data.get("spec", {})
        status = data.get("status", {})
        return Sandbox(
            name=metadata.get("name", ""),
            state=status.get("state", "Unknown"),
            blueprint=spec.get("blueprintRef", ""),
            runtime_id=status.get("runtimeID", ""),
            ip=status.get("ip"),
        )
