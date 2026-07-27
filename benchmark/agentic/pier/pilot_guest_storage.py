"""Development-only storage guard for the Pier Docker host.

This module is intentionally excluded from formal benchmark evidence.  Pier
0.3 records ``storage_mb`` but does not enforce it for Docker environments, so
the development pilot uses a narrowly privileged helper outside the task
container to observe the Lima guest and the exact running container overlay.
"""

from __future__ import annotations

import asyncio
import ctypes
import datetime as dt
import hashlib
import json
import os
import re
import secrets
import select
import signal
import stat
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Awaitable, Callable


PILOT_GUEST_STORAGE_SCHEMA = "agentic-bench/pilot-guest-storage-guard-v1"
PILOT_GUEST_STORAGE_AUTHORITY = "lima-guest-root-helper-pinned-fd-statfs-pilot-v1"
PILOT_GUEST_STORAGE_RECEIPT = "pilot-guest-storage-receipt.json"
PILOT_GUEST_PREFLIGHT_SCHEMA = "agentic-bench/pilot-guest-storage-preflight-v1"
PILOT_GUEST_PREFLIGHT_RECEIPT = "pilot-guest-storage-preflight.json"
PILOT_GUEST_PREFLIGHT_DIRECTORY = Path(
    "/home/blurooo.guest/agentic-bench/private"
)
PILOT_GUEST_RECEIPT_OWNER_UID = 501
PILOT_GUEST_RECEIPT_OWNER_GID = 1_000
PILOT_GUEST_HELPER_VERSION = "1.0.0"
DECLARED_STORAGE_MB = 20_480
START_MINIMUM_BYTES = 30_064_771_072
RUNTIME_FLOOR_BYTES = 8_589_934_592
POLL_INTERVAL_MS = 1_000
GAP_THRESHOLD_MS = 2_500

_HELPER_ENTRY = "__pilot_guest_storage_helper__"
_PREFLIGHT_ENTRY = "__pilot_guest_storage_preflight__"
_DOCKER_BINARY = "/usr/bin/docker"
_PYTHON_BINARY = "/usr/bin/python3"
_SUDO_BINARY = "/usr/bin/sudo"
_MAX_COMMAND_OUTPUT = 1 << 20
_HEX_ID = re.compile(r"^[0-9a-f]{64}$")
_HEX_SHA256 = re.compile(r"^[0-9a-f]{64}$")


class PilotGuestStorageFailure(RuntimeError):
    def __init__(self, code: str):
        super().__init__("pilot_guest_storage:" + code)
        self.code = code


class _FSID(ctypes.Structure):
    _fields_ = [("value", ctypes.c_int * 2)]


class _LinuxStatFS(ctypes.Structure):
    _fields_ = [
        ("filesystem_type", ctypes.c_long),
        ("block_size", ctypes.c_long),
        ("blocks", ctypes.c_ulong),
        ("blocks_free", ctypes.c_ulong),
        ("blocks_available", ctypes.c_ulong),
        ("files", ctypes.c_ulong),
        ("files_free", ctypes.c_ulong),
        ("filesystem_id", _FSID),
        ("name_length", ctypes.c_long),
        ("fragment_size", ctypes.c_long),
        ("mount_flags", ctypes.c_long),
        ("spare", ctypes.c_long * 4),
    ]


_LIBC = ctypes.CDLL(None, use_errno=True)
_FSTATFS = _LIBC.fstatfs
_FSTATFS.argtypes = [ctypes.c_int, ctypes.POINTER(_LinuxStatFS)]
_FSTATFS.restype = ctypes.c_int


def _utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="microseconds").replace(
        "+00:00", "Z"
    )


def _sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1 << 20):
            digest.update(chunk)
    return digest.hexdigest()


def _strict_object(raw: bytes, expected: set[str]) -> dict:
    def reject_duplicates(pairs: list[tuple[str, object]]) -> dict:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise PilotGuestStorageFailure("authority_invalid")
            result[key] = value
        return result

    try:
        value = json.loads(raw, object_pairs_hook=reject_duplicates)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise PilotGuestStorageFailure("authority_invalid") from error
    if not isinstance(value, dict) or set(value) != expected:
        raise PilotGuestStorageFailure("authority_invalid")
    return value


@dataclass(frozen=True)
class _FilesystemSample:
    private_identity: str
    public_identity: str
    filesystem_type: str
    block_size: int
    total: int
    available: int
    used: int


@dataclass
class _PinnedFilesystem:
    roles: list[str]
    fd: int
    identity: str
    public_identity: str
    filesystem_type: str
    block_size: int
    total: int
    samples: list[dict] = field(default_factory=list)
    minimum_available: int = (1 << 64) - 1
    maximum_used: int = 0
    maximum_gap_ms: int = 0


DockerCommand = Callable[[list[str]], Awaitable[bytes]]
ComposeCommand = Callable[[list[str]], Awaitable[bytes]]


async def _exact_compose_container_id(
    docker_command: DockerCommand, compose_command: ComposeCommand
) -> str:
    candidate = (await compose_command(["ps", "-q", "main"])).strip()
    if re.fullmatch(rb"[0-9a-f]{12,64}", candidate) is None:
        raise PilotGuestStorageFailure("container_identity_invalid")
    raw = await docker_command(
        [
            "docker",
            "inspect",
            "--format",
            '{"id":{{json .Id}},"running":{{json .State.Running}},"restart_count":{{json .RestartCount}}}',
            candidate.decode("ascii"),
        ]
    )
    value = _strict_object(raw, {"id", "running", "restart_count"})
    container_id = value["id"]
    if (
        not isinstance(container_id, str)
        or _HEX_ID.fullmatch(container_id) is None
        or value["running"] is not True
        or type(value["restart_count"]) is not int
        or value["restart_count"] != 0
    ):
        raise PilotGuestStorageFailure("container_identity_invalid")
    return container_id


def _open_verified_receipt_directory(path: Path) -> tuple[int, os.stat_result]:
    if not path.is_absolute() or "\x00" in str(path):
        raise PilotGuestStorageFailure("receipt_directory_invalid")
    lexical = Path(os.path.abspath(path))
    try:
        resolved = path.resolve(strict=True)
    except OSError as error:
        raise PilotGuestStorageFailure("receipt_directory_invalid") from error
    if lexical != resolved:
        raise PilotGuestStorageFailure("receipt_directory_invalid")
    flags = os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW
    try:
        fd = os.open(resolved, flags)
    except OSError as error:
        raise PilotGuestStorageFailure("receipt_directory_invalid") from error
    try:
        metadata = os.fstat(fd)
        if not stat.S_ISDIR(metadata.st_mode):
            raise PilotGuestStorageFailure("receipt_directory_invalid")
        return fd, metadata
    except Exception:
        os.close(fd)
        raise


