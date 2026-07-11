import logging
import asyncpg
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from apscheduler.triggers.interval import IntervalTrigger

from db import ScopedDB
from mining import run_mining
from config import config

logger = logging.getLogger("ai-kb-compiler")

async def scheduled_mining_job(pool: asyncpg.Pool):
    logger.info("Starting scheduled conversation mining job across all accounts...")
    
    # 1. Fetch all accounts
    try:
        rows = await pool.fetch("SELECT id FROM accounts")
    except Exception as e:
        logger.error(f"Failed to fetch accounts for scheduled mining: {e}")
        return

    logger.info(f"Found {len(rows)} accounts to scan.")

    # 2. Iterate and run mining for each account
    for row in rows:
        account_id = row["id"]
        logger.info(f"Running conversation mining for account {account_id}...")
        
        try:
            db = ScopedDB(pool, account_id)
            result = await run_mining(db)
            logger.info(
                f"Completed mining for account {account_id}: "
                f"scanned={result['messages_scanned']}, "
                f"found={result['clusters_found']}, "
                f"created={result['suggestions_created']}"
            )
        except Exception as e:
            logger.error(f"Error during conversation mining for account {account_id}: {e}", exc_info=True)

    logger.info("Scheduled conversation mining job completed.")

def start_scheduler(pool: asyncpg.Pool):
    scheduler = AsyncIOScheduler()
    
    # Add interval job
    scheduler.add_job(
        scheduled_mining_job,
        trigger=IntervalTrigger(hours=config.MINING_INTERVAL_HOURS),
        args=[pool],
        id="scheduled_mining_job",
        name="Scheduled Conversation Mining",
        replace_existing=True
    )
    
    scheduler.start()
    logger.info(f"In-process scheduler started. Running mining job every {config.MINING_INTERVAL_HOURS} hours.")
    return scheduler
