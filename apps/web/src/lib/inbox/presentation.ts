export function parseMessageContent(content: unknown): Record<string, any> {
	if (!content) return {};
	if (typeof content === 'object') return content as Record<string, any>;
	if (typeof content !== 'string') return {};
	try { return JSON.parse(content); } catch {
		try { return JSON.parse(atob(content)); } catch { return { text: content }; }
	}
}

export function formatTime(timeStr?: string): string {
	if (!timeStr) return '';
	return new Date(timeStr).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function getChannelLabel(type?: string): string {
	if (!type) return 'Direct';
	for (const [needle, label] of [['whatsapp', 'WhatsApp'], ['instagram', 'Instagram'], ['messenger', 'Messenger'], ['telegram', 'Telegram'], ['webchat', 'Webchat']] as const) {
		if (type.includes(needle)) return label;
	}
	return type;
}

export function getSnippet(convo: any): string {
	const msg = convo?.last_message_preview || convo?.last_message;
	if (!msg) return 'No messages yet';
	const parsed = parseMessageContent(msg.content);
	return parsed.text || parsed.caption || 'Message';
}

export function getContactName(convo?: any): string {
	if (!convo) return 'Select a conversation';
	return String(convo.contact_name || convo.contact?.display_name || convo.display_name || convo.contact_display_name || convo.contact?.external_identity || 'Contact')
		.replace(/\s*\((Instagram|WhatsApp|Messenger|Telegram|Webchat|Direct|matrix_[a-z]+)\)$/i, '').trim();
}

export function getContactHandle(convo?: any): string {
	if (!convo) return '';
	const type = convo.channel?.type || convo.channel_type;
	let id = convo.contact?.external_identity || convo.external_identity || '';
	if (id && !id.startsWith('@') && type?.includes('instagram')) id = `@${id}`;
	const channel = getChannelLabel(type);
	return id ? `${channel} • ${id}` : channel;
}

export function getTagColor(stateKey?: string): string {
	const key = stateKey?.toLowerCase() || '';
	if (!key) return 'bg-amber-50 text-amber-600';
	if (key.includes('new')) return 'bg-amber-50 text-amber-600 border border-amber-200/80';
	if (key.includes('interest') || key.includes('won')) return 'bg-emerald-50 text-emerald-600 border border-emerald-200/80';
	if (key.includes('follow')) return 'bg-purple-50 text-purple-600 border border-purple-200/80';
	if (key.includes('quote')) return 'bg-rose-50 text-rose-600 border border-rose-200/80';
	return 'bg-blue-50 text-blue-600 border border-blue-200/80';
}
