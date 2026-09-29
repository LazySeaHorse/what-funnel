import pytest
from fastapi.testclient import TestClient

TEST_INTERNAL_TOKEN = "test-internal-service-token-32-chars"


@pytest.fixture(autouse=True)
def _internal_auth(monkeypatch):
    """Configure a dedicated internal token and make TestClient send it by default."""
    monkeypatch.setenv("INTERNAL_SERVICE_TOKEN", TEST_INTERNAL_TOKEN)
    monkeypatch.delenv("ALLOW_INSECURE_INTERNAL_AUTH", raising=False)
    monkeypatch.delenv("SESSION_SECRET", raising=False)
    original_init = TestClient.__init__

    def init(self, *args, **kwargs):
        headers = dict(kwargs.pop("headers", None) or {})
        headers.setdefault("X-Internal-Token", TEST_INTERNAL_TOKEN)
        original_init(self, *args, headers=headers, **kwargs)

    monkeypatch.setattr(TestClient, "__init__", init)
