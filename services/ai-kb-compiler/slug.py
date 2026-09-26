import re
from db import ScopedDB


def slugify(title: str) -> str:
    """Generate a URL-friendly slug from a title string."""
    s = title.lower()
    s = re.sub(r'[^a-z0-9\s-]', '', s)
    s = re.sub(r'[\s-]+', '-', s)
    return s.strip('-')


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
