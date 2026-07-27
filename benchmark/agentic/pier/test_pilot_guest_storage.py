from __future__ import annotations

import hashlib
import os
import sys
import tempfile
import unittest
from pathlib import Path

from benchmark.agentic.pier import pilot_guest_storage as storage


class PilotGuestStorageConstantsTest(unittest.TestCase):
    def test_guard_constants_match_frozen_pilot_policy(self) -> None:
        self.assertEqual(storage.START_MINIMUM_BYTES, 28 * (1 << 30))
        self.assertEqual(storage.RUNTIME_FLOOR_BYTES, 8 * (1 << 30))
        self.assertEqual(storage.POLL_INTERVAL_MS, 1_000)
        self.assertEqual(storage.GAP_THRESHOLD_MS, 2_500)
        self.assertEqual(
            storage.PILOT_GUEST_PREFLIGHT_DIRECTORY,
            Path("/home/blurooo.guest/agentic-bench/private"),
        )
        self.assertEqual(
            storage.PILOT_GUEST_PREFLIGHT_RECEIPT,
            "pilot-guest-storage-preflight.json",
        )

    def test_preflight_cli_accepts_only_the_helper_digest(self) -> None:
        digest = "a" * 64
        self.assertEqual(
            storage._parse_preflight_arguments(
                [storage._PREFLIGHT_ENTRY, "--helper-sha256", digest]
            ),
            digest,
        )
        for arguments in (
            [storage._PREFLIGHT_ENTRY],
            [storage._PREFLIGHT_ENTRY, "--helper-sha256", "short"],
            [storage._PREFLIGHT_ENTRY, "--receipt-directory", "/tmp"],
            [
                storage._PREFLIGHT_ENTRY,
                "--helper-sha256",
                digest,
                "--container-id",
                "b" * 64,
            ],
        ):
            with self.subTest(arguments=arguments):
                with self.assertRaises(storage.PilotGuestStorageFailure):
                    storage._parse_preflight_arguments(arguments)


class ExactComposeContainerIDTest(unittest.IsolatedAsyncioTestCase):
    async def test_expands_compose_id_and_validates_exact_running_container(self) -> None:
        short_id = "0123456789ab"
        exact_id = short_id + ("c" * (64 - len(short_id)))
        calls: list[tuple[str, list[str]]] = []

        async def compose_command(argv: list[str]) -> bytes:
            calls.append(("compose", argv))
            return (short_id + "\n").encode("ascii")

        async def docker_command(argv: list[str]) -> bytes:
            calls.append(("docker", argv))
            return (
                '{"id":"'
                + exact_id
                + '","running":true,"restart_count":0}'
            ).encode("ascii")

        result = await storage._exact_compose_container_id(
            docker_command, compose_command
        )

        self.assertEqual(result, exact_id)
        self.assertEqual(
            calls,
            [
                ("compose", ["ps", "-q", "main"]),
                (
                    "docker",
                    [
                        "docker",
                        "inspect",
                        "--format",
                        '{"id":{{json .Id}},"running":{{json .State.Running}},"restart_count":{{json .RestartCount}}}',
                        short_id,
                    ],
                ),
            ],
        )

    async def test_rejects_malformed_compose_ids_before_inspect(self) -> None:
        malformed = (b"", b"abc\n", b"A" * 64, b"a" * 65, b"a" * 12 + b" x")

        for candidate in malformed:
            with self.subTest(candidate=candidate):
                docker_called = False

                async def compose_command(_: list[str]) -> bytes:
                    return candidate

                async def docker_command(_: list[str]) -> bytes:
                    nonlocal docker_called
                    docker_called = True
                    return b"{}"

                with self.assertRaises(storage.PilotGuestStorageFailure) as caught:
                    await storage._exact_compose_container_id(
                        docker_command, compose_command
                    )
                self.assertEqual(caught.exception.code, "container_identity_invalid")
                self.assertFalse(docker_called)

    async def test_rejects_malformed_or_unsafe_inspect_identity(self) -> None:
        exact_id = "d" * 64
        malformed = (
            b'{"id":"short","running":true,"restart_count":0}',
            (
                '{"id":"'
                + exact_id
                + '","running":false,"restart_count":0}'
            ).encode("ascii"),
            (
                '{"id":"'
                + exact_id
                + '","running":true,"restart_count":1}'
            ).encode("ascii"),
            (
                '{"id":"'
                + exact_id
                + '","running":true,"restart_count":true}'
            ).encode("ascii"),
            (
                '{"id":"'
                + exact_id
                + '","id":"'
                + exact_id
                + '","running":true,"restart_count":0}'
            ).encode("ascii"),
        )

        async def compose_command(_: list[str]) -> bytes:
            return (exact_id[:12] + "\n").encode("ascii")

        for response in malformed:
            with self.subTest(response=response):

                async def docker_command(_: list[str]) -> bytes:
                    return response

                with self.assertRaises(storage.PilotGuestStorageFailure) as caught:
                    await storage._exact_compose_container_id(
                        docker_command, compose_command
                    )
                self.assertIn(
                    caught.exception.code,
                    {"authority_invalid", "container_identity_invalid"},
                )


class HelperArgumentsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.arguments = [
            storage._HELPER_ENTRY,
            "--phase",
            "agent",
            "--session-sha256",
            "1" * 64,
            "--container-id",
            "2" * 64,
            "--receipt-directory",
            "/tmp/pilot-receipts",
            "--receipt-device",
            "3",
            "--receipt-inode",
            "4",
            "--receipt-owner-uid",
            "5",
            "--receipt-owner-gid",
            "6",
            "--helper-sha256",
            "7" * 64,
        ]

    def assert_invalid(self, arguments: list[str]) -> None:
        with self.assertRaises(storage.PilotGuestStorageFailure) as caught:
            storage._parse_helper_arguments(arguments)
        self.assertEqual(caught.exception.code, "helper_arguments_invalid")

    def test_parses_exact_argument_set_and_integer_fields(self) -> None:
        parsed = storage._parse_helper_arguments(self.arguments)

        self.assertEqual(parsed["--phase"], "agent")
        self.assertEqual(parsed["--container-id"], "2" * 64)
        self.assertEqual(parsed["--receipt-device"], 3)
        self.assertEqual(parsed["--receipt-inode"], 4)
        self.assertEqual(parsed["--receipt-owner-uid"], 5)
        self.assertEqual(parsed["--receipt-owner-gid"], 6)
        self.assertEqual(len(parsed), 9)

    def test_rejects_duplicate_unknown_missing_and_dangling_arguments(self) -> None:
        cases = (
            self.arguments + ["--phase", "verifier"],
            self.arguments + ["--unknown", "value"],
            self.arguments[:-2],
            self.arguments + ["--phase"],
            ["wrong-entry", *self.arguments[1:]],
            [],
        )
        for arguments in cases:
            with self.subTest(arguments=arguments):
                self.assert_invalid(arguments)

    def test_rejects_malformed_values(self) -> None:
        def replaced(key: str, value: str) -> list[str]:
            result = list(self.arguments)
            result[result.index(key) + 1] = value
            return result

        cases = (
            replaced("--phase", "Agent"),
            replaced("--session-sha256", "f" * 63),
            replaced("--session-sha256", "G" * 64),
            replaced("--container-id", "a" * 63),
            replaced("--container-id", "A" * 64),
            replaced("--helper-sha256", "not-a-digest"),
            replaced("--receipt-device", "not-an-integer"),
            replaced("--receipt-inode", "-1"),
            replaced("--receipt-owner-uid", "-1"),
            replaced("--receipt-owner-gid", "-1"),
        )
        for arguments in cases:
            with self.subTest(arguments=arguments):
                self.assert_invalid(arguments)


@unittest.skipUnless(hasattr(os, "O_NOFOLLOW"), "requires O_NOFOLLOW")
class ReceiptPathSafetyTest(unittest.TestCase):
    def test_rejects_symlinked_receipt_directory(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            real_directory = root / "real"
            real_directory.mkdir()
            symlink = root / "alias"
            symlink.symlink_to(real_directory, target_is_directory=True)

            with self.assertRaises(storage.PilotGuestStorageFailure) as caught:
                storage._open_verified_receipt_directory(symlink)

            self.assertEqual(caught.exception.code, "receipt_directory_invalid")

    def test_rejects_openat_receipt_target_symlink(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary).resolve(strict=True)
            target = directory / "outside.json"
            target.write_bytes(b"unchanged")
            receipt = directory / storage.PILOT_GUEST_STORAGE_RECEIPT
            receipt.symlink_to(target)
            directory_fd, metadata = storage._open_verified_receipt_directory(
                directory
            )
            try:
                with self.assertRaises(storage.PilotGuestStorageFailure) as caught:
                    storage._write_bytes_at(
                        directory_fd,
                        storage.PILOT_GUEST_STORAGE_RECEIPT,
                        b"replacement",
                        owner_uid=metadata.st_uid,
                        owner_gid=metadata.st_gid,
                    )
            finally:
                os.close(directory_fd)

            self.assertEqual(caught.exception.code, "receipt_target_invalid")
            self.assertEqual(target.read_bytes(), b"unchanged")
            self.assertTrue(receipt.is_symlink())


@unittest.skipUnless(sys.platform.startswith("linux"), "Linux statfs layout only")
class SampleFDTest(unittest.TestCase):
    def test_sample_fd_reports_stable_identity_and_bounded_space(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fd = os.open(
                temporary,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            )
            try:
                first = storage._sample_fd(fd)
                second = storage._sample_fd(fd)
            finally:
                os.close(fd)

        self.assertEqual(first.private_identity, second.private_identity)
        self.assertEqual(first.public_identity, second.public_identity)
        self.assertEqual(first.filesystem_type, second.filesystem_type)
        self.assertEqual(first.block_size, second.block_size)
        self.assertEqual(first.total, second.total)
        self.assertEqual(
            first.public_identity,
            hashlib.sha256(first.private_identity.encode("utf-8")).hexdigest(),
        )
        self.assertRegex(first.public_identity, r"^[0-9a-f]{64}$")
        self.assertRegex(first.filesystem_type, r"^linux-0x[0-9a-f]{16}$")
        self.assertIn("device=", first.private_identity)
        self.assertIn("filesystem_type=", first.private_identity)
        self.assertIn("fsid_0=", first.private_identity)
        self.assertIn("fsid_1=", first.private_identity)
        self.assertGreater(first.block_size, 0)
        self.assertGreater(first.total, 0)
        self.assertGreaterEqual(first.available, 0)
        self.assertLessEqual(first.available, first.total)
        self.assertGreaterEqual(first.used, 0)
        self.assertLessEqual(first.used, first.total)


if __name__ == "__main__":
    unittest.main()
