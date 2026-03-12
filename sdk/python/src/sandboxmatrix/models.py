from dataclasses import dataclass
from typing import Optional
from datetime import datetime


@dataclass
class Sandbox:
    name: str
    state: str
    blueprint: str
    runtime_id: str
    ip: Optional[str] = None
    created_at: Optional[datetime] = None


@dataclass
class ExecResult:
    exit_code: int
    stdout: str
    stderr: str


@dataclass
class Snapshot:
    id: str
    tag: str
    created_at: Optional[datetime] = None
    size: int = 0


@dataclass
class Matrix:
    name: str
    state: str
    members: list[str]


@dataclass
class Session:
    id: str
    sandbox: str
    state: str
    exec_count: int = 0


@dataclass
class FileInfo:
    name: str
    path: str
    size: int
    is_dir: bool
    mod_time: str = ""


@dataclass
class PortMapping:
    sandbox_name: str
    container_port: int
    host_port: int
    protocol: str = "tcp"


@dataclass
class InterpretResult:
    stdout: str
    stderr: str
    exit_code: int
    duration: str
    error: str = ""
    files: list = None


@dataclass
class BuildResult:
    image_id: str
    image_tag: str
    blueprint: str
    cached: bool = False
