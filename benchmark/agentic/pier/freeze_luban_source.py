#!/usr/bin/env python3
"""Freeze and build the exact Luban source identity consumed by the harness."""

from __future__ import annotations

import ctypes
import datetime as dt
import errno
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
from typing import NoReturn


_SCHEMA = "agentic-bench/luban-source-freeze-v1"
_ERROR_SCHEMA = "agentic-bench/luban-source-freeze-error-v1"
_RESULT_SCHEMA = "agentic-bench/luban-source-freeze-result-v1"
_BUILD_RECEIPT_SCHEMA = "agentic-bench/agent-build-receipt-v2"
_EXCLUSION_RECEIPT_SCHEMA = "agentic-bench/source-exclusion-receipt-v1"
_PATH_POLICY = {
    "schema_version": "agentic-bench/source-path-policy-v1",
    "excluded_prefixes": [
        ".agentic-bench/",
        ".codex/",
        ".luban/",
        ".tmp/",
        "benchmark-artifacts/",
        "benchmark-results/",
    ],
}
_HEX40 = re.compile(r"[0-9a-f]{40}\Z")
_RFC3339_UTC = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\Z")
_AT_FDCWD = -2
_RENAME_NOREPLACE = 1
_RENAME_EXCL = 0x00000004


class FreezeError(Exception):
    def __init__(self, code: str, *, operation: str = "", exit_code: int | None = None):
        super().__init__(code)
        self.code = code
        self.operation = operation
        self.exit_code = exit_code


def _canonical_json(value: object) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, separators=(",", ":"), sort_keys=False
    ).encode("utf-8")


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        while chunk := source.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def _canonical_tree_sha256(root: Path) -> str:
    records: list[tuple[str, str, int, str]] = []

    def walk_error(error: OSError) -> NoReturn:
        raise FreezeError("toolchain_tree_traversal_failed") from error

    try:
        root_info = root.lstat()
    except OSError as error:
        raise FreezeError("toolchain_tree_traversal_failed") from error
    records.append(
        (
            ".",
            f"04{stat.S_IMODE(root_info.st_mode):04o}",
            0,
            _sha256(b"directory\x00"),
        )
    )
    for directory, directory_names, file_names in os.walk(
        root, topdown=True, onerror=walk_error, followlinks=False
    ):
        directory_path = Path(directory)
        retained_directories: list[str] = []
        for name in directory_names:
            path = directory_path / name
            try:
                info = path.lstat()
            except OSError as error:
                raise FreezeError("toolchain_tree_traversal_failed") from error
            if stat.S_ISLNK(info.st_mode):
                target = os.readlink(path).encode("utf-8", "surrogateescape")
                content_sha = _sha256(b"symlink\x00" + target)
                records.append(
                    (
                        path.relative_to(root).as_posix(),
                        "120000",
                        len(target),
                        content_sha,
                    )
                )
            elif stat.S_ISDIR(info.st_mode):
                records.append(
                    (
                        path.relative_to(root).as_posix(),
                        f"04{stat.S_IMODE(info.st_mode):04o}",
                        0,
                        _sha256(b"directory\x00"),
                    )
                )
                retained_directories.append(name)
            else:
                raise FreezeError("toolchain_tree_member_unsupported")
        directory_names[:] = retained_directories
        for name in file_names:
            path = directory_path / name
            try:
                info = path.lstat()
            except OSError as error:
                raise FreezeError("toolchain_tree_traversal_failed") from error
            relative = path.relative_to(root).as_posix()
            if stat.S_ISLNK(info.st_mode):
                target = os.readlink(path).encode("utf-8", "surrogateescape")
                records.append(
                    (
                        relative,
                        "120000",
                        len(target),
                        _sha256(b"symlink\x00" + target),
                    )
                )
            elif stat.S_ISREG(info.st_mode):
                records.append(
                    (
                        relative,
                        "0755" if info.st_mode & 0o111 else "0644",
                        info.st_size,
                        _file_sha256(path),
                    )
                )
            else:
                raise FreezeError("toolchain_tree_member_unsupported")
    digest = hashlib.sha256()
    for relative, mode, size, content_sha in sorted(records):
        digest.update(relative.encode("utf-8", "surrogateescape"))
        digest.update(b"\x00")
        digest.update(mode.encode("ascii"))
        digest.update(b"\x00")
        digest.update(str(size).encode("ascii"))
        digest.update(b"\x00")
        digest.update(content_sha.encode("ascii"))
        digest.update(b"\n")
    return digest.hexdigest()


