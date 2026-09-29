import { cleanupAllTestAccounts, clearDbUsageMarker, dbWasUsed } from './db-cleanup';

export default async function globalTeardown() {
	// Only clean up when this run actually created accounts through the real API.
	if (dbWasUsed()) {
		cleanupAllTestAccounts();
	}
	clearDbUsageMarker();
}
