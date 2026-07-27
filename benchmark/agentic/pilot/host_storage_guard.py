"""Non-formal macOS APFS storage wrapper for development pilot runs."""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import re
import signal
import stat
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Protocol, Sequence

sys.dont_write_bytecode = True


SCHEMA_VERSION = "agentic-bench/pilot-host-storage-guard-v1"
EVIDENCE_MODE = "diagnostic_unilateral"

GIB = 1 << 30
ADMISSION_MINIMUM_BYTES = 100 * GIB
WARNING_BELOW_BYTES = 50 * GIB
RUNTIME_HARD_FLOOR_BYTES = 30 * GIB
POLL_INTERVAL_NS = 1_000_000_000
MAX_COMPLETION_GAP_NS = 2_500_000_000

EXIT_USAGE = 64
EXIT_GUARD_FAILURE = 74
EXIT_ADMISSION_REJECTED = 78
TERMINATION_GRACE_SECONDS = 5.0

_ROLE_ORDER = ("controller_root", "private_root", "artifact_root")
_FORBIDDEN_STOP = re.compile(
    r"(?:^|[\s;&|()])(?:[^\s;&|()]*/)?limactl\s+stop(?:$|[\s;&|()])"
)


class GuardViolation(RuntimeError):
    """A stable machine-readable guard failure."""

    def __init__(self, code: str) -> None:
        super().__init__(code)
        self.code = code


class CLIUsageError(RuntimeError):
    """A stable machine-readable command-line contract failure."""


@dataclass(frozen=True)
class StorageReading:
    directory_device: int
    directory_inode: int
    directory_mode: int
    filesystem_id: tuple[int, int]
    filesystem_type: str
    block_size: int
    total_bytes: int
    available_bytes: int
    used_bytes: int


class StorageSampler(Protocol):
    def open_root(self, path: Path) -> int: ...

    def sample(self, fd: int) -> StorageReading: ...

    def close(self, fd: int) -> None: ...


class ProcessLike(Protocol):
    pid: int
    returncode: int | None

    def poll(self) -> int | None: ...

    def wait(self, timeout: float | None = None) -> int: ...


@dataclass(frozen=True)
class _PinnedRoot:
    role: str
    fd: int
    directory_identity: tuple[int, int, int]
    filesystem_identity: tuple[int, int, int, str]
    directory_identity_sha256: str
    filesystem_identity_sha256: str


class _DarwinFSID(ctypes.Structure):
    _fields_ = [("values", ctypes.c_int32 * 2)]


class _DarwinStatFS(ctypes.Structure):
    _fields_ = [
        ("f_bsize", ctypes.c_uint32),
        ("f_iosize", ctypes.c_int32),
        ("f_blocks", ctypes.c_uint64),
        ("f_bfree", ctypes.c_uint64),
        ("f_bavail", ctypes.c_uint64),
        ("f_files", ctypes.c_uint64),
        ("f_ffree", ctypes.c_uint64),
        ("f_fsid", _DarwinFSID),
        ("f_owner", ctypes.c_uint32),
        ("f_type", ctypes.c_uint32),
        ("f_flags", ctypes.c_uint32),
        ("f_fssubtype", ctypes.c_uint32),
        ("f_fstypename", ctypes.c_char * 16),
        ("f_mntonname", ctypes.c_char * 1024),
        ("f_mntfromname", ctypes.c_char * 1024),
        ("f_flags_ext", ctypes.c_uint32),
        ("f_reserved", ctypes.c_uint32 * 7),
    ]


def _darwin_filesystem_identity(fd: int) -> tuple[tuple[int, int], str]:
    filesystem = _DarwinStatFS()
    libc = ctypes.CDLL(None, use_errno=True)
    fstatfs = libc.fstatfs
    fstatfs.argtypes = (ctypes.c_int, ctypes.POINTER(_DarwinStatFS))
    fstatfs.restype = ctypes.c_int
    if fstatfs(fd, ctypes.byref(filesystem)) != 0:
        error_number = ctypes.get_errno()
        raise OSError(error_number, os.strerror(error_number))
    filesystem_type = bytes(filesystem.f_fstypename).split(b"\0", 1)[0]
    return (
        (int(filesystem.f_fsid.values[0]), int(filesystem.f_fsid.values[1])),
        filesystem_type.decode("ascii", errors="strict").lower(),
    )


