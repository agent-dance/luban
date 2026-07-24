"""Behavioral tests for fallback signer configuration."""
import hashlib
import sys
sys.path.insert(0, "src")

from itsdangerous.serializer import Serializer

class TestIterFallbackSigners:
    def test_module_importable(self):
        old = Serializer("secret", signer_kwargs={"digest_method": hashlib.sha512})
        signed = old.dumps({"id": 1})

        new = Serializer("secret", fallback_signers=[{"digest_method": hashlib.sha512}])
        assert new.loads(signed) == {"id": 1}
