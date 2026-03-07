"""Unit tests for the sandboxMatrix Python SDK.

All tests mock subprocess.run so they do not require the smx binary or Docker.
"""

import subprocess
import unittest
from unittest.mock import patch, MagicMock
import sys
import os

# Ensure the SDK source is importable
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from sandboxmatrix import SandboxMatrixClient, CLIError, Sandbox, ExecResult


def _make_result(stdout: str = "", stderr: str = "", returncode: int = 0):
    """Helper to build a mock CompletedProcess."""
    return subprocess.CompletedProcess(
        args=[], returncode=returncode, stdout=stdout, stderr=stderr
    )


class TestClientInit(unittest.TestCase):
    """Test client initialization."""

    @patch("shutil.which", return_value="/usr/local/bin/smx")
    def test_auto_detect_binary(self, mock_which):
        client = SandboxMatrixClient()
        self.assertEqual(client.binary, "/usr/local/bin/smx")

    def test_explicit_binary(self):
        client = SandboxMatrixClient(binary="/opt/smx")
        self.assertEqual(client.binary, "/opt/smx")

    @patch("shutil.which", return_value=None)
    def test_missing_binary_raises(self, mock_which):
        with self.assertRaises(CLIError) as ctx:
            SandboxMatrixClient()
        self.assertIn("not found", str(ctx.exception))


class TestVersion(unittest.TestCase):
    """Test version command."""

    @patch("subprocess.run")
    def test_version(self, mock_run):
        mock_run.return_value = _make_result(
            stdout='{"version": "0.1.0", "commit": "abc123"}'
        )
        client = SandboxMatrixClient(binary="/usr/bin/smx")
        info = client.version()
        self.assertEqual(info["version"], "0.1.0")
        self.assertEqual(info["commit"], "abc123")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "version", "--json"],
            capture_output=True,
            text=True,
        )


class TestSandboxOperations(unittest.TestCase):
    """Test sandbox CRUD operations."""

    def setUp(self):
        self.client = SandboxMatrixClient(binary="/usr/bin/smx")

    @patch("subprocess.run")
    def test_create_sandbox(self, mock_run):
        # First call: create, second call: inspect
        mock_run.side_effect = [
            _make_result(stdout="Sandbox 'dev' created.\n"),
            _make_result(
                stdout="Name: dev\nState: Running\nBlueprint: ubuntu:22.04\nRuntime ID: abc123\nIP: 172.17.0.2\n"
            ),
        ]
        sb = self.client.create_sandbox(name="dev", blueprint="ubuntu:22.04")
        self.assertIsInstance(sb, Sandbox)
        self.assertEqual(sb.name, "dev")
        self.assertEqual(sb.state, "Running")
        self.assertEqual(sb.blueprint, "ubuntu:22.04")
        self.assertEqual(sb.ip, "172.17.0.2")

    @patch("subprocess.run")
    def test_create_sandbox_with_workspace(self, mock_run):
        mock_run.side_effect = [
            _make_result(stdout="Sandbox 'dev' created.\n"),
            _make_result(
                stdout="Name: dev\nState: Running\nBlueprint: ubuntu:22.04\nRuntime ID: abc123\n"
            ),
        ]
        sb = self.client.create_sandbox(
            name="dev", blueprint="ubuntu:22.04", workspace="/tmp/work"
        )
        # Verify workspace flag was passed
        create_call = mock_run.call_args_list[0]
        self.assertIn("-w", create_call.args[0])
        self.assertIn("/tmp/work", create_call.args[0])

    @patch("subprocess.run")
    def test_get_sandbox(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Name: test-box\nState: Stopped\nBlueprint: alpine:3.18\nRuntime ID: xyz789\n"
        )
        sb = self.client.get_sandbox("test-box")
        self.assertEqual(sb.name, "test-box")
        self.assertEqual(sb.state, "Stopped")
        self.assertEqual(sb.blueprint, "alpine:3.18")
        self.assertEqual(sb.runtime_id, "xyz789")

    @patch("subprocess.run")
    def test_get_sandbox_not_found(self, mock_run):
        mock_run.return_value = _make_result(
            stderr="sandbox 'nope' not found", returncode=1
        )
        with self.assertRaises(CLIError) as ctx:
            self.client.get_sandbox("nope")
        self.assertEqual(ctx.exception.exit_code, 1)
        self.assertIn("not found", ctx.exception.stderr)

    @patch("subprocess.run")
    def test_list_sandboxes(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="NAME       STATE     BLUEPRINT       RUNTIME_ID\ndev        Running   ubuntu:22.04    abc123\ntest       Stopped   alpine:3.18     def456\n"
        )
        sandboxes = self.client.list_sandboxes()
        self.assertEqual(len(sandboxes), 2)
        self.assertEqual(sandboxes[0].name, "dev")
        self.assertEqual(sandboxes[0].state, "Running")
        self.assertEqual(sandboxes[1].name, "test")
        self.assertEqual(sandboxes[1].state, "Stopped")

    @patch("subprocess.run")
    def test_list_sandboxes_empty(self, mock_run):
        mock_run.return_value = _make_result(stdout="No sandboxes found\n")
        sandboxes = self.client.list_sandboxes()
        self.assertEqual(sandboxes, [])

    @patch("subprocess.run")
    def test_stop_sandbox(self, mock_run):
        mock_run.return_value = _make_result(stdout="Sandbox 'dev' stopped.\n")
        self.client.stop_sandbox("dev")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "sandbox", "stop", "dev"],
            capture_output=True,
            text=True,
        )

    @patch("subprocess.run")
    def test_start_sandbox(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Sandbox 'dev' started.\n"
        )
        self.client.start_sandbox("dev")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "sandbox", "start", "dev"],
            capture_output=True,
            text=True,
        )

    @patch("subprocess.run")
    def test_destroy_sandbox(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Sandbox 'dev' destroyed.\n"
        )
        self.client.destroy_sandbox("dev")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "sandbox", "destroy", "dev"],
            capture_output=True,
            text=True,
        )


