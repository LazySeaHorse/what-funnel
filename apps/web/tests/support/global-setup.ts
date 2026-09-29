import { clearDbUsageMarker, e2eRunId } from './db-cleanup';

export default async function globalSetup() {
	// Establish the run id (inherited by workers through the environment) and
	// reset the "run touched the DB" marker. No database access happens here:
	// mock-only runs must not connect to any database.
	e2eRunId();
	clearDbUsageMarker();
}