def _clean_git_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for name in (
        "GIT_INDEX_FILE",
        "GIT_OBJECT_DIRECTORY",
        "GIT_ALTERNATE_OBJECT_DIRECTORIES",
    ):
        environment.pop(name, None)
    return environment


def _run(
    argv: list[str],
    *,
    cwd: Path,
    environment: dict[str, str],
    operation: str,
    combine_stderr: bool = False,
) -> bytes:
    try:
        completed = subprocess.run(
            argv,
            cwd=cwd,
            env=environment,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT if combine_stderr else subprocess.PIPE,
            check=False,
        )
    except OSError as error:
        raise FreezeError(operation, operation=operation, exit_code=error.errno) from error
    if completed.returncode != 0:
        raise FreezeError(
            operation, operation=operation, exit_code=completed.returncode
        )
    return completed.stdout


def _run_git(
    worktree: Path,
    environment: dict[str, str],
    operation: str,
    *arguments: str,
) -> bytes:
    return _run(
        ["git", *arguments],
        cwd=worktree,
        environment=environment,
        operation=operation,
        combine_stderr=True,
    )


def _path_policy_sha256() -> str:
    return _sha256(_canonical_json(_PATH_POLICY))


def _exclusion_receipt() -> tuple[bytes, str]:
    raw = _canonical_json(
        {
            "schema_version": _EXCLUSION_RECEIPT_SCHEMA,
            "path_policy": _PATH_POLICY,
            "path_policy_sha256": _path_policy_sha256(),
            "applied": True,
            "implementation": "git-negative-pathspec-before-content-scan-v1",
        }
    ) + b"\n"
    return raw, _sha256(raw)


def _source_add_arguments() -> list[str]:
    arguments = ["add", "-A", "--", "."]
    for prefix in _PATH_POLICY["excluded_prefixes"]:
        path = prefix.removesuffix("/")
        arguments.extend((f":(exclude){path}", f":(exclude,glob){path}/**"))
    return arguments


def _validate_base_exclusions(worktree: Path, base_commit: str) -> None:
    arguments = ["ls-tree", "-r", "--name-only", "-z", base_commit, "--"]
    arguments.extend(prefix.removesuffix("/") for prefix in _PATH_POLICY["excluded_prefixes"])
    paths = _run_git(
        worktree,
        _clean_git_environment(),
        "base_tree_exclusion_check_failed",
        *arguments,
    )
    if paths.strip(b"\x00 \t\r\n"):
        raise FreezeError("base_tree_contains_excluded_path")