class TestExec(unittest.TestCase):
    """Test command execution in sandboxes."""

    def setUp(self):
        self.client = SandboxMatrixClient(binary="/usr/bin/smx")

    @patch("subprocess.run")
    def test_exec_string_command(self, mock_run):
        mock_run.return_value = _make_result(stdout="hello world\n")
        result = self.client.exec("dev", "echo hello world")
        self.assertIsInstance(result, ExecResult)
        self.assertEqual(result.exit_code, 0)
        self.assertEqual(result.stdout, "hello world\n")
        self.assertEqual(result.stderr, "")
        # Verify the command was wrapped with sh -c
        call_args = mock_run.call_args.args[0]
        self.assertIn("sh", call_args)
        self.assertIn("-c", call_args)
        self.assertIn("echo hello world", call_args)

    @patch("subprocess.run")
    def test_exec_list_command(self, mock_run):
        mock_run.return_value = _make_result(stdout="/usr/bin/python3\n")
        result = self.client.exec("dev", ["which", "python3"])
        self.assertEqual(result.stdout, "/usr/bin/python3\n")
        call_args = mock_run.call_args.args[0]
        self.assertIn("which", call_args)
        self.assertIn("python3", call_args)
        # Should NOT have sh -c for list commands
        self.assertNotIn("-c", call_args)

    @patch("subprocess.run")
    def test_exec_nonzero_exit(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="", stderr="command not found: foo\n", returncode=127
        )
        result = self.client.exec("dev", "foo")
        self.assertEqual(result.exit_code, 127)
        self.assertIn("command not found", result.stderr)


