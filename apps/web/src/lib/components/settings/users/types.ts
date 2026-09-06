export interface WorkspaceUser {
	id: string;
	username?: string;
	name?: string;
	email?: string;
	role: string;
}

export interface UserCredentials {
	username: string;
	plaintextPassword?: string;
	role: string;
}

export function generatePassword() {
	const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%';
	return Array.from({ length: 12 }, () => chars.charAt(Math.floor(Math.random() * chars.length))).join('');
}