def _capture_source(worktree: Path, base_commit: str) -> dict[str, object]:
    clean_environment = _clean_git_environment()
    head = _run_git(
        worktree,
        clean_environment,
        "head_resolution_failed",
        "rev-parse",
        "HEAD^{commit}",
    ).decode("ascii").strip()
    if head != base_commit:
        raise FreezeError("base_commit_mismatch")
    _validate_base_exclusions(worktree, base_commit)

    with tempfile.TemporaryDirectory(prefix="agentic-bench-source-") as temporary:
        temporary_root = Path(temporary)
        object_directory = temporary_root / "objects"
        object_directory.mkdir(mode=0o700)
        base_objects = _run_git(
            worktree,
            clean_environment,
            "object_directory_resolution_failed",
            "rev-parse",
            "--path-format=absolute",
            "--git-path",
            "objects",
        ).decode("utf-8").strip()
        environment = dict(clean_environment)
        environment.update(
            {
                "GIT_INDEX_FILE": str(temporary_root / "index"),
                "GIT_OBJECT_DIRECTORY": str(object_directory),
                "GIT_ALTERNATE_OBJECT_DIRECTORIES": base_objects,
            }
        )
        _run_git(
            worktree,
            environment,
            "temporary_index_initialization_failed",
            "read-tree",
            base_commit,
        )
        _run_git(
            worktree,
            environment,
            "source_capture_failed",
            *_source_add_arguments(),
        )
        for prefix in _PATH_POLICY["excluded_prefixes"]:
            _run_git(
                worktree,
                environment,
                "source_exclusion_failed",
                "rm",
                "-r",
                "-f",
                "--cached",
                "--ignore-unmatch",
                "--",
                prefix.removesuffix("/"),
            )
        tree_oid = _run_git(
            worktree,
            environment,
            "tree_write_failed",
            "write-tree",
        ).decode("ascii").strip()
        patch = _run_git(
            worktree,
            environment,
            "patch_generation_failed",
            "diff",
            "--cached",
            "--binary",
            "--full-index",
            "--no-ext-diff",
            base_commit,
            "--",
        )
        archive = _run_git(
            worktree,
            environment,
            "archive_generation_failed",
            "archive",
            "--format=tar",
            "--mtime=1970-01-01T00:00:00Z",
            tree_oid,
        )
    exclusion_raw, exclusion_sha256 = _exclusion_receipt()
    return {
        "base_commit": base_commit,
        "tree_oid": tree_oid,
        "patch": patch,
        "patch_sha256": _sha256(patch),
        "archive": archive,
        "archive_sha256": _sha256(archive),
        "path_policy_sha256": _path_policy_sha256(),
        "exclusion_receipt": exclusion_raw,
        "exclusion_receipt_sha256": exclusion_sha256,
    }


def _safe_extract_source(archive: bytes, destination: Path) -> None:
    destination.mkdir(mode=0o700)
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as source:
        for member in source:
            relative = PurePosixPath(member.name)
            if (
                not member.name
                or relative.is_absolute()
                or any(part in ("", ".", "..") for part in relative.parts)
            ):
                raise FreezeError("archive_path_invalid")
            target = destination.joinpath(*relative.parts)
            try:
                target.relative_to(destination)
            except ValueError as error:
                raise FreezeError("archive_path_escape") from error
            if member.isdir():
                target.mkdir(mode=0o755, parents=True, exist_ok=True)
                continue
            if not member.isfile():
                raise FreezeError("archive_member_type_unsupported")
            target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
            extracted = source.extractfile(member)
            if extracted is None:
                raise FreezeError("archive_member_missing")
            try:
                descriptor = os.open(
                    target,
                    os.O_WRONLY | os.O_CREAT | os.O_EXCL,
                    0o755 if member.mode & 0o111 else 0o644,
                )
            except OSError as error:
                raise FreezeError("archive_member_create_failed") from error
            with os.fdopen(descriptor, "wb") as output:
                shutil.copyfileobj(extracted, output, length=1024 * 1024)
            if target.stat().st_size != member.size:
                raise FreezeError("archive_member_size_mismatch")


def _go_cache_paths(go_binary: Path, worktree: Path) -> tuple[Path, Path, Path]:
    try:
        user_home = Path.home().resolve(strict=True)
    except OSError as error:
        raise FreezeError("user_home_resolution_failed") from error
    argv = [
        "/usr/bin/env",
        "-i",
        "GO111MODULE=on",
        "GOENV=off",
        "GOTOOLCHAIN=local",
        f"HOME={user_home}",
        "LC_ALL=C",
        "TZ=UTC",
        str(go_binary),
        "env",
        "GOMODCACHE",
        "GOCACHE",
    ]
    raw = _run(
        argv,
        cwd=worktree,
        environment={},
        operation="go_cache_resolution_failed",
    ).decode("utf-8")
    lines = raw.splitlines()
    if len(lines) != 2:
        raise FreezeError("go_cache_resolution_invalid")
    paths: list[Path] = []
    for value in lines:
        path = Path(value)
        if not path.is_absolute():
            raise FreezeError("go_cache_path_invalid")
        resolved = path.resolve(strict=False)
        try:
            resolved.relative_to(worktree)
        except ValueError:
            pass
        else:
            raise FreezeError("go_cache_inside_worktree")
        paths.append(resolved)
    if not paths[0].is_dir():
        raise FreezeError("go_module_cache_missing")
    return user_home, paths[0], paths[1]


