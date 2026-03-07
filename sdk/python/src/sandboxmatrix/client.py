import subprocess
import json
import shutil
from pathlib import Path
from .models import Sandbox, ExecResult, Snapshot, Matrix, Session
from .exceptions import CLIError, SandboxNotFoundError


class SandboxMatrixClient:
    """Python client for sandboxMatrix.

    Wraps the `smx` CLI binary for programmatic sandbox management.
    """

    def __init__(self, binary: str | None = None):
        """Initialize the client.

        Args:
            binary: Path to the smx binary. If None, searches PATH.
        """
        if binary:
            self.binary = binary
        else:
            found = shutil.which("smx")
            if found:
                self.binary = found
            else:
                raise CLIError(
                    "smx binary not found in PATH. Install sandboxMatrix or pass binary path."
                )

    def _run(
        self, *args: str, check: bool = True
    ) -> subprocess.CompletedProcess:
        """Run an smx command."""
        cmd = [self.binary] + list(args)
        result = subprocess.run(cmd, capture_output=True, text=True)
        if check and result.returncode != 0:
            raise CLIError(
                f"smx {' '.join(args)} failed: {result.stderr.strip()}",
                exit_code=result.returncode,
                stderr=result.stderr,
            )
        return result

    def version(self) -> dict:
        """Get version info."""
        result = self._run("version", "--json")
        return json.loads(result.stdout)

    # Sandbox operations
    def create_sandbox(
        self, name: str, blueprint: str, workspace: str | None = None
    ) -> Sandbox:
        """Create a new sandbox.

        Args:
            name: Name for the sandbox.
            blueprint: Blueprint to use (e.g. "ubuntu:22.04").
            workspace: Optional workspace directory to mount.

        Returns:
            The created Sandbox object.
        """
        args = ["sandbox", "create", "-b", blueprint, "-n", name]
        if workspace:
            args.extend(["-w", workspace])
        self._run(*args)
        return self.get_sandbox(name)

    def get_sandbox(self, name: str) -> Sandbox:
        """Get details of a specific sandbox.

        Args:
            name: Name of the sandbox.

        Returns:
            Sandbox object with current state.

        Raises:
            CLIError: If the sandbox does not exist.
        """
        result = self._run("sandbox", "inspect", name)
        # Parse the output
        lines = result.stdout.strip().split("\n")
        data = {}
        for line in lines:
            if ":" in line:
                key, _, value = line.partition(":")
                data[key.strip().lower()] = value.strip()
        return Sandbox(
            name=data.get("name", name),
            state=data.get("state", "Unknown"),
            blueprint=data.get("blueprint", ""),
            runtime_id=data.get("runtime id", ""),
            ip=data.get("ip"),
        )

    def list_sandboxes(self) -> list[Sandbox]:
        """List all sandboxes.

        Returns:
            List of Sandbox objects.
        """
        result = self._run("sandbox", "list")
        if "No sandboxes found" in result.stdout:
            return []
        lines = result.stdout.strip().split("\n")[1:]  # skip header
        sandboxes = []
        for line in lines:
            parts = line.split()
            if len(parts) >= 4:
                sandboxes.append(
                    Sandbox(
                        name=parts[0],
                        state=parts[1],
                        blueprint=parts[2],
                        runtime_id=parts[3],
                    )
                )
        return sandboxes

    def exec(self, name: str, command: str | list[str]) -> ExecResult:
        """Execute a command inside a sandbox.

        Args:
            name: Name of the sandbox.
            command: Command string or list of arguments to execute.

        Returns:
            ExecResult with exit_code, stdout, and stderr.
        """
        if isinstance(command, str):
            cmd_args = ["sandbox", "exec", name, "--", "sh", "-c", command]
        else:
            cmd_args = ["sandbox", "exec", name, "--"] + command
        result = self._run(*cmd_args, check=False)
        return ExecResult(
            exit_code=result.returncode,
            stdout=result.stdout,
            stderr=result.stderr,
        )

    def stop_sandbox(self, name: str) -> None:
        """Stop a running sandbox.

        Args:
            name: Name of the sandbox to stop.
        """
        self._run("sandbox", "stop", name)

    def start_sandbox(self, name: str) -> None:
        """Start a stopped sandbox.

        Args:
            name: Name of the sandbox to start.
        """
        self._run("sandbox", "start", name)

    def destroy_sandbox(self, name: str) -> None:
        """Destroy a sandbox permanently.

        Args:
            name: Name of the sandbox to destroy.
        """
        self._run("sandbox", "destroy", name)

    # Snapshot operations
    def snapshot(self, name: str, tag: str | None = None) -> str:
        """Create a snapshot of a sandbox.

        Args:
            name: Name of the sandbox.
            tag: Optional tag for the snapshot.

        Returns:
            Snapshot ID string.
        """
        args = ["sandbox", "snapshot", name]
        if tag:
            args.extend(["--tag", tag])
        result = self._run(*args)
        # Parse snapshot ID from output
        for line in result.stdout.split("\n"):
            if "Snapshot created:" in line:
                return line.split(":")[-1].strip()
        return ""

    def restore(self, name: str, snapshot_id: str, new_name: str) -> Sandbox:
        """Restore a sandbox from a snapshot.

        Args:
            name: Name of the original sandbox.
            snapshot_id: ID of the snapshot to restore.
            new_name: Name for the restored sandbox.

        Returns:
            The restored Sandbox object.
        """
        self._run(
            "sandbox",
            "restore",
            name,
            "--snapshot",
            snapshot_id,
            "--name",
            new_name,
        )
        return self.get_sandbox(new_name)

    # Matrix operations
    def create_matrix(self, name: str, members: dict[str, str]) -> None:
        """Create a matrix (group of sandboxes).

        Args:
            name: Name for the matrix.
            members: Dict mapping member names to blueprints.
        """
        args = ["matrix", "create", name]
        for member_name, blueprint in members.items():
            args.extend(["--member", f"{member_name}:{blueprint}"])
        self._run(*args)

    def list_matrices(self) -> list[Matrix]:
        """List all matrices.

        Returns:
            List of Matrix objects.
        """
        result = self._run("matrix", "list")
        if "No matrices found" in result.stdout:
            return []
        lines = result.stdout.strip().split("\n")[1:]
        matrices = []
        for line in lines:
            parts = line.split()
            if len(parts) >= 3:
                matrices.append(
                    Matrix(
                        name=parts[0],
                        state=parts[1],
                        members=parts[2].split(","),
                    )
                )
        return matrices

    def destroy_matrix(self, name: str) -> None:
        """Destroy a matrix and all its member sandboxes.

        Args:
            name: Name of the matrix to destroy.
        """
        self._run("matrix", "destroy", name)

    # Session operations
    def start_session(self, sandbox_name: str) -> str:
        """Start a new session on a sandbox.

        Args:
            sandbox_name: Name of the sandbox.

        Returns:
            Session ID string.
        """
        result = self._run("session", "start", sandbox_name)
        # Parse session ID from output like: Session "id" started...
        for part in result.stdout.split('"'):
            if sandbox_name in part and "-" in part:
                return part
        return result.stdout.strip()

    def end_session(self, session_id: str) -> None:
        """End an active session.

        Args:
            session_id: ID of the session to end.
        """
        self._run("session", "end", session_id)