class DarwinFDStorageSampler:
    """Samples a pinned directory with fresh fstat/fstatvfs/fstatfs calls."""

    def __init__(
        self,
        *,
        filesystem_identity: Callable[
            [int], tuple[tuple[int, int], str]
        ] = _darwin_filesystem_identity,
    ) -> None:
        self._filesystem_identity = filesystem_identity

    def open_root(self, path: Path) -> int:
        flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
        flags |= getattr(os, "O_CLOEXEC", 0)
        return os.open(path, flags)

    def sample(self, fd: int) -> StorageReading:
        directory = os.fstat(fd)
        counters = os.fstatvfs(fd)
        filesystem_id, filesystem_type = self._filesystem_identity(fd)

        block_size = int(counters.f_frsize or counters.f_bsize)
        blocks = int(counters.f_blocks)
        free_blocks = int(counters.f_bfree)
        available_blocks = int(counters.f_bavail)
        if (
            block_size <= 0
            or blocks < 0
            or free_blocks < 0
            or available_blocks < 0
            or free_blocks > blocks
            or available_blocks > blocks
        ):
            raise OSError("invalid_statvfs_counters")

        total_bytes = blocks * block_size
        available_bytes = available_blocks * block_size
        used_bytes = (blocks - free_blocks) * block_size
        return StorageReading(
            directory_device=int(directory.st_dev),
            directory_inode=int(directory.st_ino),
            directory_mode=int(directory.st_mode),
            filesystem_id=filesystem_id,
            filesystem_type=filesystem_type,
            block_size=block_size,
            total_bytes=total_bytes,
            available_bytes=available_bytes,
            used_bytes=used_bytes,
        )

    def close(self, fd: int) -> None:
        os.close(fd)


def _canonical_sha256(value: object) -> str:
    encoded = json.dumps(
        value, ensure_ascii=True, separators=(",", ":"), sort_keys=True
    ).encode("ascii")
    return hashlib.sha256(encoded).hexdigest()


def _directory_identity(reading: StorageReading) -> tuple[int, int, int]:
    return (
        reading.directory_device,
        reading.directory_inode,
        stat.S_IFMT(reading.directory_mode),
    )


def _filesystem_identity(reading: StorageReading) -> tuple[int, int, int, str]:
    return (
        reading.directory_device,
        reading.filesystem_id[0],
        reading.filesystem_id[1],
        reading.filesystem_type.lower(),
    )


def _default_process_launcher(
    command: Sequence[str], *, start_new_session: bool
) -> ProcessLike:
    return subprocess.Popen(list(command), start_new_session=start_new_session)


def _atomic_write_json(path: Path, value: object) -> None:
    raw = json.dumps(
        value, ensure_ascii=True, separators=(",", ":"), sort_keys=True
    ).encode("ascii") + b"\n"
    temporary_fd = -1
    temporary_name = ""
    try:
        temporary_fd, temporary_name = tempfile.mkstemp(
            prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
        )
        os.fchmod(temporary_fd, 0o600)
        offset = 0
        while offset < len(raw):
            offset += os.write(temporary_fd, raw[offset:])
        os.fsync(temporary_fd)
        os.close(temporary_fd)
        temporary_fd = -1
        os.replace(temporary_name, path)
        temporary_name = ""
        directory_fd = os.open(
            path.parent,
            os.O_RDONLY
            | getattr(os, "O_DIRECTORY", 0)
            | getattr(os, "O_CLOEXEC", 0),
        )
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if temporary_fd >= 0:
            os.close(temporary_fd)
        if temporary_name:
            try:
                os.unlink(temporary_name)
            except FileNotFoundError:
                pass


