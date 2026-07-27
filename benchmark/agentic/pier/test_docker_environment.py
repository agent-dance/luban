from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from benchmark.agentic.pier.docker_environment import (
    _apr1_password,
    agent_security_overlay,
    frozen_squid_config,
    write_agent_security_overlay,
    write_frozen_egress_proxy_compose,
)


class AgentSecurityOverlayTest(unittest.TestCase):
    def test_agent_gets_only_required_main_service_security_options(self) -> None:
        self.assertEqual(
            agent_security_overlay(["host.docker.internal"]),
            {
                "services": {
                    "main": {
                        "security_opt": [
                            "seccomp=unconfined",
                            "apparmor=unconfined",
                        ],
                        "extra_hosts": [
                            "host.docker.internal:host-gateway"
                        ],
                    },
                    "pier-egress-proxy": {
                        "extra_hosts": [
                            "host.docker.internal:host-gateway"
                        ]
                    },
                }
            },
        )

    def test_empty_verifier_allowlist_gets_no_overlay(self) -> None:
        self.assertIsNone(agent_security_overlay([]))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "overlay.json"
            self.assertIsNone(write_agent_security_overlay(path, []))
            self.assertFalse(path.exists())

    def test_written_overlay_is_private_and_valid_compose_json(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "overlay.json"
            self.assertEqual(
                write_agent_security_overlay(path, ["host.docker.internal"]), path
            )
            self.assertEqual(
                json.loads(path.read_text(encoding="utf-8")),
                agent_security_overlay(["host.docker.internal"]),
            )
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_squid_allows_only_the_exact_ephemeral_meter_port(self) -> None:
        source = frozen_squid_config("host.docker.internal", 43123)
        self.assertIn("acl Safe_ports port 43123\n", source)
        self.assertNotIn("acl Safe_ports port 80 443", source)
        self.assertIn("acl allowed_domains dstdomain host.docker.internal", source)
        with self.assertRaises(ValueError):
            frozen_squid_config("host.docker.internal", 0)

    def test_frozen_proxy_compose_cannot_build_or_pull(self) -> None:
        image = "ubuntu/squid@sha256:" + "a" * 64
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "docker-compose-egress-proxy.json"
            write_frozen_egress_proxy_compose(
                path=path,
                image=image,
                domain="host.docker.internal",
                port=43123,
                token="ephemeral-secret",
            )
            compose = json.loads(path.read_text(encoding="utf-8"))
            service = compose["services"]["pier-egress-proxy"]
            self.assertEqual(service["image"], image)
            self.assertEqual(service["pull_policy"], "never")
            self.assertNotIn("build", service)
            self.assertTrue(service["read_only"])
            password = (Path(directory) / "egress-proxy" / "passwd").read_text()
            self.assertNotIn("ephemeral-secret", password)
            self.assertTrue(password.startswith("agent:$apr1$agentic1$"))

    def test_apr1_password_matches_apache_vector(self) -> None:
        self.assertEqual(
            _apr1_password("test-secret", "abcdefgh"),
            "$apr1$abcdefgh$I827fpT6RWDyUDmYZxZ4z/",
        )

    def test_frozen_proxy_rejects_mutable_image_reference(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(ValueError):
                write_frozen_egress_proxy_compose(
                    path=Path(directory) / "compose.json",
                    image="ubuntu/squid:latest",
                    domain="host.docker.internal",
                    port=43123,
                    token="ephemeral-secret",
                )


if __name__ == "__main__":
    unittest.main()
