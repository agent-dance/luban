"""Benchmark-only Docker environment for nested tool-sandbox conformance."""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import re
import sys
from pathlib import Path

sys.dont_write_bytecode = True

from pier.environments.agent_setup import (
    EGRESS_PROXY_PORT,
    EGRESS_PROXY_SERVICE,
    new_proxy_token,
    proxy_environment,
)
from pier.environments.docker.docker import DockerEnvironment

from benchmark.agentic.pier.pilot_guest_storage import (
    DECLARED_STORAGE_MB,
    PILOT_GUEST_STORAGE_RECEIPT,
    PilotGuestStorageFailure,
    PrivilegedPilotGuestStorageGuard,
)


_DIGEST_IMAGE = re.compile(r"^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$")
_APR1_ALPHABET = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"


def _apr1_password(password: str, salt: str) -> str:
    """Return Apache's apr1-md5 form accepted by Squid basic_ncsa_auth."""

    if not re.fullmatch(r"[A-Za-z0-9./]{1,8}", salt):
        raise ValueError("apr1 salt is invalid")
    secret = password.encode("utf-8")
    salt_bytes = salt.encode("ascii")
    magic = b"$apr1$"
    digest = hashlib.md5(
        secret + salt_bytes + secret, usedforsecurity=False
    ).digest()
    state = bytearray(secret + magic + salt_bytes)
    remaining = len(secret)
    while remaining > 0:
        state.extend(digest[: min(16, remaining)])
        remaining -= 16
    length = len(secret)
    while length:
        state.extend(b"\x00" if length & 1 else secret[:1])
        length >>= 1
    digest = hashlib.md5(state, usedforsecurity=False).digest()
    for index in range(1000):
        round_input = bytearray(secret if index & 1 else digest)
        if index % 3:
            round_input.extend(salt_bytes)
        if index % 7:
            round_input.extend(secret)
        round_input.extend(digest if index & 1 else secret)
        digest = hashlib.md5(round_input, usedforsecurity=False).digest()

    def encode(first: int, second: int, third: int, count: int) -> str:
        value = (first << 16) | (second << 8) | third
        output = []
        for _ in range(count):
            output.append(_APR1_ALPHABET[value & 0x3F])
            value >>= 6
        return "".join(output)

    encoded = "".join(
        (
            encode(digest[0], digest[6], digest[12], 4),
            encode(digest[1], digest[7], digest[13], 4),
            encode(digest[2], digest[8], digest[14], 4),
            encode(digest[3], digest[9], digest[15], 4),
            encode(digest[4], digest[10], digest[5], 4),
            encode(0, 0, digest[11], 2),
        )
    )
    return f"$apr1${salt}${encoded}"


def agent_security_overlay(domains: list[str]) -> dict | None:
    """Return the minimal overlay only for the agent's filtered-egress container."""

    if not domains:
        return None
    if len(domains) != 1:
        raise ValueError("formal benchmark requires exactly one private proxy host")
    host_gateway = [f"{domains[0]}:host-gateway"]
    return {
        "services": {
            "main": {
                "security_opt": [
                    "seccomp=unconfined",
                    "apparmor=unconfined",
                ],
                "extra_hosts": host_gateway,
            },
            # The controller reaches the host evidence meter only through
            # Pier's authenticated filtered-egress proxy. This mapping grants
            # no extra destination because Squid still enforces the allowlist.
            EGRESS_PROXY_SERVICE: {"extra_hosts": host_gateway},
        }
    }


def write_agent_security_overlay(path: Path, domains: list[str]) -> Path | None:
    overlay = agent_security_overlay(domains)
    if overlay is None:
        return None
    path.write_text(json.dumps(overlay, sort_keys=True) + "\n", encoding="utf-8")
    os.chmod(path, 0o600)
    return path


def frozen_squid_config(domain: str, port: int) -> str:
    if port < 1 or port > 65535:
        raise ValueError("private proxy port is outside the TCP port range")
    if domain != "host.docker.internal":
        raise ValueError("formal proxy domain is not the private Docker host alias")
    return f"""http_port 0.0.0.0:{EGRESS_PROXY_PORT}
pid_filename /tmp/squid.pid
coredump_dir /tmp

auth_param basic program /usr/lib/squid/basic_ncsa_auth /etc/squid/agentic-bench.passwd
auth_param basic realm AgenticBenchmark
acl authenticated proxy_auth REQUIRED

acl SSL_ports port 443
acl Safe_ports port {port}
acl CONNECT method CONNECT
acl allowed_domains dstdomain {domain}

http_access deny !Safe_ports
http_access deny CONNECT !SSL_ports
http_access allow authenticated allowed_domains
http_access deny all

cache deny all
access_log stdio:/tmp/squid_access.log
cache_log /tmp/squid_cache.log
log_mime_hdrs off
shutdown_lifetime 1 seconds
"""


