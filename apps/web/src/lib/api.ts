export interface ApiRequestOptions extends Omit<RequestInit, 'body'> {
	body?: any;
}

function getCookie(name: string): string | null {
	if (typeof document === 'undefined') return null;
	const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/([\.$?*|{}\(\)\[\]\\\/\+^])/g, '\\$1') + '=([^;]*)'));
	return match ? decodeURIComponent(match[1]) : null;
}

export async function apiRequest(path: string, options: ApiRequestOptions = {}) {
	options.credentials = 'include';
	
	const csrfToken = getCookie('csrf_token');
	const defaultHeaders: Record<string, string> = {
		'X-Requested-With': 'XMLHttpRequest'
	};
	if (csrfToken) {
		defaultHeaders['X-CSRF-Token'] = csrfToken;
	}

	const fetchOptions = { ...options } as RequestInit;
	
	if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData)) {
		fetchOptions.body = JSON.stringify(options.body);
		fetchOptions.headers = {
			'Content-Type': 'application/json',
			...defaultHeaders,
			...options.headers
		};
	} else if (options.body) {
		fetchOptions.body = options.body;
		fetchOptions.headers = {
			...defaultHeaders,
			...options.headers
		};
	} else {
		fetchOptions.headers = {
			...defaultHeaders,
			...options.headers
		};
	}
	
	let targetPath = path;
	if (targetPath.startsWith('/') && !targetPath.startsWith('/api-gateway')) {
		targetPath = `/api-gateway${targetPath}`;
	}
	
	const res = await fetch(targetPath, fetchOptions);
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || errData.detail || `Request failed with status ${res.status}`);
	}
	
	if (res.status === 204) return null;
	return res.json().catch(() => null);
}
