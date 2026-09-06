import type { SimulatorPlatform, TestContact } from './types';

export function buildNativePayload(platform: SimulatorPlatform, contact: TestContact, text: string) {
	const numericID = Number.parseInt(contact.externalID.replace(/\D/g, ''), 10) || Math.floor(100000 + Math.random() * 900000);
	const messageID = Math.floor(1000 + Math.random() * 90000);
	const nowUnix = Math.floor(Date.now() / 1000);

	if (platform === 'telegram') {
		const [firstName = 'User', ...lastNameParts] = contact.name.trim().split(' ');
		const lastName = lastNameParts.join(' ');
		return {
			update_id: Math.floor(10000000 + Math.random() * 90000000),
			message: {
				message_id: messageID,
				from: { id: numericID, is_bot: false, first_name: firstName, last_name: lastName, username: contact.name.toLowerCase().replace(/\s+/g, '_') },
				chat: { id: numericID, type: 'private', first_name: firstName, last_name: lastName, username: contact.name.toLowerCase().replace(/\s+/g, '_') },
				date: nowUnix,
				text
			}
		};
	}

	if (platform === 'whatsapp') {
		return {
			object: 'whatsapp_business_account',
			entry: [{
				id: 'biz_account_01',
				changes: [{
					value: {
						messaging_product: 'whatsapp',
						metadata: { display_phone_number: '15550000000', phone_number_id: 'phone_001' },
						contacts: [{ profile: { name: contact.name }, wa_id: contact.externalID }],
						messages: [{ from: contact.externalID, id: `wamid.HBgL${Date.now()}`, timestamp: `${nowUnix}`, text: { body: text }, type: 'text' }]
					},
					field: 'messages'
				}]
			}]
		};
	}

	return {
		object: platform === 'instagram' ? 'instagram' : 'page',
		entry: [{
			id: `page_${platform}_01`,
			time: Date.now(),
			messaging: [{
				sender: { id: contact.externalID },
				recipient: { id: `page_${platform}_01` },
				timestamp: Date.now(),
				message: { mid: `mid.${platform}.${Date.now()}`, text }
			}]
		}]
	};
}