def _toolchain_identity(
    go_binary: Path, user_home: Path, worktree: Path
) -> dict[str, object]:
    raw = _run(
        [
            "/usr/bin/env",
            "-i",
            "GOENV=off",
            "GOTOOLCHAIN=local",
            f"HOME={user_home}",
            "LC_ALL=C",
            "TZ=UTC",
            str(go_binary),
            "env",
            "GOROOT",
            "GOTOOLDIR",
        ],
        cwd=worktree,
        environment={},
        operation="toolchain_root_resolution_failed",
    ).decode("utf-8")
    lines = raw.splitlines()
    if len(lines) != 2:
        raise FreezeError("toolchain_root_resolution_invalid")
    try:
        go_root = Path(lines[0]).resolve(strict=True)
        tool_directory = Path(lines[1]).resolve(strict=True)
    except OSError as error:
        raise FreezeError("toolchain_root_invalid") from error
    if not go_root.is_dir() or not tool_directory.is_dir() or not _is_within(
        tool_directory, go_root
    ):
        raise FreezeError("toolchain_root_invalid")
    tools: dict[str, dict[str, str]] = {}
    for name in ("asm", "compile", "link"):
        path = (tool_directory / name).resolve(strict=True)
        if not path.is_file() or not os.access(path, os.X_OK):
            raise FreezeError("toolchain_tool_invalid")
        version = _run(
            [str(path), "-V=full"],
            cwd=worktree,
            environment={},
            operation="toolchain_tool_version_failed",
        ).decode("utf-8").strip()
        if not version:
            raise FreezeError("toolchain_tool_version_empty")
        tools[name] = {
            "path": str(path),
            "sha256": _file_sha256(path),
            "version": version,
        }
    go_version = _run(
        [
            "/usr/bin/env",
            "-i",
            "GOENV=off",
            "GOTOOLCHAIN=local",
            "LC_ALL=C",
            "TZ=UTC",
            str(go_binary),
            "version",
        ],
        cwd=worktree,
        environment={},
        operation="toolchain_identity_failed",
    ).decode("utf-8").strip()
    if not go_version:
        raise FreezeError("toolchain_identity_empty")
    return {
        "schema_version": "agentic-bench/go-toolchain-identity-v1",
        "go_binary": {
            "path": str(go_binary),
            "sha256": _file_sha256(go_binary),
            "version": go_version,
        },
        "go_root": str(go_root),
        "go_root_tree_sha256": _canonical_tree_sha256(go_root),
        "tools": tools,
    }


def _go_build_environment(
    user_home: Path, module_cache: Path, build_cache: Path
) -> list[str]:
    return [
        "CGO_ENABLED=0",
        "GO111MODULE=on",
        "GOARCH=amd64",
        "GOAMD64=v1",
        "GOAUTH=off",
        f"GOCACHE={build_cache}",
        "GOCACHEPROG=",
        "GODEBUG=gocacheverify=1",
        "GOENV=off",
        "GOEXPERIMENT=",
        "GOFIPS140=off",
        "GOFLAGS=",
        "GOINSECURE=",
        f"GOMODCACHE={module_cache}",
        "GONOPROXY=",
        "GONOSUMDB=",
        "GOPRIVATE=",
        "GOPROXY=off",
        "GOSUMDB=off",
        "GOTOOLCHAIN=local",
        "GOTELEMETRY=off",
        "GOVCS=*:off",
        "GOWORK=off",
        "GOOS=linux",
        f"HOME={user_home}",
        "LANG=C",
        "LC_ALL=C",
        "SOURCE_DATE_EPOCH=0",
        "TZ=UTC",
    ]