class PilotHostStorageGuard:
    """Runs one Lima shell command under a non-formal APFS storage guard."""

    def __init__(
        self,
        *,
        controller_root: Path,
        private_root: Path,
        artifact_root: Path,
        receipt_path: Path,
        sampler: StorageSampler | None = None,
        process_launcher: Callable[..., ProcessLike] = _default_process_launcher,
        signal_process_group: Callable[[int, int], None] = os.killpg,
        monotonic_ns: Callable[[], int] = time.monotonic_ns,
        wall_time_ns: Callable[[], int] = time.time_ns,
        platform_name: str = sys.platform,
    ) -> None:
        self._roots = {
            "controller_root": controller_root,
            "private_root": private_root,
            "artifact_root": artifact_root,
        }
        self._receipt_path = receipt_path
        self._sampler = sampler or DarwinFDStorageSampler()
        self._process_launcher = process_launcher
        self._signal_process_group = signal_process_group
        self._monotonic_ns = monotonic_ns
        self._wall_time_ns = wall_time_ns
        self._platform_name = platform_name
        self._pins: list[_PinnedRoot] = []
        self._receipt: dict[str, object] = {}
        self._last_completion_ns: int | None = None
        self._warned_filesystems: set[str] = set()
        self._admission_passed = False

    def run(self, command: Sequence[str]) -> int:
        process: ProcessLike | None = None
        self._initialize_receipt(command)
        try:
            self._validate_receipt_target()
            self._validate_static_inputs(command)
            initial_readings = self._pin_roots()
            initial_sample = self._record_sample(initial_readings)
            if self._minimum_available(initial_sample) < ADMISSION_MINIMUM_BYTES:
                raise GuardViolation("admission_storage_below_minimum")
            self._admission_passed = True
            self._receipt["admission_passed"] = True

            try:
                process = self._process_launcher(
                    tuple(command), start_new_session=True
                )
            except Exception as error:
                raise GuardViolation("child_launch_failed") from error
            self._receipt["process_started"] = True
            self._receipt["child_pid_sha256"] = _canonical_sha256(process.pid)

            violation = self._monitor(process)
            if violation is not None:
                return self._finish_failure(process, violation.code)

            self._receipt["child_exit_code"] = process.returncode
            self._terminalize("completed", "child_exited")
            self._write_receipt()
            return _process_exit_code(process.returncode)
        except KeyboardInterrupt:
            self._receipt["interrupted"] = True
            return self._finish_failure(process, "wrapper_interrupted")
        except GuardViolation as violation:
            return self._finish_failure(process, violation.code)
        except Exception:
            return self._finish_failure(process, "guard_internal_failure")
        finally:
            self._close_pins()

    def _initialize_receipt(self, command: Sequence[str]) -> None:
        self._last_completion_ns = None
        self._warned_filesystems.clear()
        self._admission_passed = False
        self._receipt = {
            "schema_version": SCHEMA_VERSION,
            "formal_compatible": False,
            "evidence_mode": EVIDENCE_MODE,
            "crash_proof": False,
            "limitations": ["non-crash-proof", "pilot-only"],
            "platform": self._platform_name,
            "required_filesystem_type": "apfs",
            "authority": "macos-pinned-fd-statfs-pilot-v1",
            "admission_minimum_bytes": ADMISSION_MINIMUM_BYTES,
            "warning_below_bytes": WARNING_BELOW_BYTES,
            "runtime_hard_floor_bytes": RUNTIME_HARD_FLOOR_BYTES,
            "poll_interval_ms": POLL_INTERVAL_NS // 1_000_000,
            "max_completion_gap_ms": MAX_COMPLETION_GAP_NS // 1_000_000,
            "command_argv_sha256": _canonical_sha256(list(command)),
            "started_unix_ns": self._wall_time_ns(),
            "finished_unix_ns": None,
            "status": "initializing",
            "reason_code": None,
            "admission_passed": False,
            "process_started": False,
            "child_pid_sha256": None,
            "child_exit_code": None,
            "warning": False,
            "warning_events": [],
            "kill_required": False,
            "kill_attempted": False,
            "kill_acknowledged": False,
            "sigterm_sent": False,
            "sigkill_sent": False,
            "kill_error_code": None,
            "interrupted": False,
            "root_bindings": [],
            "samples": [],
            "sample_count": 0,
        }

    def _validate_receipt_target(self) -> None:
        if not self._receipt_path.is_absolute():
            raise GuardViolation("receipt_path_not_absolute")
        parent = self._receipt_path.parent
        if not parent.is_dir():
            raise GuardViolation("receipt_parent_not_existing_directory")

    def _validate_static_inputs(self, command: Sequence[str]) -> None:
        if self._platform_name != "darwin":
            raise GuardViolation("unsupported_platform")
        if len(command) < 2 or list(command[:2]) != ["limactl", "shell"]:
            raise GuardViolation("invalid_command_prefix")
        for index in range(len(command) - 1):
            if Path(command[index]).name == "limactl" and command[index + 1] == "stop":
                raise GuardViolation("forbidden_limactl_stop")
        if any(_FORBIDDEN_STOP.search(argument) for argument in command):
            raise GuardViolation("forbidden_limactl_stop")
        for role in _ROLE_ORDER:
            root = self._roots[role]
            if not root.is_absolute():
                raise GuardViolation("root_path_not_absolute")
            if not root.is_dir():
                raise GuardViolation("root_not_existing_directory")

    def _pin_roots(self) -> dict[str, StorageReading]:
        readings: dict[str, StorageReading] = {}
        try:
            for role in _ROLE_ORDER:
                try:
                    fd = self._sampler.open_root(self._roots[role])
                except Exception as error:
                    raise GuardViolation("root_open_failed") from error
                try:
                    reading = self._sampler.sample(fd)
                    self._validate_initial_reading(reading)
                except Exception:
                    self._sampler.close(fd)
                    raise
                directory_identity = _directory_identity(reading)
                filesystem_identity = _filesystem_identity(reading)
                pin = _PinnedRoot(
                    role=role,
                    fd=fd,
                    directory_identity=directory_identity,
                    filesystem_identity=filesystem_identity,
                    directory_identity_sha256=_canonical_sha256(
                        ["directory", *directory_identity]
                    ),
                    filesystem_identity_sha256=_canonical_sha256(
                        ["filesystem", *filesystem_identity]
                    ),
                )
                self._pins.append(pin)
                readings[role] = reading
            self._receipt["root_bindings"] = [
                {
                    "role": pin.role,
                    "directory_identity_sha256": pin.directory_identity_sha256,
                    "filesystem_identity_sha256": pin.filesystem_identity_sha256,
                }
                for pin in self._pins
            ]
            return readings
        except GuardViolation:
            raise
        except Exception as error:
            raise GuardViolation("storage_sample_syscall_failed") from error

    @staticmethod
    def _validate_initial_reading(reading: StorageReading) -> None:
        if not stat.S_ISDIR(reading.directory_mode):
            raise GuardViolation("pinned_root_not_directory")
        if reading.filesystem_type.lower() != "apfs":
            raise GuardViolation("filesystem_not_apfs")
        _validate_counters(reading)

    def _read_pins(self) -> dict[str, StorageReading]:
        readings: dict[str, StorageReading] = {}
        for pin in self._pins:
            try:
                reading = self._sampler.sample(pin.fd)
            except Exception as error:
                raise GuardViolation("storage_sample_syscall_failed") from error
            self._validate_initial_reading(reading)
            if _directory_identity(reading) != pin.directory_identity:
                raise GuardViolation("pinned_root_identity_changed")
            if _filesystem_identity(reading) != pin.filesystem_identity:
                raise GuardViolation("filesystem_identity_changed")
            readings[pin.role] = reading
        return readings

    def _record_sample(self, readings: dict[str, StorageReading]) -> dict[str, object]:
        filesystems: dict[str, dict[str, object]] = {}
        for pin in self._pins:
            reading = readings[pin.role]
            group = filesystems.get(pin.filesystem_identity_sha256)
            if group is None:
                group = {
                    "identity_sha256": pin.filesystem_identity_sha256,
                    "roles": [],
                    "filesystem_type": reading.filesystem_type.lower(),
                    "block_size": reading.block_size,
                    "total_bytes": reading.total_bytes,
                    "available_bytes": reading.available_bytes,
                    "used_bytes": reading.used_bytes,
                }
                filesystems[pin.filesystem_identity_sha256] = group
            elif (
                group["block_size"] != reading.block_size
                or group["total_bytes"] != reading.total_bytes
            ):
                raise GuardViolation("filesystem_counter_mismatch")
            else:
                group["available_bytes"] = min(
                    int(group["available_bytes"]), reading.available_bytes
                )
                group["used_bytes"] = max(
                    int(group["used_bytes"]), reading.used_bytes
                )
            roles = group["roles"]
            if not isinstance(roles, list):
                raise GuardViolation("guard_internal_failure")
            roles.append(pin.role)

        completed_monotonic_ns = self._monotonic_ns()
        completed_unix_ns = self._wall_time_ns()
        completion_gap_ns = None
        if self._last_completion_ns is not None:
            completion_gap_ns = completed_monotonic_ns - self._last_completion_ns
            if completion_gap_ns < 0:
                raise GuardViolation("monotonic_clock_regressed")

        ordered_filesystems = sorted(
            filesystems.values(), key=lambda item: str(item["identity_sha256"])
        )
        for filesystem in ordered_filesystems:
            filesystem["roles"] = sorted(filesystem["roles"])
        samples = self._receipt["samples"]
        if not isinstance(samples, list):
            raise GuardViolation("guard_internal_failure")
        sample: dict[str, object] = {
            "index": len(samples),
            "completed_monotonic_ns": completed_monotonic_ns,
            "completed_unix_ns": completed_unix_ns,
            "completion_gap_ns": completion_gap_ns,
            "filesystems": ordered_filesystems,
        }
        samples.append(sample)
        self._receipt["sample_count"] = len(samples)
        self._last_completion_ns = completed_monotonic_ns

        for filesystem in ordered_filesystems:
            available_bytes = int(filesystem["available_bytes"])
            identity = str(filesystem["identity_sha256"])
            if (
                available_bytes < WARNING_BELOW_BYTES
                and identity not in self._warned_filesystems
            ):
                self._warned_filesystems.add(identity)
                self._receipt["warning"] = True
                warning_events = self._receipt["warning_events"]
                if not isinstance(warning_events, list):
                    raise GuardViolation("guard_internal_failure")
                warning_events.append(
                    {
                        "code": "storage_below_warning_threshold",
                        "sample_index": sample["index"],
                        "filesystem_identity_sha256": identity,
                        "available_bytes": available_bytes,
                    }
                )
        return sample

    @staticmethod
    def _minimum_available(sample: dict[str, object]) -> int:
        filesystems = sample["filesystems"]
        if not isinstance(filesystems, list) or not filesystems:
            raise GuardViolation("no_filesystems_sampled")
        return min(int(filesystem["available_bytes"]) for filesystem in filesystems)

    def _monitor(self, process: ProcessLike) -> GuardViolation | None:
        if self._last_completion_ns is None:
            return GuardViolation("no_initial_sample")
        next_sample_ns = self._last_completion_ns + POLL_INTERVAL_NS
        while process.poll() is None:
            now_ns = self._monotonic_ns()
            timeout_seconds = max(0, next_sample_ns - now_ns) / 1_000_000_000
            try:
                process.wait(timeout=timeout_seconds)
            except subprocess.TimeoutExpired:
                pass
            if process.poll() is not None:
                break
            try:
                sample = self._record_sample(self._read_pins())
            except GuardViolation as violation:
                return violation
            gap = sample["completion_gap_ns"]
            if isinstance(gap, int) and gap > MAX_COMPLETION_GAP_NS:
                return GuardViolation("sample_completion_gap_exceeded")
            if self._minimum_available(sample) < RUNTIME_HARD_FLOOR_BYTES:
                return GuardViolation("runtime_storage_floor_breached")
            if self._last_completion_ns is None:
                return GuardViolation("no_sample_completion")
            next_sample_ns = self._last_completion_ns + POLL_INTERVAL_NS

        try:
            final_sample = self._record_sample(self._read_pins())
        except GuardViolation as violation:
            return violation
        final_gap = final_sample["completion_gap_ns"]
        if isinstance(final_gap, int) and final_gap > MAX_COMPLETION_GAP_NS:
            return GuardViolation("sample_completion_gap_exceeded")
        if self._minimum_available(final_sample) < RUNTIME_HARD_FLOOR_BYTES:
            return GuardViolation("runtime_storage_floor_breached")
        return None

    def _finish_failure(self, process: ProcessLike | None, code: str) -> int:
        status = "guard_failed" if self._admission_passed else "admission_rejected"
        running = process is not None and process.poll() is None
        if process is not None:
            self._receipt["child_exit_code"] = process.returncode
        self._receipt["kill_required"] = running
        self._receipt["kill_attempted"] = False
        self._receipt["kill_acknowledged"] = process is not None and not running
        self._terminalize(status, code)
        self._write_receipt()

        if not running or process is None:
            return (
                EXIT_GUARD_FAILURE
                if self._admission_passed
                else EXIT_ADMISSION_REJECTED
            )

        self._receipt["kill_attempted"] = True
        try:
            self._signal_process_group(process.pid, signal.SIGTERM)
            self._receipt["sigterm_sent"] = True
        except ProcessLookupError:
            self._receipt["kill_acknowledged"] = process.poll() is not None
        except Exception:
            self._receipt["kill_error_code"] = "sigterm_failed"

        if process.poll() is None:
            try:
                process.wait(timeout=TERMINATION_GRACE_SECONDS)
            except subprocess.TimeoutExpired:
                try:
                    self._signal_process_group(process.pid, signal.SIGKILL)
                    self._receipt["sigkill_sent"] = True
                except ProcessLookupError:
                    pass
                except Exception:
                    self._receipt["kill_error_code"] = "sigkill_failed"
                if process.poll() is None:
                    try:
                        process.wait(timeout=TERMINATION_GRACE_SECONDS)
                    except subprocess.TimeoutExpired:
                        pass

        self._receipt["child_exit_code"] = process.returncode
        self._receipt["kill_acknowledged"] = process.poll() is not None
        self._write_receipt()
        return EXIT_GUARD_FAILURE

    def _terminalize(self, status: str, reason_code: str) -> None:
        self._receipt["status"] = status
        self._receipt["reason_code"] = reason_code
        self._receipt["finished_unix_ns"] = self._wall_time_ns()

    def _write_receipt(self) -> None:
        _atomic_write_json(self._receipt_path, self._receipt)

    def _close_pins(self) -> None:
        while self._pins:
            pin = self._pins.pop()
            try:
                self._sampler.close(pin.fd)
            except Exception:
                pass