class TestSnapshot(unittest.TestCase):
    """Test snapshot operations."""

    def setUp(self):
        self.client = SandboxMatrixClient(binary="/usr/bin/smx")

    @patch("subprocess.run")
    def test_snapshot_with_tag(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Snapshot created: snap-abc123\n"
        )
        snap_id = self.client.snapshot("dev", tag="v1")
        self.assertEqual(snap_id, "snap-abc123")
        call_args = mock_run.call_args.args[0]
        self.assertIn("--tag", call_args)
        self.assertIn("v1", call_args)

    @patch("subprocess.run")
    def test_snapshot_without_tag(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Snapshot created: snap-def456\n"
        )
        snap_id = self.client.snapshot("dev")
        self.assertEqual(snap_id, "snap-def456")
        call_args = mock_run.call_args.args[0]
        self.assertNotIn("--tag", call_args)

    @patch("subprocess.run")
    def test_restore(self, mock_run):
        mock_run.side_effect = [
            _make_result(stdout="Sandbox 'dev-copy' restored.\n"),
            _make_result(
                stdout="Name: dev-copy\nState: Running\nBlueprint: ubuntu:22.04\nRuntime ID: new123\n"
            ),
        ]
        sb = self.client.restore("dev", "snap-abc123", "dev-copy")
        self.assertEqual(sb.name, "dev-copy")
        self.assertEqual(sb.state, "Running")


class TestMatrix(unittest.TestCase):
    """Test matrix operations."""

    def setUp(self):
        self.client = SandboxMatrixClient(binary="/usr/bin/smx")

    @patch("subprocess.run")
    def test_create_matrix(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Matrix 'cluster' created.\n"
        )
        self.client.create_matrix(
            "cluster",
            members={"web": "nginx:latest", "db": "postgres:15"},
        )
        call_args = mock_run.call_args.args[0]
        self.assertIn("matrix", call_args)
        self.assertIn("create", call_args)
        self.assertIn("cluster", call_args)
        # Check member flags are present
        member_args = " ".join(call_args)
        self.assertIn("web:nginx:latest", member_args)
        self.assertIn("db:postgres:15", member_args)

    @patch("subprocess.run")
    def test_list_matrices(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="NAME       STATE     MEMBERS\ncluster    Running   web,db,api\n"
        )
        matrices = self.client.list_matrices()
        self.assertEqual(len(matrices), 1)
        self.assertEqual(matrices[0].name, "cluster")
        self.assertEqual(matrices[0].members, ["web", "db", "api"])

    @patch("subprocess.run")
    def test_list_matrices_empty(self, mock_run):
        mock_run.return_value = _make_result(stdout="No matrices found\n")
        matrices = self.client.list_matrices()
        self.assertEqual(matrices, [])

    @patch("subprocess.run")
    def test_destroy_matrix(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Matrix 'cluster' destroyed.\n"
        )
        self.client.destroy_matrix("cluster")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "matrix", "destroy", "cluster"],
            capture_output=True,
            text=True,
        )


class TestSession(unittest.TestCase):
    """Test session operations."""

    def setUp(self):
        self.client = SandboxMatrixClient(binary="/usr/bin/smx")

    @patch("subprocess.run")
    def test_start_session(self, mock_run):
        mock_run.return_value = _make_result(
            stdout='Session "dev-session-1" started on sandbox dev.\n'
        )
        session_id = self.client.start_session("dev")
        self.assertEqual(session_id, "dev-session-1")

    @patch("subprocess.run")
    def test_end_session(self, mock_run):
        mock_run.return_value = _make_result(
            stdout="Session ended.\n"
        )
        self.client.end_session("dev-session-1")
        mock_run.assert_called_once_with(
            ["/usr/bin/smx", "session", "end", "dev-session-1"],
            capture_output=True,
            text=True,
        )


class TestCLIError(unittest.TestCase):
    """Test CLIError exception attributes."""

    def test_cli_error_attributes(self):
        err = CLIError("something broke", exit_code=42, stderr="details here")
        self.assertEqual(str(err), "something broke")
        self.assertEqual(err.exit_code, 42)
        self.assertEqual(err.stderr, "details here")

    @patch("subprocess.run")
    def test_run_raises_on_failure(self, mock_run):
        mock_run.return_value = _make_result(
            stderr="fatal error", returncode=2
        )
        client = SandboxMatrixClient(binary="/usr/bin/smx")
        with self.assertRaises(CLIError) as ctx:
            client._run("bad", "command")
        self.assertEqual(ctx.exception.exit_code, 2)
        self.assertIn("fatal error", ctx.exception.stderr)


if __name__ == "__main__":
    unittest.main()