def _build_argv(
    go_binary: Path,
    user_home: Path,
    module_cache: Path,
    build_cache: Path,
) -> list[str]:
    return [
        "/usr/bin/env",
        "-i",
        *_go_build_environment(user_home, module_cache, build_cache),
        str(go_binary),
        "build",
        "-buildmode=exe",
        "-buildvcs=false",
        "-mod=readonly",
        "-trimpath",
        "-ldflags=-buildid= -s -w",
        "-o",
        "../binary/luban",
        "./cmd/luban-code",
    ]


def _is_within(path: Path, root: Path) -> bool:
    try:
        path.relative_to(root)
    except ValueError:
        return False
    return True


def _decode_json_stream(raw: bytes) -> list[object]:
    text = raw.decode("utf-8")
    decoder = json.JSONDecoder()
    offset = 0
    values: list[object] = []
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        try:
            value, offset = decoder.raw_decode(text, offset)
        except json.JSONDecodeError as error:
            raise FreezeError("module_graph_decode_failed") from error
        values.append(value)
    return values


def _normalize_module_graph_value(
    value: object, source_root: Path, module_cache: Path
) -> object:
    if isinstance(value, list):
        return [
            _normalize_module_graph_value(item, source_root, module_cache)
            for item in value
        ]
    if not isinstance(value, dict):
        return value
    normalized: dict[str, object] = {}
    for key, item in value.items():
        if key in {"Dir", "GoMod"} and isinstance(item, str):
            path = Path(item).resolve(strict=False)
            if _is_within(path, source_root):
                item = "{source_root}" + str(path)[len(str(source_root)) :]
            elif _is_within(path, module_cache):
                item = "{module_cache}" + str(path)[len(str(module_cache)) :]
        normalized[key] = _normalize_module_graph_value(
            item, source_root, module_cache
        )
    return normalized


