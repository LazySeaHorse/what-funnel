import type { PresetCategory, SimulatorPlatform, TestContact } from './types';

export const SIMULATOR_STORAGE_KEY = 'wf_dev_test_contacts';

export const DEFAULT_CONTACTS: TestContact[] = [
	{ id: 'c1', name: 'Alice Test', externalID: 'test-alice-001', avatar: 'A', platform: 'whatsapp' },
	{ id: 'c2', name: 'Bob Demo', externalID: 'test-bob-002', avatar: 'B', platform: 'instagram' },
	{ id: 'c3', name: 'Charlie Mock', externalID: 'test-charlie-003', avatar: 'C', platform: 'messenger' },
	{ id: 'c4', name: 'Dana Telegram', externalID: 'test-dana-004', avatar: 'D', platform: 'telegram' }
];

export const SIMULATOR_PLATFORMS: Array<{ key: SimulatorPlatform; label: string }> = [
	{ key: 'whatsapp', label: 'WhatsApp' },
	{ key: 'instagram', label: 'Instagram' },
	{ key: 'messenger', label: 'Messenger' },
	{ key: 'telegram', label: 'Telegram' }
];

export const PRESET_CATEGORIES: Record<SimulatorPlatform, PresetCategory[]> = {
	whatsapp: [
		{
			label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
			stageTag: 'L1 Fast-Path',
			badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
			prompts: ['Hi! Do you have any weekend slots available?', 'Where are you located?', 'What is your cancellation policy?']
		},
		{
			label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
			stageTag: 'L3 KB RAG',
			badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
			prompts: ['Can I see the pricing for the premium package?', 'Do you offer home concierge service?']
		}
	],
	instagram: [
		{
			label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
			stageTag: 'L1 Fast-Path',
			badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
			prompts: ["What's the price for hair coloring?", 'Where are you located?']
		},
		{
			label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
			stageTag: 'L3 KB RAG',
			badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
			prompts: ['Saw your post! Are you accepting new clients?', 'Do you offer home concierge service?']
		}
	],
	messenger: [
		{
			label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
			stageTag: 'L1 Fast-Path',
			badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
			prompts: ['What hours are you open on Sunday?', 'Where are you located?']
		},
		{
			label: '🙋 Level 4 — Human Handoff / Complex Inquiry',
			stageTag: 'L4 Escalation',
			badgeColor: 'bg-amber-50 text-amber-700 border-amber-200',
			prompts: ['Hello! Is a deposit required?', 'Can I book for a bridal party of 5?']
		}
	],
	telegram: [
		{
			label: '⚡ Level 1 — Fast-Path Trigger (Deterministic)',
			stageTag: 'L1 Fast-Path',
			badgeColor: 'bg-emerald-50 text-emerald-700 border-emerald-200',
			prompts: ['Hi! Interested in your services', 'Where are you located?']
		},
		{
			label: '🧠 Level 3 — Knowledge Base RAG (Grounded LLM)',
			stageTag: 'L3 KB RAG',
			badgeColor: 'bg-blue-50 text-blue-700 border-blue-200',
			prompts: ['Can I get a custom quote?', 'Is customer support available?']
		}
	]
};

