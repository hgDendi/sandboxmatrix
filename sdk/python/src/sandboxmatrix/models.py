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