def _write_bytes_at(
    directory_fd: int,
    name: str,
    raw: bytes,
    *,
    owner_uid: int,
    owner_gid: int,
) -> None:
    if name not in {PILOT_GUEST_STORAGE_RECEIPT, PILOT_GUEST_PREFLIGHT_RECEIPT}:
        raise PilotGuestStorageFailure("receipt_target_invalid")
    try:
        existing = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        existing = None
    if existing is not None and (
        not stat.S_ISREG(existing.st_mode)
        or existing.st_uid != owner_uid
        or existing.st_gid != owner_gid
    ):
        raise PilotGuestStorageFailure("receipt_target_invalid")
    temporary = ".pilot-guest-storage-receipt.tmp-" + secrets.token_hex(8)
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW
    fd = os.open(temporary, flags, 0o600, dir_fd=directory_fd)
    try:
        os.fchown(fd, owner_uid, owner_gid)
        os.fchmod(fd, 0o600)
        offset = 0
        while offset < len(raw):
            written = os.write(fd, raw[offset:])
            if written <= 0:
                raise OSError("short receipt write")
            offset += written
        os.fsync(fd)
    except Exception:
        try:
            os.unlink(temporary, dir_fd=directory_fd)
        except FileNotFoundError:
            pass
        raise
    finally:
        os.close(fd)
    try:
        os.replace(
            temporary,
            name,
            src_dir_fd=directory_fd,
            dst_dir_fd=directory_fd,
        )
        os.fsync(directory_fd)
    finally:
        try:
            os.unlink(temporary, dir_fd=directory_fd)
        except FileNotFoundError:
            pass


def _verify_bytes_at(directory_fd: int, name: str, expected: bytes) -> str:
    fd = os.open(
        name,
        os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
        dir_fd=directory_fd,
    )
    try:
        metadata = os.fstat(fd)
        if not stat.S_ISREG(metadata.st_mode) or metadata.st_size != len(expected):
            raise PilotGuestStorageFailure("receipt_persistence_failed")
        chunks: list[bytes] = []
        remaining = metadata.st_size
        while remaining:
            chunk = os.read(fd, min(remaining, 1 << 20))
            if not chunk:
                raise PilotGuestStorageFailure("receipt_persistence_failed")
            chunks.append(chunk)
            remaining -= len(chunk)
    finally:
        os.close(fd)
    actual = b"".join(chunks)
    if actual != expected:
        raise PilotGuestStorageFailure("receipt_persistence_failed")
    return hashlib.sha256(actual).hexdigest()


