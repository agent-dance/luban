#!/usr/bin/env python3
"""Run go-tui's terminal lifecycle fixture inside a real pseudoterminal.

The runner has no third-party dependencies. Its stdout is one JSON document so
the same command can be used as evidence on macOS, Linux, tmux, and IDE-hosted
terminals. Diagnostic child output is encoded in the per-scenario evidence and
is never treated as proof on its own: terminal flags and emitted control
sequences must both show that ownership was acquired and released.
"""

import argparse
import base64
import errno
import fcntl
import json
import os
import pathlib
import platform
import pty
import re
import select
import shlex
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time


READY_MARKER = b"GO_TUI_TERMINAL_FIXTURE_READY"
CONTROLLED_ERROR_MARKER = b'"event":"controlled_error"'
CONTROLLED_ERROR_EXIT_CODE = 23
FIXTURE_EXIT_PREFIX = b"__GO_TUI_FIXTURE_EXIT__"
SHELL_PROMPT = b"__GO_TUI_SHELL_PROMPT__"
SCHEMA = "go-tui-terminal-matrix/v1"


def escape_evidence(output):
    return {
        "render": READY_MARKER in output,
        "alternate_screen_enter": b"\x1b[?1049h" in output,
        "alternate_screen_exit": b"\x1b[?1049l" in output,
        "cursor_hide": b"\x1b[?25l" in output,
        "cursor_show": b"\x1b[?25h" in output,
        "mouse_enable": all(
            sequence in output
            for sequence in (b"\x1b[?1000h", b"\x1b[?1002h", b"\x1b[?1006h")
        ),
        "mouse_disable": all(
            sequence in output
            for sequence in (b"\x1b[?1006l", b"\x1b[?1002l", b"\x1b[?1000l")
        ),
    }


def escape_cycle_complete(evidence):
    return all(evidence.values())


def termios_equal(left, right):
    return left == right


def termios_is_raw(attributes):
    local_flags = attributes[3]
    return not local_flags & (termios.ECHO | termios.ICANON | termios.ISIG)


def detect_ide_terminal(environment):
    terminal_program = environment.get("TERM_PROGRAM", "").lower()
    return terminal_program in {"vscode", "cursor"} or any(
        environment.get(name)
        for name in ("VSCODE_INJECTION", "VSCODE_PID", "CURSOR_TRACE_ID")
    )


def resolve_job_control_shell(requested=None):
    candidate = requested or shutil.which("bash")
    if candidate and not os.path.isabs(candidate):
        candidate = shutil.which(candidate)
    if not candidate or not os.path.isfile(candidate) or not os.access(candidate, os.X_OK):
        raise RuntimeError("job-control shell is not executable: %s" % (requested or "bash"))
    if pathlib.Path(candidate).name == "bash":
        return [candidate, "--noprofile", "--norc", "-i"]
    return [candidate, "-i"]


def parse_session_process_ids(output, session_id):
    process_ids = []
    for line in output.splitlines():
        fields = line.split()
        if len(fields) != 2:
            continue
        try:
            process_id, process_session = (int(field) for field in fields)
        except ValueError:
            continue
        if process_session == session_id:
            process_ids.append(process_id)
    return process_ids


def session_process_ids(session_id):
    completed = subprocess.run(
        ["ps", "-eo", "pid=,sess="],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        return []
    return parse_session_process_ids(completed.stdout, session_id)


def set_window_size(fd, columns, rows):
    packed = struct.pack("HHHH", rows, columns, 0, 0)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, packed)


def window_size(fd):
    packed = fcntl.ioctl(fd, termios.TIOCGWINSZ, b"\0" * 8)
    rows, columns, _, _ = struct.unpack("HHHH", packed)
    return {"columns": columns, "rows": rows}


