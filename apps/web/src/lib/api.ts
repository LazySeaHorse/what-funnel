export interface ApiRequestOptions extends Omit<RequestInit, 'body'> {
	body?: any;
}

export async function apiRequest(path: string, options: ApiRequestOptions = {}) {
	options.credentials = 'include';
	
	const fetchOptions = { ...options } as RequestInit;
	
	if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData)) {
		fetchOptions.body = JSON.stringify(options.body);
		fetchOptions.headers = {
			'Content-Type': 'application/json',
			...options.headers
		};
	} else if (options.body) {
		fetchOptions.body = options.body;
	}
	
	const res = await fetch(path, fetchOptions);
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Request failed with status ${res.status}`);
	}
	
	if (res.status === 204) return null;
	return res.json().catch(() => null);
}
