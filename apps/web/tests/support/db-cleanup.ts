// @ts-ignore
import { execFileSync } from 'child_process';

declare const process: any;

const DATABASE_URL =
	process.env.DATABASE_URL ||
	'postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable';

export function runSql(sql: string): void {
	// Attempt 1: direct psql CLI if available
	try {
		execFileSync('psql', [DATABASE_URL, '-c', sql], {
			stdio: 'pipe',
			timeout: 10000
		});
		return;
	} catch {
		// psql not in PATH or connection failed, fallback to docker
	}

	// Attempt 2: docker exec whatfunnel-postgres
	try {
		execFileSync(
			'docker',
			['exec', 'whatfunnel-postgres', 'psql', '-U', 'whatfunnel', '-d', 'whatfunnel', '-c', sql],
			{
				stdio: 'pipe',
				timeout: 10000
			}
		);
		return;
	} catch {
		// fallback to docker compose
	}

	// Attempt 3: docker compose exec
	try {
		execFileSync(
			'docker',
			['compose', 'exec', '-T', 'postgres', 'psql', '-U', 'whatfunnel', '-d', 'whatfunnel', '-c', sql],
			{
				stdio: 'pipe',
				timeout: 10000
			}
		);
	} catch (err) {
		console.warn('DB cleanup execution failed:', err);
	}
}

/**
 * Remove all test accounts and synthetic users.
 * Safeguard: Never deletes foo@barr.com or account Foobarr.
 */
export function cleanupAllTestAccounts(): void {
	const sql = `
		BEGIN;
		DELETE FROM accounts
		WHERE id NOT IN (
			SELECT account_id FROM users WHERE email = 'foo@barr.com'
		) AND (
			id IN (
				SELECT DISTINCT account_id FROM users
				WHERE (email LIKE '%@e2e.local' OR email LIKE '%@example.com' OR email LIKE '%@local.test')
				  AND email != 'foo@barr.com'
			)
			OR name LIKE 'What Funnel Studio%'
			OR name LIKE 'Glamour Salon%'
			OR name LIKE 'Realtime Sync Studio%'
			OR name LIKE 'Telegram Sim Test Studio%'
			OR name LIKE '%Test Biz%'
			OR name LIKE 'Done Screen Test%'
			OR name LIKE 'Biz Type Test%'
			OR name LIKE 'Mode Test Biz%'
			OR name LIKE 'Reply Mode Test%'
			OR name LIKE 'RBAC % Biz%'
			OR name LIKE 'UI Safety Test Workspace%'
			OR name LIKE 'E2E %'
			OR name = 'TestTenant'
			OR name = 'Onboarding Test Biz'
			OR name = 'Mode Test Biz'
			OR name = 'Biz Type Test'
			OR name = 'Done Screen Test'
			OR name = 'Reply Mode Test'
			OR name = 'Channel Test Biz'
			OR name = 'Inbound Test Biz'
			OR name = 'Outbound Test Biz'
			OR name = 'Lead Test Biz'
			OR name = 'Filter Test Biz'
			OR name = 'Settings Test Biz'
			OR name = 'WS Test Biz'
			OR name = 'Auth Test Business'
			OR name = 'Login Test Biz'
		);

		DELETE FROM users
		WHERE (email LIKE '%@e2e.local' OR email LIKE '%@example.com' OR email LIKE '%@local.test')
		  AND email != 'foo@barr.com';
		COMMIT;
	`;
	runSql(sql);
}

/**
 * Delete a specific test account and user by email.
 * Safeguard: Never deletes foo@barr.com.
 */
export function cleanupAccountByEmail(email: string): void {
	if (!email || email === 'foo@barr.com') return;
	const escaped = email.replace(/'/g, "''");
	const sql = `
		BEGIN;
		DELETE FROM accounts
		WHERE id IN (
			SELECT account_id FROM users WHERE email = '${escaped}'
		) AND id NOT IN (
			SELECT account_id FROM users WHERE email = 'foo@barr.com'
		);

		DELETE FROM users
		WHERE email = '${escaped}'
		  AND email != 'foo@barr.com';
		COMMIT;
	`;
	runSql(sql);
}
