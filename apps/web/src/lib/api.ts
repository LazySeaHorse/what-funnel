export async function apiRequest(path: string, options: RequestInit = {}) {
	options.credentials = 'include';
	
	if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData)) {
		options.body = JSON.stringify(options.body);
		options.headers = {
			'Content-Type': 'application/json',
			...options.headers
		};
	}
	
	const res = await fetch(path, options);
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Request failed with status ${res.status}`);
	}
	
	if (res.status === 204) return null;
	return res.json().catch(() => null);
}
