import { cleanupAllTestAccounts } from './db-cleanup';

export default async function globalTeardown() {
	cleanupAllTestAccounts();
}
