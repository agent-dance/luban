from __future__ import annotations

import json
import signal
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from benchmark.agentic.pilot import host_storage_guard
from benchmark.agentic.pilot.host_storage_guard import (
    ADMISSION_MINIMUM_BYTES,
    EVIDENCE_MODE,
    EXIT_ADMISSION_REJECTED,
    EXIT_GUARD_FAILURE,
    GIB,
    MAX_COMPLETION_GAP_NS,
    POLL_INTERVAL_NS,
    RUNTIME_HARD_FLOOR_BYTES,
    SCHEMA_VERSION,
    WARNING_BELOW_BYTES,
    DarwinFDStorageSampler,
    PilotHostStorageGuard,
    StorageReading,
)


class _FakeClock:
    def __init__(self) -> None:
        self.monotonic = 0
        self.wall = 1_800_000_000_000_000_000

    def monotonic_ns(self) -> int:
        return self.monotonic

    def wall_time_ns(self) -> int:
        return self.wall + self.monotonic

    def advance(self, nanoseconds: int) -> None:
        self.monotonic += nanoseconds


class _FakeProcess:
    def __init__(
        self,
        clock: _FakeClock,
        *,
        exit_at_ns: int | None,
        exit_code: int = 0,
        oversleep_ns: int = 0,
        ignore_sigterm: bool = False,
    ) -> None:
        self.pid = 4242
        self.returncode: int | None = None
        self._clock = clock
        self._exit_at_ns = exit_at_ns
        self._exit_code = exit_code
        self._oversleep_ns = oversleep_ns
        self._oversleep_used = False
        self._ignore_sigterm = ignore_sigterm
        self.wait_timeouts: list[float | None] = []

    def _refresh(self) -> None:
        if (
            self.returncode is None
            and self._exit_at_ns is not None
            and self._clock.monotonic >= self._exit_at_ns
        ):
            self.returncode = self._exit_code

    def poll(self) -> int | None:
        self._refresh()
        return self.returncode

    def wait(self, timeout: float | None = None) -> int:
        self.wait_timeouts.append(timeout)
        self._refresh()
        if self.returncode is not None:
            return self.returncode
        if timeout is None:
            raise AssertionError("unbounded_wait_not_expected")
        timeout_ns = round(timeout * 1_000_000_000)
        if (
            self._exit_at_ns is not None
            and self._clock.monotonic + timeout_ns >= self._exit_at_ns
        ):
            self._clock.advance(self._exit_at_ns - self._clock.monotonic)
            self._refresh()
            assert self.returncode is not None
            return self.returncode
        if self._oversleep_ns and not self._oversleep_used:
            timeout_ns += self._oversleep_ns
            self._oversleep_used = True
        self._clock.advance(timeout_ns)
        raise subprocess.TimeoutExpired(["limactl", "shell"], timeout)

    def receive_signal(self, signal_number: int) -> None:
        if signal_number == signal.SIGTERM and self._ignore_sigterm:
            return
        self.returncode = -signal_number


class _FakeLauncher:
    def __init__(self, process: _FakeProcess) -> None:
        self.process = process
        self.calls: list[tuple[tuple[str, ...], bool]] = []

    def __call__(
        self, command: tuple[str, ...], *, start_new_session: bool
    ) -> _FakeProcess:
        self.calls.append((command, start_new_session))
        return self.process