def _validate_counters(reading: StorageReading) -> None:
    if (
        reading.block_size <= 0
        or reading.total_bytes < 0
        or reading.available_bytes < 0
        or reading.used_bytes < 0
        or reading.available_bytes > reading.total_bytes
        or reading.used_bytes > reading.total_bytes
    ):
        raise GuardViolation("invalid_filesystem_counters")


def _process_exit_code(returncode: int | None) -> int:
    if returncode is None:
        return EXIT_GUARD_FAILURE
    if returncode < 0:
        return min(255, 128 + abs(returncode))
    return min(255, returncode)


def _parse_arguments(argv: Sequence[str]) -> tuple[argparse.Namespace, list[str]]:
    raw = list(argv)
    try:
        separator = raw.index("--")
    except ValueError as error:
        raise CLIUsageError("command_separator_required") from error
    parser = argparse.ArgumentParser()
    parser.add_argument("--controller-root", required=True, type=Path)
    parser.add_argument("--private-root", required=True, type=Path)
    parser.add_argument("--artifact-root", required=True, type=Path)
    parser.add_argument("--receipt", required=True, type=Path)
    arguments = parser.parse_args(raw[:separator])
    command = raw[separator + 1 :]
    if not command:
        raise CLIUsageError("command_required")
    return arguments, command


def main(argv: Sequence[str] | None = None) -> int:
    try:
        arguments, command = _parse_arguments(
            sys.argv[1:] if argv is None else argv
        )
    except CLIUsageError as error:
        sys.stderr.write(f"pilot_host_storage_guard:{error}\n")
        return EXIT_USAGE
    guard = PilotHostStorageGuard(
        controller_root=arguments.controller_root,
        private_root=arguments.private_root,
        artifact_root=arguments.artifact_root,
        receipt_path=arguments.receipt,
    )
    return guard.run(command)


if __name__ == "__main__":
    raise SystemExit(main())