def _verify_module_graph(
    source_root: Path,
    go_binary: Path,
    user_home: Path,
    module_cache: Path,
    build_cache: Path,
) -> str:
    command_prefix = [
        "/usr/bin/env",
        "-i",
        *_go_build_environment(user_home, module_cache, build_cache),
        str(go_binary),
    ]
    verification = _run(
        [*command_prefix, "mod", "verify"],
        cwd=source_root,
        environment={},
        operation="module_verification_failed",
    )
    if verification.strip() != b"all modules verified":
        raise FreezeError("module_verification_result_invalid")
    modules = _decode_json_stream(
        _run(
            [*command_prefix, "list", "-mod=readonly", "-m", "-json", "all"],
            cwd=source_root,
            environment={},
            operation="module_graph_resolution_failed",
        )
    )
    if not modules:
        raise FreezeError("module_graph_empty")
    for value in modules:
        if not isinstance(value, dict):
            raise FreezeError("module_graph_shape_invalid")
        replacement = value.get("Replace")
        if isinstance(replacement, dict) and not replacement.get("Version"):
            replacement_directory = replacement.get("Dir")
            if not isinstance(replacement_directory, str) or not _is_within(
                Path(replacement_directory).resolve(strict=True), source_root
            ):
                raise FreezeError("local_module_replace_outside_frozen_source")
        directory = value.get("Dir")
        if isinstance(directory, str) and directory:
            resolved = Path(directory).resolve(strict=True)
            if not (
                _is_within(resolved, source_root)
                or _is_within(resolved, module_cache)
            ):
                raise FreezeError("module_source_outside_frozen_roots")
    normalized = _normalize_module_graph_value(
        modules, source_root, module_cache
    )
    return hashlib.sha256(
        json.dumps(
            normalized,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    ).hexdigest()


def _build_frozen_archive(
    archive: bytes,
    go_binary: Path,
    expected_toolchain_identity: dict[str, object],
    user_home: Path,
    module_cache: Path,
    build_cache: Path,
    temporary_root: Path,
) -> tuple[Path, list[str], str]:
    source_root = temporary_root / "source"
    binary_root = temporary_root / "binary"
    binary_root.mkdir(mode=0o700)
    _safe_extract_source(archive, source_root)
    if _toolchain_identity(go_binary, user_home, source_root) != expected_toolchain_identity:
        raise FreezeError("go_toolchain_drift_during_freeze")
    module_graph_sha256 = _verify_module_graph(
        source_root, go_binary, user_home, module_cache, build_cache
    )
    argv = _build_argv(go_binary, user_home, module_cache, build_cache)
    _run(
        argv,
        cwd=source_root,
        environment={},
        operation="frozen_source_build_failed",
    )
    binary = binary_root / "luban"
    try:
        header = binary.read_bytes()[:20]
    except OSError as error:
        raise FreezeError("binary_read_failed") from error
    if (
        len(header) < 20
        or header[:4] != b"\x7fELF"
        or header[4] != 2
        or header[5] != 1
        or int.from_bytes(header[18:20], "little") != 62
    ):
        raise FreezeError("binary_target_mismatch")
    post_build_module_graph_sha256 = _verify_module_graph(
        source_root, go_binary, user_home, module_cache, build_cache
    )
    if post_build_module_graph_sha256 != module_graph_sha256:
        raise FreezeError("module_graph_drift_during_build")
    if _toolchain_identity(go_binary, user_home, source_root) != expected_toolchain_identity:
        raise FreezeError("go_toolchain_drift_during_freeze")
    toolchain_receipt = {
        "schema_version": "agentic-bench/go-build-toolchain-receipt-v1",
        "identity": expected_toolchain_identity,
        "target": "linux/amd64/v1",
        "cgo_enabled": False,
        "module_graph_sha256": module_graph_sha256,
        "module_cache_verified": True,
        "gocacheverify": True,
    }
    toolchain = json.dumps(
        toolchain_receipt,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return binary, argv, toolchain


def _write_exclusive(path: Path, value: bytes, mode: int) -> None:
    try:
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except OSError as error:
        raise FreezeError("artifact_create_failed") from error
    with os.fdopen(descriptor, "wb") as output:
        os.fchmod(output.fileno(), mode)
        output.write(value)
        output.flush()
        os.fsync(output.fileno())


def _copy_exclusive(source: Path, destination: Path, mode: int) -> None:
    try:
        descriptor = os.open(
            destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode
        )
    except OSError as error:
        raise FreezeError("artifact_create_failed") from error
    with source.open("rb") as input_file, os.fdopen(descriptor, "wb") as output:
        os.fchmod(output.fileno(), mode)
        shutil.copyfileobj(input_file, output, length=1024 * 1024)
        output.flush()
        os.fsync(output.fileno())


def _sync_directory(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def _rename_noreplace(source: Path, destination: Path) -> None:
    libc = ctypes.CDLL(None, use_errno=True)
    source_bytes = os.fsencode(source)
    destination_bytes = os.fsencode(destination)
    if sys.platform == "darwin" and hasattr(libc, "renameatx_np"):
        function = libc.renameatx_np
        function.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        function.restype = ctypes.c_int
        result = function(
            _AT_FDCWD,
            source_bytes,
            _AT_FDCWD,
            destination_bytes,
            _RENAME_EXCL,
        )
    elif sys.platform.startswith("linux") and hasattr(libc, "renameat2"):
        function = libc.renameat2
        function.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        function.restype = ctypes.c_int
        result = function(
            _AT_FDCWD,
            source_bytes,
            _AT_FDCWD,
            destination_bytes,
            _RENAME_NOREPLACE,
        )
    else:
        raise FreezeError("atomic_noreplace_unsupported")
    if result != 0:
        error_number = ctypes.get_errno()
        if error_number in (errno.EEXIST, errno.ENOTEMPTY):
            raise FreezeError("output_exists")
        raise FreezeError("atomic_publish_failed", exit_code=error_number)


def _parse_arguments(arguments: list[str]) -> dict[str, str]:
    allowed = {"--worktree", "--output-dir", "--base-commit", "--go-binary", "--built-at"}
    if len(arguments) != 10:
        raise FreezeError("arguments_invalid")
    parsed: dict[str, str] = {}
    for index in range(0, len(arguments), 2):
        name = arguments[index]
        if name not in allowed or name in parsed or not arguments[index + 1]:
            raise FreezeError("arguments_invalid")
        parsed[name] = arguments[index + 1]
    if set(parsed) != allowed:
        raise FreezeError("arguments_invalid")
    return parsed


def _canonical_absolute_existing_directory(raw: str, error_code: str) -> Path:
    supplied = Path(raw)
    if not supplied.is_absolute():
        raise FreezeError(error_code)
    try:
        resolved = supplied.resolve(strict=True)
    except OSError as error:
        raise FreezeError(error_code) from error
    if not resolved.is_dir():
        raise FreezeError(error_code)
    return resolved


def _canonical_output(raw: str, worktree: Path) -> Path:
    supplied = Path(raw)
    if not supplied.is_absolute() or supplied.name in ("", ".", ".."):
        raise FreezeError("output_path_invalid")
    try:
        parent = supplied.parent.resolve(strict=True)
    except OSError as error:
        raise FreezeError("output_parent_invalid") from error
    resolved = parent / supplied.name
    if not parent.is_dir():
        raise FreezeError("output_parent_invalid")
    if resolved.exists() or resolved.is_symlink():
        raise FreezeError("output_exists")
    try:
        resolved.relative_to(worktree)
    except ValueError:
        pass
    else:
        raise FreezeError("output_inside_worktree")
    return resolved


def _validate_inputs(parsed: dict[str, str]) -> tuple[Path, Path, str, Path, str]:
    worktree = _canonical_absolute_existing_directory(
        parsed["--worktree"], "worktree_invalid"
    )
    top_level = _run_git(
        worktree,
        _clean_git_environment(),
        "worktree_root_resolution_failed",
        "rev-parse",
        "--show-toplevel",
    ).decode("utf-8").strip()
    if Path(top_level).resolve(strict=True) != worktree:
        raise FreezeError("worktree_not_repository_root")
    output = _canonical_output(parsed["--output-dir"], worktree)
    base_commit = parsed["--base-commit"]
    if _HEX40.fullmatch(base_commit) is None:
        raise FreezeError("base_commit_invalid")
    go_binary = Path(parsed["--go-binary"])
    if not go_binary.is_absolute():
        raise FreezeError("go_binary_invalid")
    try:
        resolved_go = go_binary.resolve(strict=True)
    except OSError as error:
        raise FreezeError("go_binary_invalid") from error
    if not resolved_go.is_file() or not os.access(resolved_go, os.X_OK):
        raise FreezeError("go_binary_invalid")
    built_at = parsed["--built-at"]
    if _RFC3339_UTC.fullmatch(built_at) is None:
        raise FreezeError("built_at_invalid")
    try:
        dt.datetime.strptime(built_at, "%Y-%m-%dT%H:%M:%SZ")
    except ValueError as error:
        raise FreezeError("built_at_invalid") from error
    return worktree, output, base_commit, resolved_go, built_at


def freeze(arguments: list[str]) -> dict[str, object]:
    parsed = _parse_arguments(arguments)
    worktree, output, base_commit, go_binary, built_at = _validate_inputs(parsed)
    generator_path = Path(__file__).resolve(strict=True)
    generator_sha256_before = _file_sha256(generator_path)
    user_home, module_cache, build_cache = _go_cache_paths(go_binary, worktree)
    toolchain_identity_before = _toolchain_identity(
        go_binary, user_home, worktree
    )
    captured = _capture_source(worktree, base_commit)

    output_parent = output.parent
    staging = Path(
        tempfile.mkdtemp(prefix=f".{output.name}.staging-", dir=output_parent)
    )
    build_temporary = Path(
        tempfile.mkdtemp(prefix="agentic-bench-luban-build-", dir=output_parent)
    )
    published = False
    try:
        binary_source, build_argv, toolchain = _build_frozen_archive(
            captured["archive"],
            go_binary,
            toolchain_identity_before,
            user_home,
            module_cache,
            build_cache,
            build_temporary,
        )
        binary_sha256 = _file_sha256(binary_source)
        build_receipt = {
            "schema_version": _BUILD_RECEIPT_SCHEMA,
            "agent_id": "luban",
            "base_commit": captured["base_commit"],
            "tree_oid": captured["tree_oid"],
            "patch_sha256": captured["patch_sha256"],
            "archive_sha256": captured["archive_sha256"],
            "path_policy": _PATH_POLICY,
            "path_policy_sha256": captured["path_policy_sha256"],
            "exclusion_receipt_sha256": captured[
                "exclusion_receipt_sha256"
            ],
            "binary_sha256": binary_sha256,
            "build_argv": build_argv,
            "toolchain": toolchain,
            "built_at": built_at,
        }
        build_receipt_raw = _canonical_json(build_receipt) + b"\n"
        build_receipt_sha256 = _sha256(build_receipt_raw)
        replacements = {
            "ABSOLUTE_LUBAN_BINARY": str(output / "luban"),
            "LUBAN_BINARY_SHA256": binary_sha256,
            "ABSOLUTE_LUBAN_WORKTREE": str(worktree),
            "LUBAN_SOURCE_BASE_COMMIT": captured["base_commit"],
            "LUBAN_SOURCE_TREE_OID": captured["tree_oid"],
            "LUBAN_SOURCE_PATCH_SHA256": captured["patch_sha256"],
            "LUBAN_SOURCE_ARCHIVE_SHA256": captured["archive_sha256"],
            "ABSOLUTE_LUBAN_BUILD_RECEIPT": str(
                output / "build-receipt.json"
            ),
            "LUBAN_BUILD_RECEIPT_SHA256": build_receipt_sha256,
        }
        manifest_values = {
            "schema_version": _SCHEMA,
            "manifest_replacements": replacements,
            "source_path_policy_sha256": captured["path_policy_sha256"],
            "source_exclusion_receipt_sha256": captured[
                "exclusion_receipt_sha256"
            ],
            "generator_sha256": generator_sha256_before,
            "build_argv": build_argv,
            "toolchain": toolchain,
            "built_at": built_at,
        }
        manifest_values_raw = _canonical_json(manifest_values) + b"\n"

        _copy_exclusive(binary_source, staging / "luban", 0o755)
        _write_exclusive(staging / "source.patch", captured["patch"], 0o644)
        _write_exclusive(staging / "source.tar", captured["archive"], 0o644)
        _write_exclusive(
            staging / "source-exclusions.json",
            captured["exclusion_receipt"],
            0o644,
        )
        _write_exclusive(
            staging / "build-receipt.json", build_receipt_raw, 0o644
        )
        _write_exclusive(
            staging / "manifest-values.json", manifest_values_raw, 0o644
        )

        recaptured = _capture_source(worktree, base_commit)
        for key in (
            "tree_oid",
            "patch_sha256",
            "archive_sha256",
            "path_policy_sha256",
            "exclusion_receipt_sha256",
        ):
            if recaptured[key] != captured[key]:
                raise FreezeError("source_drift_during_freeze")
        if _file_sha256(generator_path) != generator_sha256_before:
            raise FreezeError("generator_drift_during_freeze")
        _sync_directory(staging)
        _rename_noreplace(staging, output)
        published = True
        _sync_directory(output_parent)
        return {
            "schema_version": _RESULT_SCHEMA,
            "output_dir": str(output),
            "manifest_values": str(output / "manifest-values.json"),
            "manifest_values_sha256": _sha256(manifest_values_raw),
        }
    finally:
        shutil.rmtree(build_temporary, ignore_errors=True)
        if not published:
            shutil.rmtree(staging, ignore_errors=True)


def _fail(error: FreezeError) -> NoReturn:
    payload: dict[str, object] = {
        "schema_version": _ERROR_SCHEMA,
        "error_code": error.code,
    }
    if error.operation:
        payload["operation"] = error.operation
    if error.exit_code is not None:
        payload["exit_code"] = error.exit_code
    sys.stderr.buffer.write(_canonical_json(payload) + b"\n")
    raise SystemExit(1)


def main() -> None:
    try:
        result = freeze(sys.argv[1:])
    except FreezeError as error:
        _fail(error)
    except Exception:
        _fail(FreezeError("internal_failure"))
    sys.stdout.buffer.write(_canonical_json(result) + b"\n")


if __name__ == "__main__":
    main()
