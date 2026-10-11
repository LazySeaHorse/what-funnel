import uuid

import pytest

from whatfunnel_ai.db import json_object, load_account_settings


@pytest.mark.parametrize(
    "raw, expected",
    [
        ('{"a": 1}', {"a": 1}),
        (b'{"a": 1}', {"a": 1}),
        ({"a": 1}, {"a": 1}),
        ("[1, 2]", {}),
        ("not json", {}),
        (None, {}),
    ],
)
def test_json_object(raw, expected):
    assert json_object(raw) == expected


class _DB:
    def __init__(self, row):
        self.row, self.account_id, self.args = row, uuid.uuid4(), None

    async def fetchrow(self, query, *args):
        self.args = args
        return self.row


@pytest.mark.asyncio
async def test_load_account_settings_is_scoped_and_tolerates_missing_rows():
    db = _DB({"settings": '{"ai_enabled": false}'})
    assert await load_account_settings(db) == {"ai_enabled": False}
    assert db.args == (db.account_id,)
    assert await load_account_settings(_DB(None)) == {}