def write_frozen_egress_proxy_compose(
    *,
    path: Path,
    image: str,
    domain: str,
    port: int,
    token: str,
) -> Path:
    """Write a no-build/no-pull sidecar bound to one host and meter port."""

    if _DIGEST_IMAGE.fullmatch(image) is None:
        raise ValueError("egress proxy image must be a repository digest reference")
    policy_dir = path.parent / "egress-proxy"
    policy_dir.mkdir(parents=True, exist_ok=False)
    config_path = policy_dir / "squid.conf"
    password_path = policy_dir / "passwd"
    config_path.write_text(frozen_squid_config(domain, port), encoding="utf-8")
    password_path.write_text(
        f"agent:{_apr1_password(token, 'agentic1')}\n", encoding="ascii"
    )
    os.chmod(config_path, 0o444)
    os.chmod(password_path, 0o444)
    compose = {
        "services": {
            "main": {
                "networks": ["pier-egress-internal"],
                "depends_on": {
                    EGRESS_PROXY_SERVICE: {"condition": "service_healthy"}
                },
            },
            EGRESS_PROXY_SERVICE: {
                "image": image,
                "pull_policy": "never",
                "entrypoint": ["/usr/local/bin/entrypoint.sh"],
                "command": ["-f", "/etc/squid/squid.conf", "-NYC"],
                "read_only": True,
                "volumes": [
                    {
                        "type": "bind",
                        "source": str(config_path.resolve()),
                        "target": "/etc/squid/squid.conf",
                        "read_only": True,
                    },
                    {
                        "type": "bind",
                        "source": str(password_path.resolve()),
                        "target": "/etc/squid/agentic-bench.passwd",
                        "read_only": True,
                    },
                ],
                "tmpfs": [
                    "/tmp:uid=584792,gid=584792,mode=1777",
                    "/var/log/squid:uid=584792,gid=584792,mode=0755",
                    "/var/spool/squid:uid=584792,gid=584792,mode=0755",
                ],
                "healthcheck": {
                    "test": [
                        "CMD",
                        "/usr/bin/sh",
                        "-c",
                        'test -s /tmp/squid.pid && kill -0 "$(cat /tmp/squid.pid)"',
                    ],
                    "interval": "1s",
                    "timeout": "1s",
                    "retries": 30,
                },
                "networks": ["pier-egress-internal", "default"],
            },
        },
        "networks": {"pier-egress-internal": {"internal": True}},
    }
    path.write_text(json.dumps(compose, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.chmod(path, 0o600)
    return path


class AgenticBenchmarkDockerEnvironment(DockerEnvironment):
    """Enable unprivileged nested bwrap only in the agent main service.

    Pier constructs the separate verifier with an empty network allowlist, so
    it deliberately receives no security override and retains Docker's default
    seccomp/AppArmor policy plus Pier's no-network compose layer.
    """

    @classmethod
    def preflight(cls) -> None:
        super().preflight()
        restriction = Path(
            "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"
        )
        if sys.platform.startswith("linux") and restriction.exists():
            if restriction.read_text(encoding="ascii").strip() != "0":
                raise SystemExit(
                    "kernel.apparmor_restrict_unprivileged_userns must be 0 for the frozen nested-sandbox benchmark"
                )

    def __init__(
        self,
        *args,
        private_proxy_port: int | str | None = None,
        egress_proxy_image: str | None = None,
        **kwargs,
    ):
        self._agent_security_compose_path: Path | None = None
        self._egress_proxy_image = egress_proxy_image
        self._pilot_guest_storage_guard: PrivilegedPilotGuestStorageGuard | None = (
            None
        )
        if private_proxy_port is None:
            self._private_proxy_port = None
        else:
            if isinstance(private_proxy_port, bool):
                raise ValueError("private proxy port must be an integer")
            try:
                self._private_proxy_port = int(private_proxy_port)
            except (TypeError, ValueError) as error:
                raise ValueError("private proxy port must be an integer") from error
            if self._private_proxy_port < 1 or self._private_proxy_port > 65535:
                raise ValueError("private proxy port is outside the TCP port range")
        super().__init__(*args, **kwargs)
        domains = list(self.network_allowlist.domains)
        if domains and self.task_env_config.allow_internet:
            raise ValueError(
                "nested-sandbox security overlay requires Pier filtered egress"
            )
        if domains and self._is_windows_container:
            raise ValueError("nested-sandbox security overlay requires a Linux task")
        if domains and self._private_proxy_port is None:
            raise ValueError("agent environment requires the exact private proxy port")
        if domains and (
            not isinstance(self._egress_proxy_image, str)
            or _DIGEST_IMAGE.fullmatch(self._egress_proxy_image) is None
        ):
            raise ValueError("agent environment requires a digest-pinned proxy image")
        self._agent_security_compose_path = write_agent_security_overlay(
            self.trial_paths.trial_dir
            / "docker-compose-agent-nested-sandbox.json",
            domains,
        )

    def _prepare_egress_proxy_compose(self) -> None:
        domains = list(self.network_allowlist.domains)
        if self.task_env_config.allow_internet or not domains:
            return
        if self._uses_compose:
            raise ValueError(
                "filtered inference egress requires a Dockerfile or prebuilt-image task"
            )
        if self._private_proxy_port is None:
            raise ValueError("filtered egress was created without a private proxy port")
        if self._egress_proxy_image is None:
            raise ValueError("filtered egress was created without a pinned proxy image")
        token = new_proxy_token()
        self._egress_proxy_env = proxy_environment(
            token, EGRESS_PROXY_SERVICE, EGRESS_PROXY_PORT
        )
        self._egress_proxy_compose_path = write_frozen_egress_proxy_compose(
            path=self.trial_paths.trial_dir / "docker-compose-egress-proxy.json",
            image=self._egress_proxy_image,
            domain=domains[0],
            port=self._private_proxy_port,
            token=token,
        )

    async def _pilot_docker_command(self, argv: list[str]) -> bytes:
        if not argv or argv[0] != "docker":
            raise PilotGuestStorageFailure("docker_command_invalid")
        process = await asyncio.create_subprocess_exec(
            *argv,
            stdin=asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            stdout, _ = await asyncio.wait_for(process.communicate(), timeout=15)
        except BaseException:
            if process.returncode is None:
                process.kill()
                await process.communicate()
            raise
        if process.returncode != 0 or len(stdout) > (1 << 20):
            raise PilotGuestStorageFailure("docker_command_failed")
        return stdout

    async def _pilot_compose_command(self, argv: list[str]) -> bytes:
        result = await self._run_docker_compose_command(
            argv, check=True, timeout_sec=15
        )
        return (result.stdout or "").encode("utf-8")

    async def start(self, force_build: bool):
        if self.task_env_config.storage_mb != DECLARED_STORAGE_MB:
            raise PilotGuestStorageFailure("declared_storage_invalid")
        await super().start(force_build)
        receipt_directory = (
            self.trial_paths.verifier_dir
            if "__verifier__" in self.session_id
            else self.trial_paths.agent_dir
        )
        guard = PrivilegedPilotGuestStorageGuard(
            session_id=self.session_id,
            receipt_path=receipt_directory / PILOT_GUEST_STORAGE_RECEIPT,
            docker_command=self._pilot_docker_command,
            compose_command=self._pilot_compose_command,
        )
        try:
            await guard.start()
        except BaseException:
            self._pilot_guest_storage_guard = None
            await guard.finish()
            await super().stop(delete=False)
            raise
        self._pilot_guest_storage_guard = guard

    async def exec(
        self,
        command: str,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
        timeout_sec: int | None = None,
        user: str | int | None = None,
    ):
        guard = self._pilot_guest_storage_guard
        if guard is None:
            return await super().exec(command, cwd, env, timeout_sec, user)
        operation = asyncio.create_task(
            super().exec(command, cwd, env, timeout_sec, user)
        )
        try:
            done, _ = await asyncio.wait(
                (operation, guard.failure), return_when=asyncio.FIRST_COMPLETED
            )
            if guard.failure in done:
                failure = guard.failure.result()
                try:
                    await asyncio.wait_for(asyncio.shield(operation), timeout=10)
                except Exception:
                    if not operation.done():
                        operation.cancel()
                        await asyncio.gather(operation, return_exceptions=True)
                raise failure
            return await operation
        except BaseException:
            if not operation.done():
                operation.cancel()
                await asyncio.gather(operation, return_exceptions=True)
            raise

    async def stop(self, delete: bool):
        guard = self._pilot_guest_storage_guard
        self._pilot_guest_storage_guard = None
        failure = await guard.finish() if guard is not None else None
        try:
            await super().stop(delete)
        finally:
            if failure is not None:
                raise failure

    @property
    def _docker_compose_paths(self) -> list[Path]:
        paths = list(super()._docker_compose_paths)
        if self._agent_security_compose_path is not None:
            paths.append(self._agent_security_compose_path)
        return paths
