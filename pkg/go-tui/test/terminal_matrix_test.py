import importlib.util
import pathlib
import shutil
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("terminal_matrix.py")
SPEC = importlib.util.spec_from_file_location("terminal_matrix", MODULE_PATH)
terminal_matrix = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(terminal_matrix)


class EscapeEvidenceTests(unittest.TestCase):
    def test_parses_only_members_of_the_fixture_terminal_session(self):
        output = "101 700\n102 701\n103 700\n"

        self.assertEqual(terminal_matrix.parse_session_process_ids(output, 700), [101, 103])

    def test_fixture_build_does_not_scan_parent_vcs_metadata(self):
        command = terminal_matrix.fixture_build_command(pathlib.Path("/tmp/fixture"))

        self.assertIn("-buildvcs=false", command)

    def test_resolves_bash_as_an_isolated_interactive_job_control_shell(self):
        bash = shutil.which("bash")
        if bash is None:
            self.skipTest("bash is not installed")

        command = terminal_matrix.resolve_job_control_shell(bash)

        self.assertEqual(command, [bash, "--noprofile", "--norc", "-i"])

    def test_rejects_an_unavailable_job_control_shell_before_spawning(self):
        with self.assertRaisesRegex(RuntimeError, "job-control shell is not executable"):
            terminal_matrix.resolve_job_control_shell("/does/not/exist/go-tui-shell")

    def test_ide_detection_does_not_mislabel_regular_terminal_programs(self):
        self.assertFalse(terminal_matrix.detect_ide_terminal({"TERM_PROGRAM": "Apple_Terminal"}))
        self.assertTrue(terminal_matrix.detect_ide_terminal({"TERM_PROGRAM": "vscode"}))
        self.assertTrue(terminal_matrix.detect_ide_terminal({"VSCODE_INJECTION": "1"}))

    def test_cli_can_select_both_real_pty_backends(self):
        arguments = terminal_matrix.parse_args(["--backend", "both", "--shell", "/bin/bash"])

        self.assertEqual(arguments.backend, "both")
        self.assertEqual(arguments.shell, "/bin/bash")

    def test_reports_complete_terminal_ownership_cycle(self):
        output = b"".join(
            (
                b"\x1b[?1049h",
                b"\x1b[?1000h\x1b[?1002h\x1b[?1006h",
                b"\x1b[?25l",
                terminal_matrix.READY_MARKER,
                b"\x1b[?1006l\x1b[?1002l\x1b[?1000l",
                b"\x1b[?25h",
                b"\x1b[?1049l",
            )
        )

        evidence = terminal_matrix.escape_evidence(output)

        self.assertTrue(evidence["render"])
        self.assertTrue(evidence["alternate_screen_enter"])
        self.assertTrue(evidence["alternate_screen_exit"])
        self.assertTrue(evidence["cursor_hide"])
        self.assertTrue(evidence["cursor_show"])
        self.assertTrue(evidence["mouse_enable"])
        self.assertTrue(evidence["mouse_disable"])
        self.assertTrue(terminal_matrix.escape_cycle_complete(evidence))

    def test_rejects_partial_mouse_cleanup(self):
        output = b"".join(
            (
                b"\x1b[?1049h\x1b[?1049l",
                b"\x1b[?1000h\x1b[?1002h\x1b[?1006h",
                b"\x1b[?1006l\x1b[?1000l",
                b"\x1b[?25l\x1b[?25h",
                terminal_matrix.READY_MARKER,
            )
        )

        evidence = terminal_matrix.escape_evidence(output)

        self.assertFalse(evidence["mouse_disable"])
        self.assertFalse(terminal_matrix.escape_cycle_complete(evidence))


@unittest.skipUnless(shutil.which("tmux"), "tmux is not installed")
class TmuxBackendTests(unittest.TestCase):
    def test_normal_lifecycle_uses_real_tmux_pane_tty(self):
        module_root = pathlib.Path(__file__).resolve().parents[1]
        with tempfile.TemporaryDirectory(prefix="go-tui-tmux-test-") as temp_dir:
            binary = pathlib.Path(temp_dir) / "terminal-fixture"
            terminal_matrix.build_fixture(module_root, binary)

            result = terminal_matrix.run_tmux_normal_scenario(str(binary), 5.0)

        self.assertTrue(result["ok"], result)
        self.assertEqual(result["evidence"]["backend"], "tmux-pane")


if __name__ == "__main__":
    unittest.main()
