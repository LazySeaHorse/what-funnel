import { cleanupAllTestAccounts } from './db-cleanup';

export default async function globalSetup() {
	cleanupAllTestAccounts();
}