class _FakeSampler:
    def __init__(
        self,
        root_roles: dict[Path, str],
        rounds: list[int | dict[str, object]],
        *,
        fail_round: int | None = None,
    ) -> None:
        self._root_roles = root_roles
        self._rounds = rounds
        self._fail_round = fail_round
        self._next_fd = 100
        self._fd_roles: dict[int, str] = {}
        self._indices: dict[int, int] = {}
        self.opened: list[Path] = []
        self.closed: list[int] = []
        self.sample_calls = 0

    def open_root(self, path: Path) -> int:
        role = self._root_roles[path]
        self._next_fd += 1
        fd = self._next_fd
        self._fd_roles[fd] = role
        self._indices[fd] = 0
        self.opened.append(path)
        return fd

    def sample(self, fd: int) -> StorageReading:
        index = self._indices[fd]
        self._indices[fd] = index + 1
        self.sample_calls += 1
        if self._fail_round is not None and index == self._fail_round:
            raise OSError("injected_sample_failure")
        raw_spec = self._rounds[min(index, len(self._rounds) - 1)]
        if isinstance(raw_spec, int):
            spec: dict[str, object] = {"available_bytes": raw_spec}
        else:
            spec = raw_spec
        role = self._fd_roles[fd]
        role_index = {
            "controller_root": 1,
            "private_root": 2,
            "artifact_root": 3,
        }[role]
        total_bytes = int(spec.get("total_bytes", 200 * GIB))
        available_bytes = int(spec.get("available_bytes", 120 * GIB))
        filesystem_id = spec.get("filesystem_id", (7, 9))
        assert isinstance(filesystem_id, tuple)
        return StorageReading(
            directory_device=int(spec.get("directory_device", 42)),
            directory_inode=10_000 + role_index + int(spec.get("inode_delta", 0)),
            directory_mode=int(spec.get("directory_mode", stat.S_IFDIR | 0o700)),
            filesystem_id=filesystem_id,
            filesystem_type=str(spec.get("filesystem_type", "apfs")),
            block_size=int(spec.get("block_size", 4096)),
            total_bytes=total_bytes,
            available_bytes=available_bytes,
            used_bytes=total_bytes - available_bytes,
        )

    def close(self, fd: int) -> None:
        self.closed.append(fd)


class PilotHostStorageGuardTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary_directory.name)
        self.controller = self.root / "controller"
        self.private = self.root / "private"
        self.artifact = self.root / "artifact"
        for path in (self.controller, self.private, self.artifact):
            path.mkdir()
        self.receipt_path = self.root / "host-storage-receipt.json"

    def tearDown(self) -> None:
        self.temporary_directory.cleanup()

    def _components(
        self,
        rounds: list[int | dict[str, object]],
        *,
        exit_at_ns: int | None = 1_500_000_000,
        exit_code: int = 0,
        oversleep_ns: int = 0,
        ignore_sigterm: bool = False,
        fail_round: int | None = None,
        signal_observer=None,
    ):
        clock = _FakeClock()
        process = _FakeProcess(
            clock,
            exit_at_ns=exit_at_ns,
            exit_code=exit_code,
            oversleep_ns=oversleep_ns,
            ignore_sigterm=ignore_sigterm,
        )
        launcher = _FakeLauncher(process)
        sampler = _FakeSampler(
            {
                self.controller: "controller_root",
                self.private: "private_root",
                self.artifact: "artifact_root",
            },
            rounds,
            fail_round=fail_round,
        )
        signals: list[tuple[int, int]] = []

        def signal_process_group(pid: int, signal_number: int) -> None:
            signals.append((pid, signal_number))
            if signal_observer is not None:
                signal_observer(pid, signal_number)
            process.receive_signal(signal_number)

        guard = PilotHostStorageGuard(
            controller_root=self.controller,
            private_root=self.private,
            artifact_root=self.artifact,
            receipt_path=self.receipt_path,
            sampler=sampler,
            process_launcher=launcher,
            signal_process_group=signal_process_group,
            monotonic_ns=clock.monotonic_ns,
            wall_time_ns=clock.wall_time_ns,
            platform_name="darwin",
        )
        return guard, process, launcher, sampler, signals, clock

    def _receipt(self) -> dict[str, object]:
        return json.loads(self.receipt_path.read_text(encoding="ascii"))

    def test_normal_exit_is_explicitly_nonformal_and_deduplicates_roles(self) -> None:
        guard, _, launcher, sampler, signals, _ = self._components(
            [120 * GIB, 120 * GIB, 120 * GIB], exit_code=7
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, 7)
        self.assertEqual(
            launcher.calls,
            [(('limactl', 'shell', 'pilot-vm', 'true'), True)],
        )
        self.assertEqual(signals, [])
        self.assertEqual(len(sampler.opened), 3)
        self.assertEqual(len(sampler.closed), 3)
        receipt = self._receipt()
        self.assertEqual(receipt["schema_version"], SCHEMA_VERSION)
        self.assertFalse(receipt["formal_compatible"])
        self.assertEqual(receipt["evidence_mode"], EVIDENCE_MODE)
        self.assertFalse(receipt["crash_proof"])
        self.assertIn("non-crash-proof", receipt["limitations"])
        self.assertEqual(receipt["status"], "completed")
        self.assertEqual(receipt["reason_code"], "child_exited")
        self.assertEqual(receipt["child_exit_code"], 7)
        self.assertEqual(receipt["admission_minimum_bytes"], 100 * GIB)
        self.assertEqual(receipt["warning_below_bytes"], 50 * GIB)
        self.assertEqual(receipt["runtime_hard_floor_bytes"], 30 * GIB)
        self.assertEqual(receipt["poll_interval_ms"], 1000)
        self.assertEqual(receipt["max_completion_gap_ms"], 2500)
        self.assertFalse(receipt["warning"])
        self.assertFalse(receipt["kill_attempted"])
        self.assertFalse(receipt["kill_acknowledged"])
        self.assertEqual(receipt["sample_count"], 3)
        for sample in receipt["samples"]:
            self.assertEqual(len(sample["filesystems"]), 1)
            self.assertEqual(
                sample["filesystems"][0]["roles"],
                ["artifact_root", "controller_root", "private_root"],
            )

    def test_admission_below_100_gib_never_launches(self) -> None:
        guard, _, launcher, sampler, signals, _ = self._components([99 * GIB])

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_ADMISSION_REJECTED)
        self.assertEqual(launcher.calls, [])
        self.assertEqual(signals, [])
        self.assertEqual(len(sampler.closed), 3)
        receipt = self._receipt()
        self.assertEqual(receipt["status"], "admission_rejected")
        self.assertEqual(
            receipt["reason_code"], "admission_storage_below_minimum"
        )
        self.assertFalse(receipt["admission_passed"])
        self.assertFalse(receipt["process_started"])
        self.assertFalse(receipt["kill_attempted"])

    def test_warning_below_50_gib_does_not_abort_above_30_gib(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, 40 * GIB, 40 * GIB]
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, 0)
        self.assertEqual(signals, [])
        receipt = self._receipt()
        self.assertEqual(receipt["status"], "completed")
        self.assertTrue(receipt["warning"])
        self.assertEqual(len(receipt["warning_events"]), 1)
        self.assertEqual(
            receipt["warning_events"][0]["code"],
            "storage_below_warning_threshold",
        )
        self.assertEqual(receipt["warning_events"][0]["sample_index"], 1)

    def test_exact_thresholds_are_admitted_and_hard_floor_does_not_abort(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [ADMISSION_MINIMUM_BYTES, WARNING_BELOW_BYTES, RUNTIME_HARD_FLOOR_BYTES],
            exit_at_ns=2_500_000_000,
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, 0)
        self.assertEqual(signals, [])
        receipt = self._receipt()
        self.assertEqual(receipt["status"], "completed")
        self.assertTrue(receipt["warning"])
        self.assertEqual(receipt["warning_events"][0]["sample_index"], 2)

    def test_runtime_floor_persists_terminal_receipt_before_sigterm(self) -> None:
        receipt_observed_at_signal: list[dict[str, object]] = []

        def observe_signal(_pid: int, _signal_number: int) -> None:
            receipt_observed_at_signal.append(self._receipt())

        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, 20 * GIB],
            exit_at_ns=None,
            signal_observer=observe_signal,
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_GUARD_FAILURE)
        self.assertEqual(signals, [(4242, signal.SIGTERM)])
        self.assertEqual(len(receipt_observed_at_signal), 1)
        before_signal = receipt_observed_at_signal[0]
        self.assertEqual(before_signal["status"], "guard_failed")
        self.assertEqual(
            before_signal["reason_code"], "runtime_storage_floor_breached"
        )
        self.assertTrue(before_signal["kill_required"])
        self.assertFalse(before_signal["kill_attempted"])
        self.assertFalse(before_signal["kill_acknowledged"])
        self.assertFalse(before_signal["sigterm_sent"])
        receipt = self._receipt()
        self.assertTrue(receipt["kill_attempted"])
        self.assertTrue(receipt["kill_acknowledged"])
        self.assertTrue(receipt["sigterm_sent"])
        self.assertFalse(receipt["sigkill_sent"])
        self.assertEqual(receipt["child_exit_code"], -signal.SIGTERM)

    def test_sigkill_is_used_only_after_sigterm_timeout(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, 20 * GIB],
            exit_at_ns=None,
            ignore_sigterm=True,
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_GUARD_FAILURE)
        self.assertEqual(
            signals, [(4242, signal.SIGTERM), (4242, signal.SIGKILL)]
        )
        receipt = self._receipt()
        self.assertTrue(receipt["sigterm_sent"])
        self.assertTrue(receipt["sigkill_sent"])
        self.assertTrue(receipt["kill_acknowledged"])
        self.assertEqual(receipt["child_exit_code"], -signal.SIGKILL)

    def test_completion_gap_strictly_above_2500_ms_aborts(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, 120 * GIB],
            exit_at_ns=None,
            oversleep_ns=MAX_COMPLETION_GAP_NS - POLL_INTERVAL_NS + 1,
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_GUARD_FAILURE)
        self.assertEqual(signals, [(4242, signal.SIGTERM)])
        receipt = self._receipt()
        self.assertEqual(
            receipt["reason_code"], "sample_completion_gap_exceeded"
        )
        self.assertEqual(
            receipt["samples"][1]["completion_gap_ns"],
            MAX_COMPLETION_GAP_NS + 1,
        )

    def test_filesystem_identity_drift_fails_closed(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, {"available_bytes": 120 * GIB, "filesystem_id": (8, 9)}],
            exit_at_ns=None,
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_GUARD_FAILURE)
        self.assertEqual(signals, [(4242, signal.SIGTERM)])
        receipt = self._receipt()
        self.assertEqual(receipt["reason_code"], "filesystem_identity_changed")
        self.assertEqual(receipt["sample_count"], 1)

    def test_sampling_syscall_failure_fails_closed(self) -> None:
        guard, _, _, _, signals, _ = self._components(
            [120 * GIB, 120 * GIB], exit_at_ns=None, fail_round=1
        )

        exit_code = guard.run(["limactl", "shell", "pilot-vm", "true"])

        self.assertEqual(exit_code, EXIT_GUARD_FAILURE)
        self.assertEqual(signals, [(4242, signal.SIGTERM)])
        receipt = self._receipt()
        self.assertEqual(
            receipt["reason_code"], "storage_sample_syscall_failed"
        )

    def test_command_gate_rejects_wrong_prefix_and_any_limactl_stop(self) -> None:
        commands = (
            ["python3", "run.py"],
            ["limactl", "shell", "pilot-vm", "limactl", "stop", "pilot-vm"],
            [
                "limactl",
                "shell",
                "pilot-vm",
                "bash",
                "-lc",
                "run; limactl stop pilot-vm",
            ],
        )
        for index, command in enumerate(commands):
            with self.subTest(command=command):
                self.receipt_path = self.root / f"command-rejection-{index}.json"
                guard, _, launcher, sampler, signals, _ = self._components(
                    [120 * GIB]
                )
                exit_code = guard.run(command)
                self.assertEqual(exit_code, EXIT_ADMISSION_REJECTED)
                self.assertEqual(launcher.calls, [])
                self.assertEqual(sampler.opened, [])
                self.assertEqual(signals, [])
                receipt = self._receipt()
                expected = (
                    "invalid_command_prefix"
                    if index == 0
                    else "forbidden_limactl_stop"
                )
                self.assertEqual(receipt["reason_code"], expected)

    def test_non_darwin_non_apfs_and_missing_roots_are_rejected(self) -> None:
        guard, _, launcher, sampler, _, _ = self._components([120 * GIB])
        guard._platform_name = "linux"
        self.assertEqual(
            guard.run(["limactl", "shell", "pilot-vm"]),
            EXIT_ADMISSION_REJECTED,
        )
        self.assertEqual(self._receipt()["reason_code"], "unsupported_platform")
        self.assertEqual(launcher.calls, [])
        self.assertEqual(sampler.opened, [])

        self.receipt_path = self.root / "non-apfs.json"
        guard, _, launcher, _, _, _ = self._components(
            [{"available_bytes": 120 * GIB, "filesystem_type": "ext4"}]
        )
        self.assertEqual(
            guard.run(["limactl", "shell", "pilot-vm"]),
            EXIT_ADMISSION_REJECTED,
        )
        self.assertEqual(self._receipt()["reason_code"], "filesystem_not_apfs")
        self.assertEqual(launcher.calls, [])

        self.receipt_path = self.root / "missing-root.json"
        self.private.rmdir()
        guard, _, launcher, _, _, _ = self._components([120 * GIB])
        self.assertEqual(
            guard.run(["limactl", "shell", "pilot-vm"]),
            EXIT_ADMISSION_REJECTED,
        )
        self.assertEqual(
            self._receipt()["reason_code"], "root_not_existing_directory"
        )
        self.assertEqual(launcher.calls, [])

    def test_fd_sampler_uses_fresh_fstat_and_fstatvfs_each_sample(self) -> None:
        directory = SimpleNamespace(
            st_dev=42,
            st_ino=99,
            st_mode=stat.S_IFDIR | 0o700,
        )
        counters = SimpleNamespace(
            f_frsize=4096,
            f_bsize=4096,
            f_blocks=100,
            f_bfree=40,
            f_bavail=30,
        )
        filesystem_identity = mock.Mock(return_value=((3, 4), "apfs"))
        sampler = DarwinFDStorageSampler(
            filesystem_identity=filesystem_identity
        )
        with (
            mock.patch.object(host_storage_guard.os, "fstat", return_value=directory) as fstat_mock,
            mock.patch.object(host_storage_guard.os, "fstatvfs", return_value=counters) as fstatvfs_mock,
        ):
            first = sampler.sample(12)
            second = sampler.sample(12)

        self.assertEqual(first, second)
        self.assertEqual(first.total_bytes, 409_600)
        self.assertEqual(first.available_bytes, 122_880)
        self.assertEqual(first.used_bytes, 245_760)
        self.assertEqual(fstat_mock.call_count, 2)
        self.assertEqual(fstatvfs_mock.call_count, 2)
        self.assertEqual(filesystem_identity.call_count, 2)

    def test_cli_requires_explicit_separator(self) -> None:
        base = [
            "--controller-root",
            str(self.controller),
            "--private-root",
            str(self.private),
            "--artifact-root",
            str(self.artifact),
            "--receipt",
            str(self.receipt_path),
        ]
        arguments, command = host_storage_guard._parse_arguments(
            [*base, "--", "limactl", "shell", "pilot-vm"]
        )
        self.assertEqual(arguments.controller_root, self.controller)
        self.assertEqual(command, ["limactl", "shell", "pilot-vm"])
        with self.assertRaises(host_storage_guard.CLIUsageError):
            host_storage_guard._parse_arguments(
                [*base, "limactl", "shell", "pilot-vm"]
            )


if __name__ == "__main__":
    unittest.main()
