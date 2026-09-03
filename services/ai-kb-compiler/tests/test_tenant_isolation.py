import os
import uuid
import pytest
from db import ScopedDB, create_db_pool

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
)

@pytest.mark.asyncio
async def test_tenant_isolation():
    pool = await create_db_pool(DATABASE_URL)
    
    # Create two isolated accounts
    account_a = uuid.uuid4()
    account_b = uuid.uuid4()

    try:
        async with pool.acquire() as conn:
            await conn.execute(
                "INSERT INTO accounts (id, name, plan) VALUES ($1, 'Python Account A', 'self_hosted'), ($2, 'Python Account B', 'self_hosted')",
                account_a, account_b
            )

            try:
                # Insert a concept belonging to Account B
                concept_id_b = uuid.uuid4()
                await conn.execute(
                    """
                    INSERT INTO kb_concepts (id, account_id, slug, type, title, body_text, source)
                    VALUES ($1, $2, 'b-slug', 'faq', 'B Title', 'B body', 'owner_pasted')
                    """,
                    concept_id_b, account_b
                )

                # Create a ScopedDB bound to Account A
                sdb_a = ScopedDB(pool, account_a)

                # Query using ScopedDB and check if we can read Account B's row
                pos, args = sdb_a.append_account_id([concept_id_b])
                query = f"SELECT id FROM kb_concepts WHERE id = $1 AND account_id = ${pos}"
                row = await sdb_a.fetchrow(query, *args)

                assert row is None, "Account A must not be able to read Account B's concept row"

            finally:
                # Clean up
                await conn.execute("DELETE FROM kb_concepts WHERE account_id IN ($1, $2)", account_a, account_b)
                await conn.execute("DELETE FROM accounts WHERE id IN ($1, $2)", account_a, account_b)
    finally:
        await pool.close()
