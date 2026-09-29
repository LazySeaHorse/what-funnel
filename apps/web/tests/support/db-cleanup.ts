// @ts-ignore
import { execFileSync } from 'child_process';
// @ts-ignore
import { existsSync, rmSync, writeFileSync } from 'fs';
// @ts-ignore
import { tmpdir } from 'os';
// @ts-ignore
import { join } from 'path';

declare const process: any;

/**
 * Every account the E2E suites create through the real API is marked by an
 * e-mail address on this reserved (RFC 6761 ".test") domain. Cleanup deletes
 * ONLY accounts whose users are all on this domain, never anything else that
 * happens to live in the dev database.
 */
export const E2E_EMAIL_DOMAIN = 'e2e.whatfunnel.test';

// Set explicitly (e.g. by `make pw-fuzz-live` for the fuzz DB). When it is not
// set we may fall back to `docker exec` against the default dev stack.
const EXPLICIT_DATABASE_URL: string | undefined = process.env.DATABASE_URL;
const DATABASE_URL =
	EXPLICIT_DATABASE_URL || 'postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable';

/** Per-run identifier, created by global setup and inherited by workers via env. */
export function e2eRunId(): string {
	if (!process.env.E2E_RUN_ID) {
		process.env.E2E_RUN_ID = `${Date.now().toString(36)}${Math.floor(Math.random() * 0xffff).toString(36)}`;
	}
	return process.env.E2E_RUN_ID as string;
}

function markerFile(): string {
	return join(tmpdir(), `whatfunnel-e2e-${e2eRunId()}.used-db`);
}

/**
 * Build a unique marked e-mail for a test that creates a real account. Calling
 * this records that the run touched the database, so teardown knows cleanup is
 * needed. Mock-only runs never call it and therefore never connect to the DB.
 */
export function e2eEmail(prefix: string): string {
	try {
		writeFileSync(markerFile(), '1');
	} catch (err) {
		console.warn('[e2e] could not record DB usage marker; account cleanup may be skipped:', err);
	}
	return `${prefix}-${e2eRunId()}-${Math.floor(Math.random() * 1_000_000)}@${E2E_EMAIL_DOMAIN}`;
}

export function dbWasUsed(): boolean {
	return existsSync(markerFile());
}

export function clearDbUsageMarker(): void {
	rmSync(markerFile(), { force: true });
}

function attempt(cmd: string, args: string[]): { ok: true } | { ok: false; error: string } {
	try {
		execFileSync(cmd, args, { stdio: 'pipe', timeout: 10000 });
		return { ok: true };
	} catch (err: any) {
		const stderr = err?.stderr?.toString?.().trim() || '';
		return { ok: false, error: `${cmd}: ${stderr || err?.message || err}` };
	}
}

/** Run SQL. Returns null on success, or a description of every failed attempt. */
export function runSql(sql: string): string | null {
	const failures: string[] = [];

	const direct = attempt('psql', [DATABASE_URL, '-v', 'ON_ERROR_STOP=1', '-c', sql]);
	if (direct.ok) return null;
	failures.push(direct.error);

	// Docker fallbacks only target the default dev database; never use them when
	// the caller pointed us at a specific database.
	if (!EXPLICIT_DATABASE_URL) {
		const pgArgs = ['psql', '-v', 'ON_ERROR_STOP=1', '-U', 'whatfunnel', '-d', 'whatfunnel', '-c', sql];
		const exec = attempt('docker', ['exec', 'whatfunnel-postgres', ...pgArgs]);
		if (exec.ok) return null;
		failures.push(exec.error);

		const compose = attempt('docker', ['compose', 'exec', '-T', 'postgres', ...pgArgs]);
		if (compose.ok) return null;
		failures.push(compose.error);
	}
	return failures.join('\n  ');
}

function warnCleanupFailure(what: string, detail: string): void {
	console.error(
		`\n!!! [e2e] DATABASE CLEANUP FAILED (${what}). Test accounts may be left in the database.\n  ${detail}\n`
	);
}

const MARKER_LIKE = `'%@${E2E_EMAIL_DOMAIN}'`;

/**
 * Remove accounts created by E2E runs: accounts whose users ALL use the E2E
 * marker domain. Accounts with any real user are left untouched.
 */
export function cleanupAllTestAccounts(): void {
	const sql = `
		BEGIN;
		DELETE FROM accounts
		WHERE id IN (
			SELECT account_id FROM users
			GROUP BY account_id
			HAVING bool_and(email LIKE ${MARKER_LIKE})
		);
		DELETE FROM users WHERE email LIKE ${MARKER_LIKE};
		COMMIT;
	`;
	const failure = runSql(sql);
	if (failure) warnCleanupFailure('all marked accounts', failure);
}

/**
 * Delete a specific test account by e-mail. Refuses any address outside the
 * E2E marker domain.
 */
export function cleanupAccountByEmail(email: string): void {
	if (!email) return;
	if (!email.endsWith(`@${E2E_EMAIL_DOMAIN}`)) {
		console.warn(`[e2e] refusing to clean up non-E2E account: ${email}`);
		return;
	}
	const escaped = email.replace(/'/g, "''");
	const sql = `
		BEGIN;
		DELETE FROM accounts
		WHERE id IN (SELECT account_id FROM users WHERE email = '${escaped}')
		  AND id IN (
			SELECT account_id FROM users
			GROUP BY account_id
			HAVING bool_and(email LIKE ${MARKER_LIKE})
		);
		DELETE FROM users WHERE email = '${escaped}';
		COMMIT;
	`;
	const failure = runSql(sql);
	if (failure) warnCleanupFailure(`account ${email}`, failure);
}
