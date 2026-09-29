import os

import pytest

# Tests must not depend on the developer's environment for the inter-service token.
os.environ.setdefault("INTERNAL_SERVICE_TOKEN", "test-internal-service-token-32-chars")


@pytest.fixture(autouse=True)
def _internal_token(monkeypatch):
    monkeypatch.setattr("config.config.INTERNAL_SERVICE_TOKEN", os.environ["INTERNAL_SERVICE_TOKEN"], raising=False)