class _RootPilotGuestStorageMonitor:
    """Root-only monitor. Paths are derived from Docker authority, never input."""

    def __init__(
        self,
        *,
        phase: str,
        session_identity_sha256: str,
        container_id: str,
        receipt_directory_fd: int,
        receipt_owner_uid: int,
        receipt_owner_gid: int,
        helper_sha256: str,
    ) -> None:
        self._phase = phase
        self._session_identity_sha256 = session_identity_sha256
        self._container_id = container_id
        self._receipt_directory_fd = receipt_directory_fd
        self._receipt_owner_uid = receipt_owner_uid
        self._receipt_owner_gid = receipt_owner_gid
        self._helper_sha256 = helper_sha256
        self._origin_ns = 0
        self._started_at = _utc_now()
        self._finished_at: str | None = None
        self._finished_delta_ms: int | None = None
        self._docker_root = ""
        self._merged_dir = ""
        self._container_pid = 0
        self._container_started_at = ""
        self._proc_fd = -1
        self._pid_fd = -1
        self._proc_identity: tuple[int, int, int] | None = None
        self._proc_start_ticks = ""
        self._cgroup_sha256 = ""
        self._filesystems: list[_PinnedFilesystem] = []
        self._status = "initializing"
        self._kill_attempted = False
        self._kill_acknowledged = False
        self._monitor_task: asyncio.Task[None] | None = None
        self._failure: asyncio.Future[PilotGuestStorageFailure] | None = None
        self._lock = asyncio.Lock()

    @property
    def failure(self) -> asyncio.Future[PilotGuestStorageFailure]:
        if self._failure is None:
            raise PilotGuestStorageFailure("not_started")
        return self._failure

    async def start(self) -> None:
        if self._failure is not None:
            raise PilotGuestStorageFailure("duplicate_start")
        self._failure = asyncio.get_running_loop().create_future()
        try:
            await self._resolve_authority()
            self._pin_container_process()
            self._pin_filesystems()
            self._origin_ns = asyncio.get_running_loop().time_ns() if hasattr(
                asyncio.get_running_loop(), "time_ns"
            ) else int(asyncio.get_running_loop().time() * 1_000_000_000)
            await self._sample_all(require_start=True)
            self._status = "running"
            self._write_receipt()
        except PilotGuestStorageFailure as failure:
            await self._terminal_failure(failure)
            raise
        except Exception as error:
            failure = PilotGuestStorageFailure("observation_failed")
            await self._terminal_failure(failure)
            raise failure from error
        self._monitor_task = asyncio.create_task(self._monitor())

    async def finish(self) -> PilotGuestStorageFailure | None:
        task = self._monitor_task
        self._monitor_task = None
        if task is not None:
            task.cancel()
            await asyncio.gather(task, return_exceptions=True)
        failure = (
            self._failure.result()
            if self._failure is not None and self._failure.done()
            else None
        )
        if failure is None:
            try:
                async with self._lock:
                    await self._verify_container(running=True)
                    self._verify_pinned_container_process()
                    await self._sample_all(require_start=False)
                    self._status = "completed_above_guard"
                    self._finished_at = _utc_now()
                    self._finished_delta_ms = self._delta_ms()
                    self._write_receipt()
            except PilotGuestStorageFailure as terminal:
                await self._terminal_failure(terminal)
                failure = terminal
        self._close_filesystems()
        self._close_container_process()
        return failure

    async def fail(self, code: str) -> PilotGuestStorageFailure:
        failure = PilotGuestStorageFailure(code)
        await self._terminal_failure(failure)
        return failure

    async def _monitor(self) -> None:
        loop = asyncio.get_running_loop()
        interval_ns = POLL_INTERVAL_MS * 1_000_000
        now_ns = loop.time_ns() if hasattr(loop, "time_ns") else int(
            loop.time() * 1_000_000_000
        )
        next_sample_ns = now_ns + interval_ns
        try:
            while True:
                now_ns = loop.time_ns() if hasattr(loop, "time_ns") else int(
                    loop.time() * 1_000_000_000
                )
                await asyncio.sleep(max(0, next_sample_ns - now_ns) / 1_000_000_000)
                async with self._lock:
                    self._verify_pinned_container_process()
                    await self._sample_all(require_start=False)
                    self._write_receipt()
                next_sample_ns += interval_ns
        except asyncio.CancelledError:
            return
        except PilotGuestStorageFailure as failure:
            await self._terminal_failure(failure)
        except Exception:
            await self._terminal_failure(
                PilotGuestStorageFailure("observation_failed")
            )

    async def _resolve_authority(self) -> None:
        if _HEX_ID.fullmatch(self._container_id) is None:
            raise PilotGuestStorageFailure("container_identity_invalid")
        root_raw = await _root_docker_command(
            ["docker", "info", "--format", "{{json .DockerRootDir}}"]
        )
        try:
            root = json.loads(root_raw)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise PilotGuestStorageFailure("authority_invalid") from error
        if (
            not isinstance(root, str)
            or not root.startswith("/")
            or os.path.normpath(root) != root
            or os.path.realpath(root) != root
            or "\x00" in root
        ):
            raise PilotGuestStorageFailure("authority_invalid")
        raw = await _root_docker_command(
            [
                "docker",
                "inspect",
                "--format",
                '{"id":{{json .Id}},"running":{{json .State.Running}},"restart_count":{{json .RestartCount}},"driver":{{json .GraphDriver.Name}},"merged_dir":{{json .GraphDriver.Data.MergedDir}},"pid":{{json .State.Pid}},"started_at":{{json .State.StartedAt}}}',
                self._container_id,
            ]
        )
        value = _strict_object(
            raw,
            {
                "id",
                "running",
                "restart_count",
                "driver",
                "merged_dir",
                "pid",
                "started_at",
            },
        )
        merged = value["merged_dir"]
        if (
            value["id"] != self._container_id
            or value["running"] is not True
            or type(value["restart_count"]) is not int
            or value["restart_count"] != 0
            or value["driver"] != "overlay2"
            or type(value["pid"]) is not int
            or value["pid"] <= 1
            or not isinstance(value["started_at"], str)
            or not value["started_at"].endswith("Z")
            or len(value["started_at"]) < 20
            or not isinstance(merged, str)
            or not merged.startswith("/")
            or os.path.normpath(merged) != merged
            or os.path.realpath(merged) != merged
            or "\x00" in merged
        ):
            raise PilotGuestStorageFailure("container_identity_invalid")
        try:
            if os.path.commonpath((root, merged)) != root or merged == root:
                raise PilotGuestStorageFailure("container_identity_invalid")
        except ValueError as error:
            raise PilotGuestStorageFailure("container_identity_invalid") from error
        self._docker_root = root
        self._merged_dir = merged
        self._container_pid = value["pid"]
        self._container_started_at = value["started_at"]

    async def _verify_container(self, *, running: bool) -> None:
        raw = await _root_docker_command(
            [
                "docker",
                "inspect",
                "--format",
                '{"id":{{json .Id}},"running":{{json .State.Running}},"restart_count":{{json .RestartCount}},"driver":{{json .GraphDriver.Name}},"merged_dir":{{json .GraphDriver.Data.MergedDir}},"pid":{{json .State.Pid}},"started_at":{{json .State.StartedAt}}}',
                self._container_id,
            ]
        )
        value = _strict_object(
            raw,
            {
                "id",
                "running",
                "restart_count",
                "driver",
                "merged_dir",
                "pid",
                "started_at",
            },
        )
        if (
            value["id"] != self._container_id
            or value["running"] is not running
            or type(value["restart_count"]) is not int
            or value["restart_count"] != 0
            or value["driver"] != "overlay2"
            or value["merged_dir"] != self._merged_dir
            or value["started_at"] != self._container_started_at
            or type(value["pid"]) is not int
            or (running and value["pid"] != self._container_pid)
            or (not running and value["pid"] != 0)
        ):
            raise PilotGuestStorageFailure("container_identity_changed")

    @staticmethod
    def _read_proc_file(directory_fd: int, name: str, maximum: int) -> bytes:
        fd = os.open(
            name,
            os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=directory_fd,
        )
        try:
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = os.read(fd, min(4096, maximum + 1 - total))
                if not chunk:
                    break
                chunks.append(chunk)
                total += len(chunk)
                if total > maximum:
                    raise PilotGuestStorageFailure("container_identity_invalid")
            return b"".join(chunks)
        finally:
            os.close(fd)

    def _read_proc_identity(self, directory_fd: int) -> tuple[str, str]:
        process_stat = self._read_proc_file(directory_fd, "stat", 16 << 10)
        prefix = str(self._container_pid).encode("ascii") + b" ("
        closing = process_stat.rfind(b") ")
        if not process_stat.startswith(prefix) or closing < len(prefix):
            raise PilotGuestStorageFailure("container_identity_invalid")
        fields = process_stat[closing + 2 :].split()
        if (
            len(fields) < 20
            or len(fields[0]) != 1
            or fields[0] in {b"Z", b"X", b"x"}
            or not fields[19].isdigit()
        ):
            raise PilotGuestStorageFailure("container_identity_invalid")
        cgroup = self._read_proc_file(directory_fd, "cgroup", 64 << 10)
        container = self._container_id.encode("ascii")
        if re.search(
            rb"(?<![0-9a-f])" + re.escape(container) + rb"(?![0-9a-f])",
            cgroup,
        ) is None:
            raise PilotGuestStorageFailure("container_identity_invalid")
        return fields[19].decode("ascii"), hashlib.sha256(cgroup).hexdigest()

    def _pin_container_process(self) -> None:
        proc_fd = -1
        pid_fd = -1
        try:
            proc_fd = os.open(
                f"/proc/{self._container_pid}",
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            )
            metadata = os.fstat(proc_fd)
            if not stat.S_ISDIR(metadata.st_mode):
                raise PilotGuestStorageFailure("container_identity_invalid")
            if not hasattr(os, "pidfd_open"):
                raise PilotGuestStorageFailure("container_identity_invalid")
            pid_fd = os.pidfd_open(self._container_pid, 0)
            if select.select([pid_fd], [], [], 0)[0]:
                raise PilotGuestStorageFailure("container_identity_invalid")
            start_ticks, cgroup_sha256 = self._read_proc_identity(proc_fd)
            self._proc_identity = (
                int(metadata.st_dev),
                int(metadata.st_ino),
                stat.S_IFMT(metadata.st_mode),
            )
            self._proc_start_ticks = start_ticks
            self._cgroup_sha256 = cgroup_sha256
            self._proc_fd = proc_fd
            self._pid_fd = pid_fd
            proc_fd = -1
            pid_fd = -1
        except PilotGuestStorageFailure:
            raise
        except Exception as error:
            raise PilotGuestStorageFailure("container_identity_invalid") from error
        finally:
            if proc_fd >= 0:
                os.close(proc_fd)
            if pid_fd >= 0:
                os.close(pid_fd)

    def _verify_pinned_container_process(self) -> None:
        if (
            self._proc_fd < 0
            or self._pid_fd < 0
            or self._proc_identity is None
            or select.select([self._pid_fd], [], [], 0)[0]
        ):
            raise PilotGuestStorageFailure("container_identity_changed")
        try:
            metadata = os.fstat(self._proc_fd)
            identity = (
                int(metadata.st_dev),
                int(metadata.st_ino),
                stat.S_IFMT(metadata.st_mode),
            )
            start_ticks, cgroup_sha256 = self._read_proc_identity(self._proc_fd)
        except PilotGuestStorageFailure as error:
            raise PilotGuestStorageFailure("container_identity_changed") from error
        except Exception as error:
            raise PilotGuestStorageFailure("container_identity_changed") from error
        if (
            identity != self._proc_identity
            or start_ticks != self._proc_start_ticks
            or cgroup_sha256 != self._cgroup_sha256
        ):
            raise PilotGuestStorageFailure("container_identity_changed")

    def _close_container_process(self) -> None:
        for descriptor_name in ("_proc_fd", "_pid_fd"):
            descriptor = getattr(self, descriptor_name)
            if descriptor >= 0:
                try:
                    os.close(descriptor)
                except OSError:
                    pass
                setattr(self, descriptor_name, -1)

    def _pin_filesystems(self) -> None:
        opened: list[_PinnedFilesystem] = []
        current_fd: int | None = None
        try:
            for role, path in (
                ("guest_root", "/"),
                ("docker_root", self._docker_root),
                ("container_overlay", self._merged_dir),
            ):
                current_fd = os.open(
                    path,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                )
                sample = _sample_fd(current_fd)
                existing = next(
                    (
                        item
                        for item in opened
                        if item.identity == sample.private_identity
                    ),
                    None,
                )
                if existing is not None:
                    existing.roles.append(role)
                    os.close(current_fd)
                    current_fd = None
                    continue
                opened.append(
                    _PinnedFilesystem(
                        roles=[role],
                        fd=current_fd,
                        identity=sample.private_identity,
                        public_identity=sample.public_identity,
                        filesystem_type=sample.filesystem_type,
                        block_size=sample.block_size,
                        total=sample.total,
                    )
                )
                current_fd = None
            for item in opened:
                item.roles.sort()
            opened.sort(key=lambda item: item.roles)
            self._filesystems = opened
        except Exception as error:
            if current_fd is not None:
                os.close(current_fd)
            for item in opened:
                os.close(item.fd)
            if isinstance(error, PilotGuestStorageFailure):
                raise
            raise PilotGuestStorageFailure("pin_failed") from error

    async def _sample_all(self, *, require_start: bool) -> None:
        for filesystem in self._filesystems:
            start_delta = self._delta_ms()
            try:
                sample = _sample_fd(filesystem.fd)
            except OSError as error:
                raise PilotGuestStorageFailure("observation_failed") from error
            end_delta = self._delta_ms()
            if (
                sample.private_identity != filesystem.identity
                or sample.public_identity != filesystem.public_identity
                or sample.filesystem_type != filesystem.filesystem_type
                or sample.block_size != filesystem.block_size
                or sample.total != filesystem.total
            ):
                raise PilotGuestStorageFailure("filesystem_identity_changed")
            previous_end = (
                filesystem.samples[-1]["end_delta_ms"]
                if filesystem.samples
                else 0
            )
            gap = end_delta - previous_end
            if gap < 0 or gap > GAP_THRESHOLD_MS:
                raise PilotGuestStorageFailure("monitoring_gap")
            filesystem.maximum_gap_ms = max(filesystem.maximum_gap_ms, gap)
            filesystem.minimum_available = min(
                filesystem.minimum_available, sample.available
            )
            filesystem.maximum_used = max(filesystem.maximum_used, sample.used)
            filesystem.samples.append(
                {
                    "observed_at": _utc_now(),
                    "start_delta_ms": start_delta,
                    "end_delta_ms": end_delta,
                    "available_bytes": sample.available,
                    "used_bytes": sample.used,
                }
            )
            if require_start and sample.available < START_MINIMUM_BYTES:
                raise PilotGuestStorageFailure("start_below_guard")
            if not require_start and sample.available < RUNTIME_FLOOR_BYTES:
                raise PilotGuestStorageFailure("runtime_floor_breached")

    async def _terminal_failure(self, failure: PilotGuestStorageFailure) -> None:
        async with self._lock:
            terminal = self._status not in {
                "completed_above_guard",
                "start_below_guard",
                "runtime_floor_breached",
                "monitoring_gap",
                "filesystem_identity_changed",
                "container_identity_changed",
                "observation_failed",
                "pin_failed",
                "authority_invalid",
                "container_identity_invalid",
                "control_channel_closed",
                "control_message_invalid",
                "receipt_persistence_failed",
            }
            if terminal:
                self._status = failure.code
                self._finished_at = _utc_now()
                self._finished_delta_ms = self._delta_ms()
                self._kill_attempted = True
                persistence_failed = False
                try:
                    self._write_receipt()
                except Exception:
                    persistence_failed = True
                try:
                    await asyncio.wait_for(
                        _root_docker_command(
                            ["docker", "kill", self._container_id]
                        ),
                        timeout=10,
                    )
                    await asyncio.wait_for(
                        self._verify_container(running=False), timeout=10
                    )
                    self._kill_acknowledged = True
                except Exception:
                    self._kill_acknowledged = False
                try:
                    self._write_receipt()
                except Exception:
                    persistence_failed = True
                if persistence_failed:
                    failure = PilotGuestStorageFailure(
                        "receipt_persistence_failed"
                    )
                    self._status = failure.code
            if self._failure is not None and not self._failure.done():
                self._failure.set_result(failure)

    def _delta_ms(self) -> int:
        if self._origin_ns == 0:
            return 0
        loop = asyncio.get_running_loop()
        now_ns = loop.time_ns() if hasattr(loop, "time_ns") else int(
            loop.time() * 1_000_000_000
        )
        return max(0, (now_ns - self._origin_ns) // 1_000_000)

    def _receipt(self) -> dict:
        filesystems = []
        for group, filesystem in enumerate(self._filesystems):
            filesystems.append(
                {
                    "group": group,
                    "roles": list(filesystem.roles),
                    "volume_identity_sha256": filesystem.public_identity,
                    "filesystem_type": filesystem.filesystem_type,
                    "device_role_count": len(filesystem.roles),
                    "block_size_bytes": filesystem.block_size,
                    "total_bytes": filesystem.total,
                    "minimum_available_bytes": (
                        filesystem.minimum_available
                        if filesystem.samples
                        else 0
                    ),
                    "maximum_used_bytes": filesystem.maximum_used,
                    "maximum_completion_gap_ms": filesystem.maximum_gap_ms,
                    "samples": list(filesystem.samples),
                }
            )
        return {
            "schema_version": PILOT_GUEST_STORAGE_SCHEMA,
            "formal_compatible": False,
            "evidence_mode": "diagnostic_unilateral",
            "crash_proof": False,
            "phase": self._phase,
            "session_identity_sha256": self._session_identity_sha256,
            "container_identity_sha256": _sha256_text(self._container_id),
            "container_process_authority": {
                "method": "pidfd-pinned-proc-starttime-cgroup-v1",
                "proc_directory_identity_sha256": (
                    _sha256_text(
                        "agentic-bench/proc-directory-identity-v1\n"
                        + ":".join(str(value) for value in self._proc_identity)
                    )
                    if self._proc_identity is not None
                    else ""
                ),
                "proc_start_ticks_sha256": (
                    _sha256_text(self._proc_start_ticks)
                    if self._proc_start_ticks
                    else ""
                ),
                "cgroup_sha256": self._cgroup_sha256,
                "pidfd_pinned": self._pid_fd >= 0,
            },
            "enforcement": "declared_by_task_unenforced_by_pier-0.3",
            "declared_storage_mb": DECLARED_STORAGE_MB,
            "guest_storage_guard": {
                "schema_version": "agentic-bench/guest-storage-guard-v1",
                "start_minimum_available_bytes": START_MINIMUM_BYTES,
                "runtime_abort_below_available_bytes": RUNTIME_FLOOR_BYTES,
                "poll_interval_ms": POLL_INTERVAL_MS,
                "monitoring_gap_threshold_ms": GAP_THRESHOLD_MS,
                "measurement": "statfs_bavail_times_block_size",
            },
            "authority": PILOT_GUEST_STORAGE_AUTHORITY,
            "helper": {
                "version": PILOT_GUEST_HELPER_VERSION,
                "sha256": self._helper_sha256,
                "python_isolated": True,
                "python_no_bytecode": True,
            },
            "started_at": self._started_at,
            "finished_at": self._finished_at,
            "finished_delta_ms": self._finished_delta_ms,
            "filesystems": filesystems,
            "kill_attempted": self._kill_attempted,
            "kill_acknowledged": self._kill_acknowledged,
            "status": self._status,
        }

    def _write_receipt(self) -> None:
        raw = (
            json.dumps(self._receipt(), sort_keys=True, separators=(",", ":"))
            + "\n"
        ).encode("utf-8")
        _write_bytes_at(
            self._receipt_directory_fd,
            PILOT_GUEST_STORAGE_RECEIPT,
            raw,
            owner_uid=self._receipt_owner_uid,
            owner_gid=self._receipt_owner_gid,
        )

    def _close_filesystems(self) -> None:
        for filesystem in self._filesystems:
            try:
                os.close(filesystem.fd)
            except OSError:
                pass
        self._filesystems = []


async def _root_docker_command(argv: list[str]) -> bytes:
    if not argv or argv[0] != "docker" or not Path(_DOCKER_BINARY).is_file():
        raise PilotGuestStorageFailure("authority_invalid")
    process = await asyncio.create_subprocess_exec(
        _DOCKER_BINARY,
        *argv[1:],
        stdin=asyncio.subprocess.DEVNULL,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env={"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"},
    )
    try:
        stdout, _ = await asyncio.wait_for(process.communicate(), timeout=10)
    except BaseException:
        if process.returncode is None:
            process.kill()
            await process.communicate()
        raise
    if process.returncode != 0 or len(stdout) > _MAX_COMMAND_OUTPUT:
        raise PilotGuestStorageFailure("docker_authority_failed")
    return stdout


def _sample_fd(fd: int) -> _FilesystemSample:
    metadata = os.fstat(fd)
    statvfs = os.fstatvfs(fd)
    native = _LinuxStatFS()
    if _FSTATFS(fd, ctypes.byref(native)) != 0:
        errno = ctypes.get_errno()
        raise OSError(errno, os.strerror(errno))
    block_size = statvfs.f_frsize or statvfs.f_bsize
    if (
        block_size <= 0
        or statvfs.f_blocks <= 0
        or statvfs.f_bfree < 0
        or statvfs.f_bavail < 0
        or statvfs.f_bfree > statvfs.f_blocks
        or statvfs.f_bavail > statvfs.f_blocks
    ):
        raise OSError("invalid_statfs")
    total = statvfs.f_blocks * block_size
    available = statvfs.f_bavail * block_size
    used = (statvfs.f_blocks - statvfs.f_bfree) * block_size
    filesystem_type = f"linux-0x{native.filesystem_type & ((1 << 64) - 1):016x}"
    private_identity = (
        "agentic-bench/guest-volume-identity-v1\n"
        f"device={metadata.st_dev & ((1 << 64) - 1):016x}\n"
        f"filesystem_type={native.filesystem_type & ((1 << 64) - 1):016x}\n"
        f"fsid_0={native.filesystem_id.value[0] & ((1 << 64) - 1):016x}\n"
        f"fsid_1={native.filesystem_id.value[1] & ((1 << 64) - 1):016x}"
    )
    return _FilesystemSample(
        private_identity=private_identity,
        public_identity=_sha256_text(private_identity),
        filesystem_type=filesystem_type,
        block_size=block_size,
        total=total,
        available=available,
        used=used,
    )


class PrivilegedPilotGuestStorageGuard:
    """Unprivileged Pier-side supervisor for the isolated root helper."""

    def __init__(
        self,
        *,
        session_id: str,
        receipt_path: Path,
        docker_command: DockerCommand,
        compose_command: ComposeCommand,
    ) -> None:
        self._session_id = session_id
        self._phase = "verifier" if "__verifier__" in session_id else "agent"
        self._receipt_path = receipt_path
        self._docker_command = docker_command
        self._compose_command = compose_command
        self._receipt_directory_fd: int | None = None
        self._process: asyncio.subprocess.Process | None = None
        self._watch_task: asyncio.Task[None] | None = None
        self._failure: asyncio.Future[PilotGuestStorageFailure] | None = None
        self._finishing = False
        self._container_id = ""
        self._helper_sha256 = ""

    @property
    def failure(self) -> asyncio.Future[PilotGuestStorageFailure]:
        if self._failure is None:
            raise PilotGuestStorageFailure("not_started")
        return self._failure

    async def start(self) -> None:
        if self._failure is not None:
            raise PilotGuestStorageFailure("duplicate_start")
        loop = asyncio.get_running_loop()
        self._failure = loop.create_future()
        if self._receipt_path.name != PILOT_GUEST_STORAGE_RECEIPT:
            raise PilotGuestStorageFailure("receipt_target_invalid")
        for required in (_SUDO_BINARY, _PYTHON_BINARY, _DOCKER_BINARY):
            if not Path(required).is_file():
                raise PilotGuestStorageFailure("helper_runtime_missing")
        self._receipt_path.parent.mkdir(parents=True, exist_ok=True)
        directory_fd, directory_metadata = _open_verified_receipt_directory(
            self._receipt_path.parent
        )
        self._receipt_directory_fd = directory_fd
        if (
            directory_metadata.st_uid != os.getuid()
            or directory_metadata.st_gid != os.getgid()
        ):
            self._close_directory()
            raise PilotGuestStorageFailure("receipt_directory_invalid")
        try:
            self._container_id = await _exact_compose_container_id(
                self._docker_command, self._compose_command
            )
            helper_path = Path(__file__).resolve(strict=True)
            if not stat.S_ISREG(helper_path.stat().st_mode):
                raise PilotGuestStorageFailure("helper_runtime_invalid")
            self._helper_sha256 = _file_sha256(helper_path)
            self._process = await asyncio.create_subprocess_exec(
                _SUDO_BINARY,
                "-n",
                "--",
                _PYTHON_BINARY,
                "-I",
                "-B",
                str(helper_path),
                _HELPER_ENTRY,
                "--phase",
                self._phase,
                "--session-sha256",
                _sha256_text(self._session_id),
                "--container-id",
                self._container_id,
                "--receipt-directory",
                str(self._receipt_path.parent),
                "--receipt-device",
                str(directory_metadata.st_dev),
                "--receipt-inode",
                str(directory_metadata.st_ino),
                "--receipt-owner-uid",
                str(directory_metadata.st_uid),
                "--receipt-owner-gid",
                str(directory_metadata.st_gid),
                "--helper-sha256",
                self._helper_sha256,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            if self._process.stdout is None:
                raise PilotGuestStorageFailure("helper_launch_failed")
            line = await asyncio.wait_for(self._process.stdout.readline(), timeout=30)
            event = _strict_object(line, {"helper_sha256", "kind"})
            if (
                event["kind"] != "ready"
                or event["helper_sha256"] != self._helper_sha256
            ):
                raise PilotGuestStorageFailure("helper_start_failed")
            self._watch_task = asyncio.create_task(self._watch_helper())
        except Exception as error:
            code = (
                error.code
                if isinstance(error, PilotGuestStorageFailure)
                else "helper_launch_failed"
            )
            await self._emergency_kill()
            await self._terminate_helper()
            self._close_directory()
            failure = PilotGuestStorageFailure(code)
            if not self._failure.done():
                self._failure.set_result(failure)
            raise failure from error

    async def finish(self) -> PilotGuestStorageFailure | None:
        self._finishing = True
        process = self._process
        if process is not None and process.returncode is None:
            try:
                if process.stdin is None:
                    raise PilotGuestStorageFailure("helper_control_failed")
                process.stdin.write(b"finish\n")
                await process.stdin.drain()
                process.stdin.close()
                await asyncio.wait_for(process.wait(), timeout=20)
            except Exception:
                await self._emergency_kill()
                await self._terminate_helper()
                if self._failure is not None and not self._failure.done():
                    self._failure.set_result(
                        PilotGuestStorageFailure("helper_finish_failed")
                    )
        if self._watch_task is not None:
            await asyncio.gather(self._watch_task, return_exceptions=True)
            self._watch_task = None
        failure = (
            self._failure.result()
            if self._failure is not None and self._failure.done()
            else None
        )
        self._close_directory()
        return failure

    async def _watch_helper(self) -> None:
        process = self._process
        if process is None:
            return
        return_code = await process.wait()
        if self._finishing and return_code == 0:
            return
        code = "helper_exited"
        try:
            receipt = self._read_receipt()
            status = receipt.get("status")
            if isinstance(status, str) and status != "completed_above_guard":
                code = status
        except Exception:
            code = "helper_receipt_missing"
        if self._failure is not None and not self._failure.done():
            self._failure.set_result(PilotGuestStorageFailure(code))

    def _read_receipt(self) -> dict:
        if self._receipt_directory_fd is None:
            raise PilotGuestStorageFailure("helper_receipt_missing")
        fd = os.open(
            PILOT_GUEST_STORAGE_RECEIPT,
            os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=self._receipt_directory_fd,
        )
        try:
            metadata = os.fstat(fd)
            if not stat.S_ISREG(metadata.st_mode) or metadata.st_size > (8 << 20):
                raise PilotGuestStorageFailure("helper_receipt_invalid")
            chunks: list[bytes] = []
            remaining = metadata.st_size
            while remaining:
                chunk = os.read(fd, min(remaining, 1 << 20))
                if not chunk:
                    raise PilotGuestStorageFailure("helper_receipt_invalid")
                chunks.append(chunk)
                remaining -= len(chunk)
        finally:
            os.close(fd)
        value = json.loads(b"".join(chunks))
        if (
            not isinstance(value, dict)
            or value.get("schema_version") != PILOT_GUEST_STORAGE_SCHEMA
            or value.get("formal_compatible") is not False
            or value.get("evidence_mode") != "diagnostic_unilateral"
            or value.get("container_identity_sha256")
            != _sha256_text(self._container_id)
            or value.get("helper", {}).get("sha256") != self._helper_sha256
        ):
            raise PilotGuestStorageFailure("helper_receipt_invalid")
        return value

    async def _emergency_kill(self) -> None:
        if _HEX_ID.fullmatch(self._container_id) is None:
            return
        try:
            await asyncio.wait_for(
                self._docker_command(["docker", "kill", self._container_id]),
                timeout=10,
            )
        except Exception:
            pass

    async def _terminate_helper(self) -> None:
        process = self._process
        if process is None or process.returncode is not None:
            return
        process.terminate()
        try:
            await asyncio.wait_for(process.wait(), timeout=5)
        except asyncio.TimeoutError:
            process.kill()
            await process.wait()

    def _close_directory(self) -> None:
        if self._receipt_directory_fd is not None:
            try:
                os.close(self._receipt_directory_fd)
            except OSError:
                pass
            self._receipt_directory_fd = None


def _parse_preflight_arguments(argv: list[str]) -> str:
    if (
        len(argv) != 3
        or argv[0] != _PREFLIGHT_ENTRY
        or argv[1] != "--helper-sha256"
        or _HEX_SHA256.fullmatch(argv[2]) is None
    ):
        raise PilotGuestStorageFailure("preflight_arguments_invalid")
    return argv[2]


async def _guest_preflight_main(expected_helper_sha256: str) -> int:
    helper_path = Path(__file__).resolve(strict=True)
    helper_sha256 = _file_sha256(helper_path)
    if os.geteuid() != 0 or helper_sha256 != expected_helper_sha256:
        return 70

    directory_fd = -1
    pinned_fds: list[int] = []
    started_at = _utc_now()
    observed_at = started_at
    filesystems: list[dict] = []
    status = "preflight_failed"
    reason_code = "observation_failed"
    exit_code = 70
    try:
        directory_fd, metadata = _open_verified_receipt_directory(
            PILOT_GUEST_PREFLIGHT_DIRECTORY
        )
        if (
            metadata.st_uid != PILOT_GUEST_RECEIPT_OWNER_UID
            or metadata.st_gid != PILOT_GUEST_RECEIPT_OWNER_GID
        ):
            raise PilotGuestStorageFailure("receipt_directory_invalid")
        root_raw = await _root_docker_command(
            ["docker", "info", "--format", "{{json .DockerRootDir}}"]
        )
        try:
            docker_root = json.loads(root_raw)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise PilotGuestStorageFailure("authority_invalid") from error
        if (
            not isinstance(docker_root, str)
            or not docker_root.startswith("/")
            or os.path.normpath(docker_root) != docker_root
            or os.path.realpath(docker_root) != docker_root
            or "\x00" in docker_root
        ):
            raise PilotGuestStorageFailure("authority_invalid")

        grouped: dict[str, dict] = {}
        for role, path in (("guest_root", "/"), ("docker_root", docker_root)):
            fd = os.open(
                path,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            )
            pinned_fds.append(fd)
            sample = _sample_fd(fd)
            group = grouped.get(sample.private_identity)
            if group is None:
                group = {
                    "roles": [],
                    "volume_identity_sha256": sample.public_identity,
                    "filesystem_type": sample.filesystem_type,
                    "block_size_bytes": sample.block_size,
                    "total_bytes": sample.total,
                    "available_bytes": sample.available,
                    "used_bytes": sample.used,
                }
                grouped[sample.private_identity] = group
            else:
                if (
                    group["filesystem_type"] != sample.filesystem_type
                    or group["block_size_bytes"] != sample.block_size
                    or group["total_bytes"] != sample.total
                ):
                    raise PilotGuestStorageFailure("filesystem_identity_changed")
                group["available_bytes"] = min(
                    int(group["available_bytes"]), sample.available
                )
                group["used_bytes"] = max(int(group["used_bytes"]), sample.used)
            group["roles"].append(role)
        observed_at = _utc_now()
        for group_number, group in enumerate(
            sorted(grouped.values(), key=lambda item: item["volume_identity_sha256"])
        ):
            group["roles"].sort()
            filesystems.append(
                {
                    "group": group_number,
                    "roles": group["roles"],
                    "device_role_count": len(group["roles"]),
                    "volume_identity_sha256": group["volume_identity_sha256"],
                    "filesystem_type": group["filesystem_type"],
                    "block_size_bytes": group["block_size_bytes"],
                    "total_bytes": group["total_bytes"],
                    "available_bytes": group["available_bytes"],
                    "used_bytes": group["used_bytes"],
                }
            )
        if not filesystems:
            raise PilotGuestStorageFailure("observation_failed")
        if min(item["available_bytes"] for item in filesystems) < START_MINIMUM_BYTES:
            status = "admission_rejected"
            reason_code = "start_below_guard"
            exit_code = 78
        else:
            status = "passed"
            reason_code = "above_start_minimum"
            exit_code = 0
    except PilotGuestStorageFailure as failure:
        reason_code = failure.code
    except Exception:
        reason_code = "observation_failed"
    finally:
        for fd in pinned_fds:
            try:
                os.close(fd)
            except OSError:
                pass

    if directory_fd < 0:
        return 70
    try:
        receipt = {
            "schema_version": PILOT_GUEST_PREFLIGHT_SCHEMA,
            "formal_compatible": False,
            "evidence_mode": "diagnostic_unilateral",
            "crash_proof": False,
            "stage": "experiment_preflight",
            "authority": PILOT_GUEST_STORAGE_AUTHORITY,
            "declared_storage_mb": DECLARED_STORAGE_MB,
            "start_minimum_available_bytes": START_MINIMUM_BYTES,
            "measurement": "statfs_bavail_times_block_size",
            "helper": {
                "version": PILOT_GUEST_HELPER_VERSION,
                "sha256": helper_sha256,
                "python_isolated": True,
                "python_no_bytecode": True,
            },
            "started_at": started_at,
            "observed_at": observed_at,
            "filesystems": filesystems,
            "status": status,
            "reason_code": reason_code,
        }
        raw = (
            json.dumps(receipt, sort_keys=True, separators=(",", ":")) + "\n"
        ).encode("ascii")
        _write_bytes_at(
            directory_fd,
            PILOT_GUEST_PREFLIGHT_RECEIPT,
            raw,
            owner_uid=PILOT_GUEST_RECEIPT_OWNER_UID,
            owner_gid=PILOT_GUEST_RECEIPT_OWNER_GID,
        )
        receipt_sha256 = _verify_bytes_at(
            directory_fd, PILOT_GUEST_PREFLIGHT_RECEIPT, raw
        )
        event = {
            "formal_compatible": False,
            "receipt_sha256": receipt_sha256,
            "schema_version": PILOT_GUEST_PREFLIGHT_SCHEMA,
            "status": status,
        }
        sys.stdout.write(
            json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n"
        )
        sys.stdout.flush()
        return exit_code
    except Exception:
        return 70
    finally:
        os.close(directory_fd)


def _parse_helper_arguments(argv: list[str]) -> dict[str, object]:
    if not argv or argv[0] != _HELPER_ENTRY:
        raise PilotGuestStorageFailure("helper_arguments_invalid")
    values: dict[str, str] = {}
    index = 1
    allowed = {
        "--phase",
        "--session-sha256",
        "--container-id",
        "--receipt-directory",
        "--receipt-device",
        "--receipt-inode",
        "--receipt-owner-uid",
        "--receipt-owner-gid",
        "--helper-sha256",
    }
    while index < len(argv):
        key = argv[index]
        if key not in allowed or key in values or index + 1 >= len(argv):
            raise PilotGuestStorageFailure("helper_arguments_invalid")
        values[key] = argv[index + 1]
        index += 2
    if set(values) != allowed:
        raise PilotGuestStorageFailure("helper_arguments_invalid")
    if values["--phase"] not in {"agent", "verifier"}:
        raise PilotGuestStorageFailure("helper_arguments_invalid")
    for key in ("--session-sha256", "--helper-sha256"):
        if _HEX_SHA256.fullmatch(values[key]) is None:
            raise PilotGuestStorageFailure("helper_arguments_invalid")
    if _HEX_ID.fullmatch(values["--container-id"]) is None:
        raise PilotGuestStorageFailure("helper_arguments_invalid")
    integers: dict[str, int] = {}
    for key in (
        "--receipt-device",
        "--receipt-inode",
        "--receipt-owner-uid",
        "--receipt-owner-gid",
    ):
        try:
            integers[key] = int(values[key])
        except ValueError as error:
            raise PilotGuestStorageFailure("helper_arguments_invalid") from error
        if integers[key] < 0:
            raise PilotGuestStorageFailure("helper_arguments_invalid")
    return {**values, **integers}


def _emit_helper_event(kind: str, helper_sha256: str) -> None:
    raw = (
        json.dumps(
            {"helper_sha256": helper_sha256, "kind": kind},
            sort_keys=True,
            separators=(",", ":"),
        )
        + "\n"
    ).encode("ascii")
    sys.stdout.buffer.write(raw)
    sys.stdout.buffer.flush()


async def _wait_for_control_or_failure(
    monitor: _RootPilotGuestStorageMonitor,
) -> bytes | None:
    loop = asyncio.get_running_loop()
    control: asyncio.Future[bytes | None] = loop.create_future()
    buffer = bytearray()

    def receive() -> None:
        if control.done():
            return
        try:
            chunk = os.read(sys.stdin.fileno(), 64)
        except OSError:
            control.set_result(None)
            return
        if not chunk:
            control.set_result(None)
            return
        buffer.extend(chunk)
        if len(buffer) > 64:
            control.set_result(bytes(buffer))
            return
        if b"\n" in buffer:
            control.set_result(bytes(buffer))

    reader_registered = False
    try:
        loop.add_reader(sys.stdin.fileno(), receive)
        reader_registered = True
    except PermissionError:
        # epoll rejects regular files and /dev/null with EPERM.  Never fall
        # back to a blocking read: an unsupported control FD is a closed
        # fail-safe channel, while /dev/null is observed as immediate EOF.
        try:
            os.set_blocking(sys.stdin.fileno(), False)
            receive()
        except (OSError, ValueError):
            if not control.done():
                control.set_result(None)
    termination: asyncio.Future[bytes | None] = loop.create_future()

    def terminate() -> None:
        if not termination.done():
            termination.set_result(None)

    for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
        loop.add_signal_handler(number, terminate)
    try:
        done, _ = await asyncio.wait(
            (control, termination, monitor.failure),
            return_when=asyncio.FIRST_COMPLETED,
        )
        if monitor.failure in done:
            return b"failed\n"
        if termination in done:
            return None
        return control.result()
    finally:
        if reader_registered:
            loop.remove_reader(sys.stdin.fileno())
        for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
            loop.remove_signal_handler(number)


async def _helper_main(arguments: dict[str, object]) -> int:
    helper_path = Path(__file__).resolve(strict=True)
    helper_sha256 = _file_sha256(helper_path)
    expected_sha256 = str(arguments["--helper-sha256"])
    container_id = str(arguments["--container-id"])
    if helper_sha256 != expected_sha256 or os.geteuid() != 0:
        try:
            await _root_docker_command(["docker", "kill", container_id])
        except Exception:
            pass
        return 70
    directory_fd = -1
    monitor: _RootPilotGuestStorageMonitor | None = None
    try:
        receipt_path = Path(str(arguments["--receipt-directory"]))
        directory_fd, metadata = _open_verified_receipt_directory(receipt_path)
        if (
            metadata.st_dev != arguments["--receipt-device"]
            or metadata.st_ino != arguments["--receipt-inode"]
            or metadata.st_uid != arguments["--receipt-owner-uid"]
            or metadata.st_gid != arguments["--receipt-owner-gid"]
        ):
            raise PilotGuestStorageFailure("receipt_directory_invalid")
        monitor = _RootPilotGuestStorageMonitor(
            phase=str(arguments["--phase"]),
            session_identity_sha256=str(arguments["--session-sha256"]),
            container_id=container_id,
            receipt_directory_fd=directory_fd,
            receipt_owner_uid=int(arguments["--receipt-owner-uid"]),
            receipt_owner_gid=int(arguments["--receipt-owner-gid"]),
            helper_sha256=helper_sha256,
        )
        try:
            await monitor.start()
        except PilotGuestStorageFailure:
            await monitor.finish()
            _emit_helper_event("failed", helper_sha256)
            return 70
        _emit_helper_event("ready", helper_sha256)
        control = await _wait_for_control_or_failure(monitor)
        if control == b"finish\n":
            failure = await monitor.finish()
        elif control == b"failed\n":
            failure = await monitor.finish()
        elif control is None:
            failure = await monitor.fail("control_channel_closed")
            await monitor.finish()
        else:
            failure = await monitor.fail("control_message_invalid")
            await monitor.finish()
        _emit_helper_event("finished" if failure is None else "failed", helper_sha256)
        return 0 if failure is None else 70
    except Exception:
        if monitor is not None:
            try:
                await monitor.fail("observation_failed")
                await monitor.finish()
            except Exception:
                pass
        else:
            try:
                await _root_docker_command(["docker", "kill", container_id])
            except Exception:
                pass
        _emit_helper_event("failed", helper_sha256)
        return 70
    finally:
        if directory_fd >= 0:
            os.close(directory_fd)


def _run_helper_from_argv() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == _PREFLIGHT_ENTRY:
        try:
            expected_sha256 = _parse_preflight_arguments(sys.argv[1:])
        except PilotGuestStorageFailure:
            return 64
        return asyncio.run(_guest_preflight_main(expected_sha256))
    try:
        arguments = _parse_helper_arguments(sys.argv[1:])
    except PilotGuestStorageFailure:
        return 64
    return asyncio.run(_helper_main(arguments))


if __name__ == "__main__":
    raise SystemExit(_run_helper_from_argv())
