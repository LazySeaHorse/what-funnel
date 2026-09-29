import re
from db import ScopedDB


def slugify(title: str) -> str:
    """Generate a URL-friendly slug from a title string (may be empty)."""
    s = (title or "").lower()
    s = re.sub(r'[^a-z0-9\s-]', '', s)
    s = re.sub(r'[\s-]+', '-', s)
    return s.strip('-')


def concept_base_slug(title: str) -> str:
    """Slug for a concept title, falling back to "concept" when nothing usable remains."""
    return slugify(title) or "concept"


async def lock_concept_slugs(db: ScopedDB) -> None:
    """Serialise slug allocation for the account until the current transaction ends.

    Must be called on a transaction-bound ScopedDB (see ScopedDB.transaction()); it makes the
    check-then-insert in get_unique_slug() race free across concurrent requests/replicas.
    """
    await db.execute(
        "SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))",
        f"kb_concept_slug:{db.account_id}",
    )


async def get_unique_slug(db: ScopedDB, base_slug: str) -> str:
    """Find an unused slug in the account's kb_concepts, appending suffix if needed."""
    slug = base_slug
    suffix = 1
    while True:
        row = await db.fetchrow(
            "SELECT id FROM kb_concepts WHERE account_id = $1 AND slug = $2",
            db.account_id, slug
        )
        if not row:
            return slug
        slug = f"{base_slug}-{suffix}"
        suffix += 1