class PtyProcess:
    def __init__(self, binary, shell_command=None, columns=80, rows=24):
        shell_command = shell_command or resolve_job_control_shell()
        master_fd, slave_fd = pty.openpty()
        set_window_size(slave_fd, columns, rows)
        self.baseline_termios = termios.tcgetattr(slave_fd)
        self.master_fd = master_fd
        self.inspect_fd = slave_fd
        self.output = bytearray()
        self.reaped = False
        self.exit_code = None
        self.pid = os.fork()
        if self.pid == 0:
            self._exec_shell(shell_command, master_fd, slave_fd)

        os.set_blocking(self.master_fd, False)
        self.wait_for_output(SHELL_PROMPT, 0, 5.0)
        self.baseline_termios = self.current_termios()
        self.write((shlex.quote(binary) + "\n").encode())

    @staticmethod
    def _exec_shell(shell_command, master_fd, slave_fd):
        try:
            os.close(master_fd)
            os.setsid()
            fcntl.ioctl(slave_fd, termios.TIOCSCTTY, 0)
            os.tcsetpgrp(slave_fd, os.getpid())
            for target_fd in (0, 1, 2):
                os.dup2(slave_fd, target_fd)
            if slave_fd > 2:
                os.close(slave_fd)
            environment = os.environ.copy()
            environment["TERM"] = "xterm-256color"
            environment["LC_ALL"] = "C"
            environment["PS1"] = SHELL_PROMPT.decode()
            environment["ENV"] = "/dev/null"
            os.execve(shell_command[0], shell_command, environment)
        except BaseException as error:
            message = ("terminal fixture exec failed: %s\n" % error).encode()
            try:
                os.write(2, message)
            finally:
                os._exit(127)

    def current_termios(self):
        return termios.tcgetattr(self.inspect_fd)

    def write(self, data):
        os.write(self.master_fd, data)

    def resize(self, columns, rows):
        set_window_size(self.master_fd, columns, rows)

    def drain(self):
        while True:
            try:
                chunk = os.read(self.master_fd, 65536)
            except BlockingIOError:
                return
            except OSError as error:
                if error.errno == errno.EIO:
                    return
                raise
            if not chunk:
                return
            self.output.extend(chunk)

    def wait_for_output(self, marker, start, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            if marker in self.output[start:]:
                return bytes(self.output[start:])
            readable, _, _ = select.select([self.master_fd], [], [], 0.02)
            if readable:
                self.drain()
            self._poll_exit()
            if self.reaped:
                break
        self.drain()
        raise TimeoutError(
            "timed out waiting for %r; output_tail=%r"
            % (marker, bytes(self.output[-240:]))
        )

    def wait_for_terminal_release(self, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            if termios_equal(self.baseline_termios, self.current_termios()):
                self.drain()
                return
            self._poll_exit()
            if self.reaped:
                raise RuntimeError("shell exited before terminal release: %s" % self.exit_code)
            time.sleep(0.01)
        raise TimeoutError(
            "timed out waiting for terminal release; current_termios=%r"
            % (self.current_termios(),)
        )

    def continue_process(self):
        self.write(b"fg\n")

    def terminate(self):
        self.write(b"t")

    def wait_fixture_exit(self, timeout):
        pattern = re.compile(re.escape(FIXTURE_EXIT_PREFIX) + rb"([0-9]+)")
        start = len(self.output)
        command = "printf '\\n%s%%s\\n' \"$?\"\n" % FIXTURE_EXIT_PREFIX.decode()
        self.write(command.encode())
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            match = pattern.search(self.output[start:])
            if match:
                return int(match.group(1))
            self._poll_exit()
            if self.reaped:
                raise RuntimeError("shell exited before reporting fixture status: %s" % self.exit_code)
            time.sleep(0.01)
        raise TimeoutError("timed out waiting for fixture status marker")

    def finish_shell(self, expected_code, timeout):
        self.write(("exit %d\n" % expected_code).encode())
        exit_code = self.wait_exit(timeout)
        if exit_code != expected_code:
            raise RuntimeError(
                "shell exit code=%s, want fixture exit code=%s"
                % (exit_code, expected_code)
            )
        return exit_code

    def wait_exit(self, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            self._poll_exit()
            if self.reaped:
                self.drain()
                return self.exit_code
            time.sleep(0.01)
        raise TimeoutError("timed out waiting for fixture exit")

    def _poll_exit(self):
        if self.reaped:
            return
        waited_pid, status = os.waitpid(self.pid, os.WNOHANG)
        if waited_pid == self.pid:
            self._record_exit(status)

    def _record_exit(self, status):
        self.reaped = True
        if os.WIFEXITED(status):
            self.exit_code = os.WEXITSTATUS(status)
        elif os.WIFSIGNALED(status):
            self.exit_code = 128 + os.WTERMSIG(status)
        else:
            self.exit_code = 255

    def close(self):
        if not self.reaped:
            members = session_process_ids(self.pid)
            for process_id in members:
                if process_id == self.pid:
                    continue
                try:
                    os.kill(process_id, signal.SIGCONT)
                    os.kill(process_id, signal.SIGTERM)
                except ProcessLookupError:
                    pass
            try:
                os.kill(self.pid, signal.SIGHUP)
            except ProcessLookupError:
                pass
            time.sleep(0.05)
            for process_id in session_process_ids(self.pid):
                try:
                    os.kill(process_id, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            try:
                _, status = os.waitpid(self.pid, 0)
                self._record_exit(status)
            except ChildProcessError:
                self.reaped = True
        os.close(self.master_fd)
        os.close(self.inspect_fd)


class TmuxPane:
    def __init__(self, binary, shell_command=None):
        shell_command = shell_command or resolve_job_control_shell()
        self.temp_dir = tempfile.TemporaryDirectory(prefix="go-tui-tmux-pane-")
        self.session = "go-tui-matrix-%d-%d" % (os.getpid(), time.time_ns())
        self.target = self.session + ":0.0"
        self.raw_path = pathlib.Path(self.temp_dir.name) / "pane.raw"
        self.output = bytearray()
        self.raw_offset = 0
        self.inspect_fd = None
        self.closed = False

        shell_script = pathlib.Path(self.temp_dir.name) / "shell.sh"
        shell_script.write_text(
            "#!/bin/sh\n"
            "export LC_ALL=C\n"
            "export TERM=tmux-256color\n"
            "export PS1='%s'\n"
            "exec %s\n"
            % (
                SHELL_PROMPT.decode(),
                " ".join(shlex.quote(argument) for argument in shell_command),
            ),
            encoding="utf-8",
        )
        shell_script.chmod(0o700)

        self._tmux(
            "new-session",
            "-d",
            "-s",
            self.session,
            "-x",
            "80",
            "-y",
            "24",
            str(shell_script),
        )
        pane_tty = self._tmux(
            "display-message", "-p", "-t", self.target, "#{pane_tty}"
        ).stdout.strip()
        self.inspect_fd = os.open(pane_tty, os.O_RDWR | os.O_NOCTTY)
        self._tmux(
            "pipe-pane",
            "-t",
            self.target,
            "dd of=%s bs=1" % shlex.quote(str(self.raw_path)),
        )
        ready_frame = b"\r\n__GO_TUI_TMUX_SHELL_READY__\r\n"
        self.send_line("printf '\\n__GO_TUI_TMUX_SHELL_READY__\\n'")
        self.wait_for_output(ready_frame, 0, 5.0)
        self.baseline_termios = self.current_termios()
        self.send_line(shlex.quote(binary))

    def _tmux(self, *arguments, check=True):
        completed = subprocess.run(
            ["tmux", *arguments],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if check and completed.returncode != 0:
            raise RuntimeError(
                "tmux %s failed: %s"
                % (" ".join(arguments), completed.stderr.strip())
            )
        return completed

    def current_termios(self):
        return termios.tcgetattr(self.inspect_fd)

    def send_literal(self, text):
        self._tmux("send-keys", "-t", self.target, "-l", text)

    def send_line(self, text):
        self.send_literal(text)
        self._tmux("send-keys", "-t", self.target, "Enter")

    def send_key(self, key):
        self._tmux("send-keys", "-t", self.target, key)

    def resize(self, columns, rows):
        self._tmux(
            "resize-window",
            "-t",
            self.session + ":0",
            "-x",
            str(columns),
            "-y",
            str(rows),
        )

    def drain(self):
        try:
            with self.raw_path.open("rb") as raw_file:
                raw_file.seek(self.raw_offset)
                chunk = raw_file.read()
        except FileNotFoundError:
            return
        if chunk:
            self.output.extend(chunk)
            self.raw_offset += len(chunk)

    def wait_for_output(self, marker, start, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            if marker in self.output[start:]:
                return bytes(self.output[start:])
            if not self.session_alive():
                break
            time.sleep(0.02)
        self.drain()
        raise TimeoutError(
            "timed out waiting for %r in tmux pane; output_tail=%r"
            % (marker, bytes(self.output[-240:]))
        )

    def wait_for_terminal_release(self, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            if termios_equal(self.baseline_termios, self.current_termios()):
                return
            time.sleep(0.01)
        raise TimeoutError("timed out waiting for tmux pane terminal release")

    def wait_fixture_exit(self, timeout):
        pattern = re.compile(re.escape(FIXTURE_EXIT_PREFIX) + rb"([0-9]+)")
        start = len(self.output)
        self.send_line("printf '\\n%s%%s\\n' \"$?\"" % FIXTURE_EXIT_PREFIX.decode())
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            match = pattern.search(self.output[start:])
            if match:
                return int(match.group(1))
            time.sleep(0.01)
        raise TimeoutError("timed out waiting for tmux fixture status marker")

    def session_alive(self):
        return self._tmux("has-session", "-t", self.session, check=False).returncode == 0

    def finish_shell(self, expected_code, timeout):
        self.send_line("exit %d" % expected_code)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.drain()
            if not self.session_alive():
                self.closed = True
                return expected_code
            time.sleep(0.02)
        raise TimeoutError("timed out waiting for tmux shell exit")

    def close(self):
        self.drain()
        if self.session_alive():
            self._tmux("kill-session", "-t", self.session, check=False)
        if self.inspect_fd is not None:
            os.close(self.inspect_fd)
            self.inspect_fd = None
        self.temp_dir.cleanup()
        self.closed = True


def check(result, name, condition, detail=None):
    result["checks"][name] = bool(condition)
    if not condition:
        result["errors"].append(detail or name)


def new_result(name):
    return {"name": name, "ok": False, "checks": {}, "errors": [], "evidence": {}}


def record_output(result, process):
    output = bytes(process.output)
    result["evidence"]["escape_sequences"] = escape_evidence(output)
    result["evidence"]["output_bytes"] = len(output)
    result["evidence"]["output_sha256"] = __import__("hashlib").sha256(output).hexdigest()
    result["evidence"]["output_tail_base64"] = base64.b64encode(output[-512:]).decode()


def run_normal_scenario(binary, timeout, shell_command=None):
    result = new_result("normal_resize_suspend_resume")
    process = PtyProcess(binary, shell_command=shell_command)
    try:
        startup = process.wait_for_output(READY_MARKER, 0, timeout)
        startup_evidence = escape_evidence(startup)
        check(result, "rendered_initial_frame", startup_evidence["render"])
        check(result, "raw_while_active", termios_is_raw(process.current_termios()))
        check(result, "alternate_screen_entered", startup_evidence["alternate_screen_enter"])
        check(result, "cursor_hidden", startup_evidence["cursor_hide"])
        check(result, "mouse_enabled", startup_evidence["mouse_enable"])

        resize_start = len(process.output)
        process.resize(100, 30)
        resize_output = process.wait_for_output(READY_MARKER, resize_start, timeout)
        check(result, "resize_rendered", READY_MARKER in resize_output)
        check(result, "resize_applied", window_size(process.inspect_fd) == {"columns": 100, "rows": 30})
        result["evidence"]["resized_to"] = window_size(process.inspect_fd)

        suspend_start = len(process.output)
        process.write(b"\x1a")
        process.wait_for_terminal_release(timeout)
        process.wait_for_output(SHELL_PROMPT, suspend_start, timeout)
        suspend_output = bytes(process.output[suspend_start:])
        suspend_evidence = escape_evidence(suspend_output)
        check(
            result,
            "termios_released_while_stopped",
            termios_equal(process.baseline_termios, process.current_termios()),
        )
        check(result, "cursor_shown_while_stopped", suspend_evidence["cursor_show"])
        check(result, "mouse_disabled_while_stopped", suspend_evidence["mouse_disable"])
        check(result, "alternate_screen_exited_while_stopped", suspend_evidence["alternate_screen_exit"])

        resume_start = len(process.output)
        process.continue_process()
        resume_output = process.wait_for_output(READY_MARKER, resume_start, timeout)
        resume_evidence = escape_evidence(resume_output)
        check(result, "raw_after_fg", termios_is_raw(process.current_termios()))
        check(result, "rendered_after_fg", resume_evidence["render"])
        check(result, "alternate_screen_reentered", resume_evidence["alternate_screen_enter"])
        check(result, "cursor_rehidden", resume_evidence["cursor_hide"])
        check(result, "mouse_reenabled", resume_evidence["mouse_enable"])

        exit_start = len(process.output)
        process.write(b"q")
        process.wait_for_terminal_release(timeout)
        process.wait_for_output(SHELL_PROMPT, exit_start, timeout)
        exit_code = process.wait_fixture_exit(timeout)
        result["evidence"]["exit_code"] = exit_code
        check(result, "normal_exit_code", exit_code == 0, "exit code=%s" % exit_code)
        check(
            result,
            "termios_released_after_normal_exit",
            termios_equal(process.baseline_termios, process.current_termios()),
        )
        record_output(result, process)
        check(
            result,
            "complete_escape_cycle",
            escape_cycle_complete(result["evidence"]["escape_sequences"]),
        )
        process.finish_shell(exit_code, timeout)
    except BaseException as error:
        result["errors"].append("%s: %s" % (type(error).__name__, error))
        record_output(result, process)
    finally:
        process.close()
    result["ok"] = not result["errors"] and all(result["checks"].values())
    return result


def run_tmux_normal_scenario(binary, timeout, shell_command=None):
    result = new_result("tmux_normal_resize_suspend_resume")
    process = None
    try:
        process = TmuxPane(binary, shell_command=shell_command)
        result["evidence"]["backend"] = "tmux-pane"
        startup = process.wait_for_output(READY_MARKER, 0, timeout)
        startup_evidence = escape_evidence(startup)
        check(result, "rendered_initial_frame", startup_evidence["render"])
        check(result, "raw_while_active", termios_is_raw(process.current_termios()))
        check(result, "alternate_screen_entered", startup_evidence["alternate_screen_enter"])
        check(result, "cursor_hidden", startup_evidence["cursor_hide"])
        check(result, "mouse_enabled", startup_evidence["mouse_enable"])

        resize_start = len(process.output)
        process.resize(100, 30)
        resize_output = process.wait_for_output(READY_MARKER, resize_start, timeout)
        check(result, "resize_rendered", READY_MARKER in resize_output)
        check(result, "resize_applied", window_size(process.inspect_fd) == {"columns": 100, "rows": 30})
        result["evidence"]["resized_to"] = window_size(process.inspect_fd)

        suspend_start = len(process.output)
        process.send_key("C-z")
        process.wait_for_terminal_release(timeout)
        process.wait_for_output(SHELL_PROMPT, suspend_start, timeout)
        suspend_output = bytes(process.output[suspend_start:])
        suspend_evidence = escape_evidence(suspend_output)
        check(
            result,
            "termios_released_while_stopped",
            termios_equal(process.baseline_termios, process.current_termios()),
        )
        check(result, "cursor_shown_while_stopped", suspend_evidence["cursor_show"])
        check(result, "mouse_disabled_while_stopped", suspend_evidence["mouse_disable"])
        check(result, "alternate_screen_exited_while_stopped", suspend_evidence["alternate_screen_exit"])

        resume_start = len(process.output)
        process.send_line("fg")
        resume_output = process.wait_for_output(READY_MARKER, resume_start, timeout)
        resume_evidence = escape_evidence(resume_output)
        check(result, "raw_after_fg", termios_is_raw(process.current_termios()))
        check(result, "rendered_after_fg", resume_evidence["render"])
        check(result, "alternate_screen_reentered", resume_evidence["alternate_screen_enter"])
        check(result, "cursor_rehidden", resume_evidence["cursor_hide"])
        check(result, "mouse_reenabled", resume_evidence["mouse_enable"])

        exit_start = len(process.output)
        process.send_literal("q")
        process.wait_for_terminal_release(timeout)
        process.wait_for_output(SHELL_PROMPT, exit_start, timeout)
        exit_code = process.wait_fixture_exit(timeout)
        result["evidence"]["exit_code"] = exit_code
        check(result, "normal_exit_code", exit_code == 0, "exit code=%s" % exit_code)
        check(
            result,
            "termios_released_after_normal_exit",
            termios_equal(process.baseline_termios, process.current_termios()),
        )
        record_output(result, process)
        check(
            result,
            "complete_escape_cycle",
            escape_cycle_complete(result["evidence"]["escape_sequences"]),
        )
        process.finish_shell(exit_code, timeout)
    except BaseException as error:
        result["errors"].append("%s: %s" % (type(error).__name__, error))
        if process is not None:
            record_output(result, process)
    finally:
        if process is not None:
            process.close()
    result["ok"] = not result["errors"] and all(result["checks"].values())
    return result


def run_exit_scenario(binary, timeout, name, action, expected_code, shell_command=None):
    result = new_result(name)
    process = PtyProcess(binary, shell_command=shell_command)
    try:
        process.wait_for_output(READY_MARKER, 0, timeout)
        check(result, "raw_while_active", termios_is_raw(process.current_termios()))
        exit_start = len(process.output)
        if action == "term":
            process.terminate()
        elif action == "controlled_error":
            process.write(b"e")
        else:
            raise ValueError("unknown exit action: %s" % action)
        process.wait_for_terminal_release(timeout)
        process.wait_for_output(SHELL_PROMPT, exit_start, timeout)
        exit_code = process.wait_fixture_exit(timeout)
        result["evidence"]["exit_code"] = exit_code
        check(result, "expected_exit_code", exit_code == expected_code, "exit code=%s" % exit_code)
        check(
            result,
            "termios_released_after_exit",
            termios_equal(process.baseline_termios, process.current_termios()),
        )
        record_output(result, process)
        check(
            result,
            "complete_escape_cycle",
            escape_cycle_complete(result["evidence"]["escape_sequences"]),
        )
        if action == "controlled_error":
            check(
                result,
                "controlled_error_reported_after_cleanup",
                CONTROLLED_ERROR_MARKER in process.output,
            )
        process.finish_shell(exit_code, timeout)
    except BaseException as error:
        result["errors"].append("%s: %s" % (type(error).__name__, error))
        record_output(result, process)
    finally:
        process.close()
    result["ok"] = not result["errors"] and all(result["checks"].values())
    return result


def fixture_build_command(output_path):
    return [
        "go",
        "build",
        "-buildvcs=false",
        "-o",
        str(output_path),
        "./cmd/terminal-fixture",
    ]


def build_fixture(module_root, output_path):
    completed = subprocess.run(
        fixture_build_command(output_path),
        cwd=module_root,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        raise RuntimeError("fixture build failed: %s" % completed.stderr.strip())


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", help="prebuilt terminal-fixture binary")
    parser.add_argument(
        "--backend",
        choices=("openpty", "tmux", "both"),
        default="openpty",
        help="real terminal backend to exercise (default: openpty)",
    )
    parser.add_argument(
        "--shell",
        help="interactive job-control shell (default: bash with startup files disabled)",
    )
    parser.add_argument("--timeout", type=float, default=5.0, help="seconds per lifecycle transition")
    parser.add_argument("--pretty", action="store_true", help="pretty-print JSON evidence")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    module_root = pathlib.Path(__file__).resolve().parents[1]
    with tempfile.TemporaryDirectory(prefix="go-tui-terminal-matrix-") as temp_dir:
        binary = pathlib.Path(args.binary).resolve() if args.binary else pathlib.Path(temp_dir) / "terminal-fixture"
        build_error = None
        if not args.binary:
            try:
                build_fixture(module_root, binary)
            except BaseException as error:
                build_error = "%s: %s" % (type(error).__name__, error)

        scenarios = []
        environment_errors = []
        shell_command = None
        try:
            shell_command = resolve_job_control_shell(args.shell)
        except RuntimeError as error:
            environment_errors.append(str(error))
        if build_error is None and shell_command is not None:
            if args.backend in ("openpty", "both"):
                scenarios.append(run_normal_scenario(str(binary), args.timeout, shell_command))
                scenarios.append(
                    run_exit_scenario(
                        str(binary), args.timeout, "sigterm", "term", 0, shell_command
                    )
                )
                scenarios.append(
                    run_exit_scenario(
                        str(binary),
                        args.timeout,
                        "controlled_nonzero_exit",
                        "controlled_error",
                        CONTROLLED_ERROR_EXIT_CODE,
                        shell_command,
                    )
                )
            if args.backend in ("tmux", "both"):
                if shutil.which("tmux"):
                    scenarios.append(
                        run_tmux_normal_scenario(str(binary), args.timeout, shell_command)
                    )
                else:
                    environment_errors.append("tmux backend requested but tmux is not installed")

        report = {
            "schema": SCHEMA,
            "ok": build_error is None
            and not environment_errors
            and bool(scenarios)
            and all(item["ok"] for item in scenarios),
            "host": {
                "system": platform.system(),
                "release": platform.release(),
                "machine": platform.machine(),
                "python": platform.python_version(),
                "term": os.environ.get("TERM", ""),
                "terminal_program": os.environ.get("TERM_PROGRAM", ""),
                "inside_tmux": bool(os.environ.get("TMUX")),
                "inside_ide_terminal": detect_ide_terminal(os.environ),
            },
            "fixture": str(binary),
            "backend": args.backend,
            "job_control_shell": shell_command,
            "build_error": build_error,
            "environment_errors": environment_errors,
            "scenarios": scenarios,
        }
        print(json.dumps(report, indent=2 if args.pretty else None, sort_keys=True))
        return 0 if report["ok"] else 1


if __name__ == "__main__":
    sys.exit(main())
