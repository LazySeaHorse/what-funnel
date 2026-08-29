<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiRequest } from '$lib/api';
	import { InboxState } from '$lib/store.svelte';
	import { WorkspaceState } from '$lib/workspace.svelte';
	import BrandLogo from '$lib/components/BrandLogo.svelte';
	import ChannelBadge from '$lib/components/ChannelBadge.svelte';
	import LeadStateBadge from '$lib/components/LeadStateBadge.svelte';
	import UserAvatar from '$lib/components/UserAvatar.svelte';
	import LeadsView from '$lib/components/leads/LeadsView.svelte';
	import SettingsView from '$lib/components/settings/SettingsView.svelte';
	import PersonalPreferences from '$lib/components/settings/PersonalPreferences.svelte';
	import ContactsView from '$lib/components/inbox/ContactsView.svelte';
	import SimulatorView from '$lib/components/inbox/SimulatorView.svelte';
	import { formatTime, getChannelLabel, getContactHandle, getContactName, getSnippet, getTagColor, parseMessageContent } from '$lib/inbox/presentation';

	const inbox = new InboxState();
	const workspace = new WorkspaceState();
	let capabilities = $derived(workspace.capabilities);

	// Navigation state
	let selectedNav = $state<'inbox' | 'leads' | 'automation' | 'knowledge' | 'contacts' | 'simulate' | 'settings'>('inbox');
	let leadTab = $state<'lead' | 'details' | 'activity'>('lead');
	let replyTab = $state<'reply' | 'note'>('reply');
	let searchQuery = $state('');
	let aiAutoReplyEnabled = $state(false);
	let aiProviderConfigured = $state(false);
	let aiProviderStatusLoaded = $state(false);
	let messageInput = $state('');
	let appliedAIReplyDraftID = $state<string | null>(null);
	let internalNoteInput = $state('');
	let messageContainer: HTMLDivElement | null = $state(null);

	// Dropdowns and UI popovers
	let showInboxFilterMenu = $state(false);
	let showLeadStateDropdown = $state(false);
	let showWorkspaceDropdown = $state(false);
	let showAssignDropdown = $state(false);
	let showStatusDropdown = $state(false);
	let showAddTagInput = $state(false);
	let newTagText = $state('');

	// Workspace & Lead state
	let accountName = $state('What Funnel Workspace');
	let productMode = $state('full_workspace');
	let leadTrackingEnabled = $state(true);
	let pipelineStates = $state<any[]>([]);
	let notes = $state<any[]>([]);
	let history = $state<any[]>([]);
	let loadingNotes = $state(false);
	let isTyping = $state(false);

	function canOpenNav(tab: typeof selectedNav): boolean {
		if (tab === 'leads') return capabilities.leadTracking;
		if (tab === 'contacts') return capabilities.viewContacts;
		if (tab === 'automation') return capabilities.manageAutomation;
		if (tab === 'knowledge') return capabilities.manageKnowledge;
		if (tab === 'simulate') return capabilities.useSimulator;
		return true;
	}

	$effect(() => {
		const account = workspace.account;
		if (account) {
			accountName = account.name || 'What Funnel Workspace';
			productMode = account.product_mode || 'full_workspace';
			try {
				const parsed = account.settings ? JSON.parse(atob(account.settings)) : {};
				leadTrackingEnabled = productMode !== 'chatbot_only' && parsed.lead_tracking_enabled !== false;
				aiAutoReplyEnabled = parsed.ai_enabled === true;
			} catch (err) {
				console.error('Failed to parse account settings', err);
				leadTrackingEnabled = productMode !== 'chatbot_only';
				aiAutoReplyEnabled = false;
			}
		}
		pipelineStates = workspace.pipeline?.states || [];
		inbox.users = workspace.users;
	});

	$effect(() => {
		if (!canOpenNav(selectedNav)) selectedNav = 'inbox';
		if (!capabilities.leadTracking) {
			replyTab = 'reply';
			inbox.stateFilter = '';
		}
		inbox.configureCapabilities(capabilities);
	});

	// Setup banner — step numbers reflect the 7-step onboarding flow:
	// 1=Business info, 2=Channels, 3=Lead pipeline, 4=Team members,
	// 5=AI Assistant, 6=Knowledge base, 7=Review & finish
	const SKIPPED_STEP_NAMES: Record<string, { label: string; step: number }> = {
		channel_connect: { label: 'Connect a channel', step: 2 },
		add_team:        { label: 'Add team members', step: 4 },
		reply_mode:      { label: 'Configure AI assistant', step: 5 },
		kb_setup:        { label: 'Set up your knowledge base', step: 6 },
		pipeline_setup:  { label: 'Review your pipeline', step: 3 },
		team_invite:     { label: 'Add team members', step: 4 }
	};
	let showSetupBanner = $state(false);
	let bannerSkippedSteps = $state<Array<{ label: string; step: number }>>([]);

	// ─── Leads Tab Enhanced State (Real Data Only) ───────────────────────────
	let leadsFilterTab = $state<string>('all');
	let leadsChannelFilter = $state<string>('all');
	let leadsAssigneeFilter = $state<string>('all');
	let leadsSort = $state<'newest' | 'oldest' | 'name'>('newest');
	let showSortDropdown = $state(false);
	let showFiltersDropdown = $state(false);
	let selectedLeadId = $state<string | null>(null);
	let showLeadDrawer = $state(true);
	let selectedLeadRowIds = $state<string[]>([]);
	let leadsPerPage = $state(10);
	let leadsCurrentPage = $state(1);

	let activeLeadsFilterCount = $derived(
		(leadsChannelFilter !== 'all' ? 1 : 0) + (leadsAssigneeFilter !== 'all' ? 1 : 0)
	);

	function resetLeadsFilters() {
		leadsChannelFilter = 'all';
		leadsAssigneeFilter = 'all';
		showFiltersDropdown = false;
	}

	function getLeadStateInfo(key: string) {
		const matchedState = pipelineStates.find((s) => s.key === key);
		const label = matchedState?.label || (
			key === 'new' ? 'New Lead' :
			key === 'contacted' ? 'Contacted' :
			key === 'follow_up' ? 'Follow-up' :
			key === 'interested' ? 'Interested' :
			key === 'converted' || key === 'closed_won' ? 'Converted' :
			key
		);

		switch (key) {
			case 'new':
				return { label, color: 'amber', dot: 'bg-amber-500', bg: 'bg-amber-50 text-amber-700 border border-amber-200/80' };
			case 'contacted':
				return { label, color: 'blue', dot: 'bg-blue-500', bg: 'bg-blue-50 text-blue-700 border border-blue-200/80' };
			case 'follow_up':
				return { label, color: 'purple', dot: 'bg-purple-500', bg: 'bg-purple-50 text-purple-700 border border-purple-200/80' };
			case 'interested':
				return { label, color: 'green', dot: 'bg-emerald-500', bg: 'bg-emerald-50 text-emerald-700 border border-emerald-200/80' };
			case 'converted':
			case 'closed_won':
				return { label, color: 'emerald', dot: 'bg-teal-500', bg: 'bg-teal-50 text-teal-700 border border-teal-200/80' };
			default:
				return { label, color: 'blue', dot: 'bg-blue-500', bg: 'bg-blue-50 text-blue-700 border border-blue-200/80' };
		}
	}

	let realLeads = $derived.by(() => {
		return inbox.conversations.filter((c) => Boolean(c.lead)).map((c) => {
			const channelType = c.channel?.type || c.channel_type || '';
			let channel: 'instagram' | 'whatsapp' | 'messenger' | 'telegram' | 'webchat' | 'unknown' = 'unknown';
			if (channelType.includes('instagram')) channel = 'instagram';
			else if (channelType.includes('whatsapp')) channel = 'whatsapp';
			else if (channelType.includes('messenger')) channel = 'messenger';
			else if (channelType.includes('telegram')) channel = 'telegram';
			else if (channelType.includes('webchat')) channel = 'webchat';

			const stKey = c.lead.current_state_key || 'unknown';
			const stInfo = getLeadStateInfo(stKey);

			return {
				id: c.id,
				convoId: c.id,
				name: getContactName(c),
				avatar: c.contact?.avatar_url || '',
				avatarBg: 'bg-blue-100 text-blue-700',
				handle: getContactHandle(c) || c.contact?.external_identity || '',
				channel,
				stateKey: stKey,
				stateLabel: stInfo.label,
				stateColor: stInfo.color,
				assignees: (c.assigned_user_ids || []).map((uid: string) => {
					const u = inbox.users.find((usr) => usr.id === uid);
					return {
						id: uid,
						name: u?.name || u?.email?.split('@')[0] || 'User',
						initials: (u?.name || u?.email || 'U').charAt(0).toUpperCase(),
						avatar: u?.avatar_url || '',
						bg: 'bg-blue-600'
					};
				}),
				assigneesExtra: Math.max(0, (c.assigned_user_ids?.length || 0) - 2),
				lastMessage: getSnippet(c) || 'No messages yet',
				updatedAt: formatTime(c.last_message_at || c.created_at) || 'Unknown',
				tags: c.lead.tags || [],
				contactInfo: [
					{
						type: channel === 'whatsapp' ? 'phone' : 'instagram',
						value: getContactHandle(c) || c.contact?.external_identity || 'Not provided',
						label: getChannelLabel(channelType)
					}
				],
				realConvo: c
			};
		});
	});

	let totalLeadsCount = $derived(realLeads.length);
	let countNewLead = $derived(realLeads.filter((l) => l.stateKey === 'new').length);
	let countContacted = $derived(realLeads.filter((l) => l.stateKey === 'contacted').length);
	let countFollowUp = $derived(realLeads.filter((l) => l.stateKey === 'follow_up').length);
	let countInterested = $derived(realLeads.filter((l) => l.stateKey === 'interested').length);
	let countConverted = $derived(realLeads.filter((l) => l.stateKey === 'converted' || l.stateKey === 'closed_won').length);

	let filteredLeads = $derived.by(() => {
		let list = realLeads.filter((l) => {
			if (leadsFilterTab !== 'all') {
				if (leadsFilterTab === 'new' && l.stateKey !== 'new') return false;
				if (leadsFilterTab === 'contacted' && l.stateKey !== 'contacted') return false;
				if (leadsFilterTab === 'follow_up' && l.stateKey !== 'follow_up') return false;
				if (leadsFilterTab === 'interested' && l.stateKey !== 'interested') return false;
				if (leadsFilterTab === 'converted' && l.stateKey !== 'converted' && l.stateKey !== 'closed_won') return false;
			}
			if (leadsChannelFilter !== 'all' && l.channel !== leadsChannelFilter) {
				return false;
			}
			if (leadsAssigneeFilter !== 'all') {
				if (leadsAssigneeFilter === 'unassigned') {
					if (l.assignees && l.assignees.length > 0) return false;
				} else {
					if (!l.assignees || !l.assignees.some((a: any) => a.id === leadsAssigneeFilter)) return false;
				}
			}
			if (searchQuery.trim() !== '') {
				const q = searchQuery.toLowerCase();
				if (!l.name.toLowerCase().includes(q) && !l.lastMessage.toLowerCase().includes(q) && !l.handle.toLowerCase().includes(q)) {
					return false;
				}
			}
			return true;
		});

		if (leadsSort === 'name') {
			list = [...list].sort((a, b) => a.name.localeCompare(b.name));
		}
		return list;
	});

	let activeLead = $derived.by(() => {
		if (selectedLeadId) {
			const found = realLeads.find((l) => l.id === selectedLeadId);
			if (found) return found;
		}
		return realLeads[0] || null;
	});

	async function handleSelectLeadRow(lead: any) {
		selectedLeadId = lead.id;
		selectedLeadRowIds = [lead.id];
		showLeadDrawer = true;

		if (lead.convoId) {
			await inbox.selectConversation(lead.convoId);
			const targetLeadId = inbox.activeConvo?.lead?.id || lead.realConvo?.lead?.id;
			if (targetLeadId) {
				await loadLeadDetails(targetLeadId);
			}
		}
	}

	function handleCloseLeadDrawer() {
		showLeadDrawer = false;
		selectedLeadId = null;
		selectedLeadRowIds = [];
	}

	function toggleLeadRowCheckbox(id: string, e: MouseEvent) {
		e.stopPropagation();
		if (selectedLeadRowIds.includes(id)) {
			selectedLeadRowIds = selectedLeadRowIds.filter((item) => item !== id);
		} else {
			selectedLeadRowIds = [...selectedLeadRowIds, id];
		}
	}

	function toggleAllLeadRows(e: MouseEvent) {
		e.stopPropagation();
		if (selectedLeadRowIds.length === filteredLeads.length) {
			selectedLeadRowIds = [];
		} else {
			selectedLeadRowIds = filteredLeads.map((l) => l.id);
		}
	}

	async function handleDrawerStateChange(newKey: string) {
		const targetLeadId = inbox.activeConvo?.lead?.id || activeLead?.realConvo?.lead?.id;
		if (targetLeadId) {
			try {
				const lead = await apiRequest(`/leads/${targetLeadId}/state`, {
					method: 'PATCH',
					body: { state_key: newKey }
				});
				if (inbox.activeConvo?.lead) {
					inbox.activeConvo.lead.current_state_key = lead.current_state_key;
				}
				if (activeLead?.realConvo?.lead) {
					activeLead.realConvo.lead.current_state_key = lead.current_state_key;
				}
				await inbox.loadConversations();
			} catch (err) {
				console.error(err);
			}
		}
	}

	async function handleDrawerAddTag(tag: string) {
		const lead = inbox.activeConvo?.lead || activeLead?.realConvo?.lead;
		if (lead?.id && tag.trim()) {
			const tagClean = tag.trim();
			const currentTags = lead.tags || [];
			if (!currentTags.includes(tagClean)) {
				const newTags = [...currentTags, tagClean];
				try {
					const updated = await apiRequest(`/leads/${lead.id}/tags`, {
						method: 'PATCH',
						body: { tags: newTags }
					});
					lead.tags = updated.tags;
					if (inbox.activeConvo?.lead) {
						inbox.activeConvo.lead.tags = updated.tags;
					}
					await inbox.loadConversations();
				} catch (err) {
					console.error(err);
				}
			}
		}
	}

	async function handleDrawerRemoveTag(tagToRemove: string) {
		const lead = inbox.activeConvo?.lead || activeLead?.realConvo?.lead;
		if (!lead?.id) return;
		const currentTags = lead.tags || [];
		const newTags = currentTags.filter((t: string) => t !== tagToRemove);
		try {
			const updated = await apiRequest(`/leads/${lead.id}/tags`, {
				method: 'PATCH',
				body: { tags: newTags }
			});
			lead.tags = updated.tags;
			if (inbox.activeConvo?.lead) {
				inbox.activeConvo.lead.tags = updated.tags;
			}
			await inbox.loadConversations();
		} catch (err) {
			console.error(err);
		}
	}

	async function handleDrawerSaveNote(text: string) {
		const leadId = inbox.activeConvo?.lead?.id || activeLead?.realConvo?.lead?.id;
		if (leadId && text.trim()) {
			try {
				await apiRequest(`/leads/${leadId}/notes`, {
					method: 'POST',
					body: { body: text.trim() }
				});
				await loadLeadDetails(leadId);
			} catch (err) {
				console.error(err);
			}
		}
	}

	onMount(() => {
		const handleLeadStateChange = (e: CustomEvent) => {
			if (inbox.activeConvo?.lead && e.detail.lead_id === inbox.activeConvo.lead.id) {
				loadLeadDetails(inbox.activeConvo.lead.id);
			}
		};
		const handleDevMessageSent = async () => {
			await inbox.loadConversations();
			if (inbox.activeConvoID) {
				await inbox.loadMessages(true);
				await tick();
				scrollToBottom();
			}
		};

		window.addEventListener('lead-state-changed', handleLeadStateChange as EventListener);
		window.addEventListener('dev-message-sent', handleDevMessageSent);

		const pollTimer = setInterval(() => {
			if (selectedNav === 'leads' || selectedNav === 'inbox') {
				void inbox.loadConversations();
			}
		}, 1500);

		(async () => {
			try {
				await inbox.init();
				if (!inbox.currentUser) {
					goto('/login');
					return;
				}
				workspace.users = inbox.users;
				await workspace.loadCore(inbox.currentUser);
				inbox.users = workspace.users;
				inbox.configureCapabilities(capabilities);
				if (!capabilities.leadTracking && inbox.filter !== 'all') {
					inbox.filter = 'all';
					await inbox.loadConversations();
				}
				void loadSetupBanner();
				void loadAIProviderStatus();

				if (typeof window !== 'undefined') {
					const urlParams = new URLSearchParams(window.location.search);
					const tabParam = urlParams.get('tab');
					if (tabParam && ['inbox', 'leads', 'automation', 'knowledge', 'contacts', 'simulate', 'settings'].includes(tabParam)) {
						const requestedTab = tabParam as typeof selectedNav;
						selectedNav = canOpenNav(requestedTab) ? requestedTab : 'inbox';
					}
				}

				// Select first conversation if none selected
				if (inbox.conversations.length > 0 && !inbox.activeConvoID) {
					await selectConvo(inbox.conversations[0].id);
				}
				if (capabilities.manageWorkspace) {
					// Warm the Settings-only channel data after the first conversation is
					// ready, so entering Settings normally needs no network round trip.
					void workspace.loadSettings(inbox.currentUser).catch((err) => console.error('Failed to prefetch settings', err));
				}
			} catch (err) {
				goto('/login');
			}
		})();

		return () => {
			clearInterval(pollTimer);
			window.removeEventListener('lead-state-changed', handleLeadStateChange as EventListener);
			window.removeEventListener('dev-message-sent', handleDevMessageSent);
		};
	});

	async function loadSetupBanner() {
		try {
			if (sessionStorage.getItem('setup-banner-dismissed')) return;
			const onboarding = await apiRequest('/onboarding/status');
			if (!onboarding?.completed_at || !onboarding.skipped_steps?.length) return;
			const skipped = (onboarding.skipped_steps as string[])
				.filter((key: string) => key in SKIPPED_STEP_NAMES)
				.map((key: string) => SKIPPED_STEP_NAMES[key]);
			if (skipped.length > 0) {
				bannerSkippedSteps = skipped;
				showSetupBanner = true;
			}
		} catch (_) {
			// The setup banner is optional and should never delay the inbox.
		}
	}

	async function loadAIProviderStatus() {
		try {
			const status = await apiRequest('/workspace/account/ai-config/status');
			aiProviderConfigured = status?.configured === true;
		} catch (_) {
			aiProviderConfigured = false;
		} finally {
			aiProviderStatusLoaded = true;
		}
	}

	// Derived list of displayable messages
	let displayMessages = $derived.by(() => {
		const msgs = inbox.messages;
		const reactionsMap: Record<string, string[]> = {};

		for (const m of msgs) {
			if (m.content_type === 'reaction') {
				try {
					const contentObj = parseMessageContent(m.content);
					const reaction = contentObj.text || contentObj.reaction;
					const targetExtID = contentObj.reply_to_external_id;
					if (targetExtID && reaction) {
						if (!reactionsMap[targetExtID]) {
							reactionsMap[targetExtID] = [];
						}
						reactionsMap[targetExtID].push(reaction);
					}
				} catch (e) {}
			}
		}

		return msgs
			.filter((m: any) => m.content_type !== 'reaction')
			.map((m: any) => {
				const contentObj = parseMessageContent(m.content);
				return {
					...m,
					parsedContent: contentObj,
					reactions: m.external_message_id ? reactionsMap[m.external_message_id] || [] : []
				};
			});
	});

	// Filtered live conversations
	let filteredConversations = $derived.by(() => {
		return inbox.conversations.filter((c) => {
			const contactName = getContactName(c);
			const lastText = getSnippet(c);
			const matchesSearch =
				searchQuery.trim() === '' ||
				contactName.toLowerCase().includes(searchQuery.toLowerCase()) ||
				lastText.toLowerCase().includes(searchQuery.toLowerCase());
			return matchesSearch;
		});
	});

	// Counts
	let countAll = $derived(inbox.conversations.length);
	let countUnassigned = $derived(inbox.conversations.filter((c) => !c.assigned_user_ids || c.assigned_user_ids.length === 0).length);
	let countMine = $derived(
		inbox.conversations.filter((c) => c.assigned_user_ids && inbox.currentUser && c.assigned_user_ids.includes(inbox.currentUser.user_id || inbox.currentUser.id)).length
	);

	const defaultPipelineStages = [
		{ key: 'new', label: 'New Lead' },
		{ key: 'contacted', label: 'Contacted' },
		{ key: 'follow_up', label: 'Follow-up' },
		{ key: 'interested', label: 'Interested' },
		{ key: 'converted', label: 'Converted' }
	];
	let availablePipelineStates = $derived(
		pipelineStates.length > 0 ? pipelineStates : defaultPipelineStages
	);

	async function changeFilter(tab: 'all' | 'unassigned' | 'mine') {
		inbox.filter = tab;
		await inbox.loadConversations();
		if (inbox.conversations.length > 0 && !inbox.conversations.some((c) => c.id === inbox.activeConvoID)) {
			await selectConvo(inbox.conversations[0].id);
		}
	}

	async function changeInboxStateFilter(stateKey: string) {
		inbox.stateFilter = stateKey;
		await inbox.loadConversations();
		if (inbox.conversations.length > 0 && !inbox.conversations.some((c) => c.id === inbox.activeConvoID)) {
			await selectConvo(inbox.conversations[0].id);
		} else if (inbox.conversations.length === 0) {
			inbox.activeConvoID = null;
			inbox.activeConvo = null;
			inbox.messages = [];
		}
		showInboxFilterMenu = false;
	}

	async function selectConvo(id: string) {
		appliedAIReplyDraftID = null;
		await inbox.selectConversation(id);
		await tick();
		scrollToBottom();
	}

	async function handleSendMessage() {
		if (!messageInput.trim() || !inbox.activeConvoID) return;
		const text = messageInput.trim();
		const sent = await inbox.sendMessage(text, appliedAIReplyDraftID ?? undefined);
		if (!sent) return;
		messageInput = '';
		appliedAIReplyDraftID = null;
		await tick();
		scrollToBottom();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSendMessage();
		}
	}

	function scrollToBottom() {
		if (messageContainer) {
			messageContainer.scrollTop = messageContainer.scrollHeight;
		}
	}

	async function loadLeadDetails(leadId: string) {
		loadingNotes = true;
		try {
			notes = await apiRequest(`/leads/${leadId}/notes`);
			history = await apiRequest(`/leads/${leadId}/history`);
		} catch (err) {
			console.error('Failed to load lead details', err);
		} finally {
			loadingNotes = false;
		}
	}

	$effect(() => {
		const lead = inbox.activeConvo?.lead;
		if (capabilities.leadTracking && lead && lead.id) {
			loadLeadDetails(lead.id);
		} else {
			notes = [];
			history = [];
		}
	});

	async function updateLeadState(stateKey: string) {
		if (!inbox.activeConvo?.lead) return;
		try {
			const lead = await apiRequest(`/leads/${inbox.activeConvo.lead.id}/state`, {
				method: 'PATCH',
				body: { state_key: stateKey }
			});
			inbox.activeConvo.lead.current_state_key = lead.current_state_key;
			showLeadStateDropdown = false;
			await inbox.loadConversations();
		} catch (err) {
			console.error(err);
		}
	}

	async function addTag() {
		if (!newTagText.trim() || !inbox.activeConvo?.lead) return;
		const tag = newTagText.trim();
		newTagText = '';
		showAddTagInput = false;

		const currentTags = inbox.activeConvo.lead.tags || [];
		if (currentTags.includes(tag)) return;
		const newTags = [...currentTags, tag];

		try {
			const lead = await apiRequest(`/leads/${inbox.activeConvo.lead.id}/tags`, {
				method: 'PATCH',
				body: { tags: newTags }
			});
			inbox.activeConvo.lead.tags = lead.tags;
		} catch (err) {
			console.error(err);
		}
	}

	async function postInternalNote() {
		if (!internalNoteInput.trim() || !inbox.activeConvo?.lead) return;
		const text = internalNoteInput.trim();
		internalNoteInput = '';

		try {
			await apiRequest(`/leads/${inbox.activeConvo.lead.id}/notes`, {
				method: 'POST',
				body: { body: text }
			});
			await loadLeadDetails(inbox.activeConvo.lead.id);
			replyTab = 'reply';
		} catch (err) {
			console.error(err);
		}
	}

	function toggleUserAssignment(userID: string) {
		if (!inbox.activeConvo) return;
		const current = inbox.activeConvo.assigned_user_ids || [];
		let updated: string[];
		if (current.includes(userID)) {
			updated = current.filter((id: string) => id !== userID);
		} else {
			updated = [...current, userID];
		}
		inbox.assignConversation(updated);
	}

	async function handleLogout() {
		try {
			await apiRequest('/auth/logout', { method: 'POST' });
			goto('/login');
		} catch (err) {
			console.error(err);
		}
	}

	let activeAIReplyDraft = $derived(
		inbox.activeConvoID ? inbox.replyDrafts[inbox.activeConvoID] ?? null : null
	);

	function useAISuggestion() {
		if (!activeAIReplyDraft) return;
		messageInput = activeAIReplyDraft.draft_text;
		appliedAIReplyDraftID = activeAIReplyDraft.id;
	}

	async function dismissAISuggestion() {
		if (!inbox.activeConvoID || !activeAIReplyDraft) return;
		const draftID = activeAIReplyDraft.id;
		try {
			await inbox.dismissReplyDraft(inbox.activeConvoID, draftID);
			if (appliedAIReplyDraftID === draftID) appliedAIReplyDraftID = null;
		} catch (err) {
			console.error('Failed to dismiss AI reply suggestion', err);
		}
	}

	function handleBackToConversations() {
		inbox.activeConvoID = null;
		inbox.activeConvo = null;
		inbox.messages = [];
		appliedAIReplyDraftID = null;
	}

	// ─── Knowledge Tab State ───────────────────────────────────────────────────
	let kbConcepts = $state<any[]>([]);
	let kbPatterns = $state<any[]>([]);
	let kbSuggestions = $state<any[]>([]);
	let kbLastRun = $state<any>(null);
	let kbLoading = $state(false);
	let kbActiveTab = $state<'concepts' | 'patterns' | 'suggestions'>('concepts');
	let kbPasteText = $state('');
	let kbPasting = $state(false);
	let kbPasteResult = $state<{ added?: number; queued?: number; error?: string } | null>(null);
	let kbExpandedConcept = $state<string | null>(null);
	let kbExpandedPattern = $state<string | null>(null);
	let kbMiningTriggerLoading = $state(false);
	let kbMiningTriggerResult = $state<{ messages_scanned?: number; clusters_found?: number; suggestions_created?: number } | null>(null);
	let kbLoaded = $state(false);
	let kbLoadPromise: Promise<void> | null = null;

	async function loadKnowledgeTab(refresh = false) {
		if (kbLoaded && !refresh) return;
		if (kbLoadPromise) return kbLoadPromise;

		// Existing knowledge remains visible during refreshes. The only blocking
		// state is a genuinely uncached first visit.
		kbLoading = !kbLoaded;
		kbLoadPromise = (async () => {
			const [conceptsRes, patternsRes, suggestionsRes, miningRes] = await Promise.allSettled([
				apiRequest('/api/kb/concepts'),
				apiRequest('/api/kb/patterns'),
				apiRequest('/api/kb/suggestions?status_filter=pending'),
				apiRequest('/api/kb/mining-runs/latest')
			]);
			if (conceptsRes.status === 'fulfilled') kbConcepts = conceptsRes.value?.concepts ?? [];
			if (patternsRes.status === 'fulfilled') kbPatterns = patternsRes.value?.patterns ?? [];
			if (suggestionsRes.status === 'fulfilled') {
				kbSuggestions = (suggestionsRes.value?.suggestions ?? []).map((s: any) => {
					const payload = typeof s.proposed_payload === 'string'
						? JSON.parse(s.proposed_payload)
						: (s.proposed_payload ?? {});
					return { ...s, _payload: payload };
				});
			}
			if (miningRes.status === 'fulfilled') kbLastRun = miningRes.value?.last_run ?? null;
			kbLoaded = true;
		})().catch((err) => {
			console.error('Failed to load knowledge tab', err);
		}).finally(() => {
			kbLoading = false;
			kbLoadPromise = null;
		});

		return kbLoadPromise;
	}

	$effect(() => {
		if (selectedNav === 'knowledge') {
			void loadKnowledgeTab();
		} else if (selectedNav === 'leads' || selectedNav === 'inbox') {
			void inbox.loadConversations();
		}
	});

	async function kbDeleteConcept(id: string) {
		if (!confirm('Delete this knowledge concept?')) return;
		try {
			await apiRequest(`/api/kb/concepts/${id}`, { method: 'DELETE' });
			kbConcepts = kbConcepts.filter((c) => c.id !== id);
			if (kbExpandedConcept === id) kbExpandedConcept = null;
		} catch (err) {
			console.error(err);
		}
	}

	async function kbDeletePattern(id: string) {
		if (!confirm('Delete this pattern?')) return;
		try {
			await apiRequest(`/api/kb/patterns/${id}`, { method: 'DELETE' });
			kbPatterns = kbPatterns.filter((p) => p.id !== id);
			if (kbExpandedPattern === id) kbExpandedPattern = null;
		} catch (err) {
			console.error(err);
		}
	}

	async function kbCompilePaste() {
		if (!kbPasteText.trim()) return;
		kbPasting = true;
		kbPasteResult = null;
		try {
			const res = await apiRequest('/api/kb/compile-paste', {
				method: 'POST',
				body: { raw_text: kbPasteText.trim() }
			});
			if (res.added_concepts) {
				kbPasteResult = { added: res.added_concepts.length };
				kbConcepts = [...res.added_concepts, ...kbConcepts];
			} else if (res.suggestion_ids) {
				kbPasteResult = { queued: res.suggestion_ids.length };
			}
			kbPasteText = '';
		} catch (err: any) {
			kbPasteResult = { error: err.message || 'Failed to compile' };
		} finally {
			kbPasting = false;
		}
	}

	async function kbApproveSuggestion(id: string) {
		try {
			await apiRequest(`/api/kb/suggestions/${id}/approve`, {
				method: 'POST',
				body: { reviewed_by: inbox.currentUser?.user_id || inbox.currentUser?.id || '' }
			});
			kbSuggestions = kbSuggestions.filter((s) => s.id !== id);
			await loadKnowledgeTab(true);
		} catch (err) {
			console.error(err);
		}
	}

	async function kbRejectSuggestion(id: string) {
		try {
			await apiRequest(`/api/kb/suggestions/${id}/reject`, {
				method: 'POST',
				body: { reviewed_by: inbox.currentUser?.user_id || inbox.currentUser?.id || '' }
			});
			kbSuggestions = kbSuggestions.filter((s) => s.id !== id);
		} catch (err) {
			console.error(err);
		}
	}

	async function kbTriggerMining() {
		kbMiningTriggerLoading = true;
		kbMiningTriggerResult = null;
		try {
			const res = await apiRequest('/api/kb/mine/trigger', { method: 'POST' });
			kbMiningTriggerResult = res;
			await loadKnowledgeTab(true);
		} catch (err) {
			console.error(err);
		} finally {
			kbMiningTriggerLoading = false;
		}
	}

	function kbFormatDate(iso?: string | null): string {
		if (!iso) return 'Never';
		const d = new Date(iso);
		return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function kbTypeLabel(type?: string): string {
		if (!type) return 'General';
		return type.charAt(0).toUpperCase() + type.slice(1).replace(/_/g, ' ');
	}

	function kbTypeColor(type?: string): string {
		const t = (type || '').toLowerCase();
		if (t === 'faq') return 'bg-blue-50 text-blue-600 border-blue-200/80';
		if (t === 'pricing') return 'bg-emerald-50 text-emerald-600 border-emerald-200/80';
		if (t === 'policy') return 'bg-amber-50 text-amber-600 border-amber-200/80';
		if (t === 'hours') return 'bg-purple-50 text-purple-600 border-purple-200/80';
		if (t === 'service') return 'bg-rose-50 text-rose-600 border-rose-200/80';
		return 'bg-slate-50 text-slate-600 border-slate-200/80';
	}
</script>

<svelte:head>
	<title>What Funnel - Omni Channel Lead Management</title>
</svelte:head>

<div class="flex h-screen w-full bg-slate-50 overflow-hidden text-slate-800 font-sans">
	
	<!-- ================= LEFT MAIN NAVIGATION (Desktop) ================= -->
	<aside class="hidden lg:flex relative w-56 flex-col justify-between p-4 bg-transparent shrink-0 overflow-hidden">
		<!-- Bottom Hero Illustration (anchored to bottom, full width, fading to background at top and bottom) -->
		<div class="absolute bottom-0 left-0 right-0 w-full pointer-events-none select-none z-0 overflow-hidden">
			<img
				src="/images/dashboard-sidebar-hero.webp"
				alt=""
				class="w-full h-auto object-cover object-bottom"
				style="mask-image: linear-gradient(to bottom, transparent 0%, black 18%, black 78%, transparent 100%); -webkit-mask-image: linear-gradient(to bottom, transparent 0%, black 18%, black 78%, transparent 100%);"
			/>
		</div>

		<div class="relative z-10">
			<!-- What Funnel logo -->
			<button type="button" class="px-2 pt-1 pb-6" onclick={() => selectedNav = 'inbox'} aria-label="Go to inbox">
				<BrandLogo size="sm" />
			</button>

			<!-- Nav links -->
			<nav class="space-y-1">
				<!-- Inbox -->
				<button
					onclick={() => selectedNav = 'inbox'}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'inbox' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'inbox' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
					</svg>
					<span>Inbox</span>
				</button>

				{#if capabilities.leadTracking}
				<!-- Leads -->
				<button
					onclick={() => selectedNav = 'leads'}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'leads' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'leads' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
					</svg>
					<span>Leads</span>
				</button>
				{/if}

				{#if capabilities.manageAutomation}
				<!-- Automation -->
				<button
					onclick={() => selectedNav = 'automation'}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'automation' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'automation' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
					<span>Automation</span>
				</button>
				{/if}

				{#if capabilities.manageKnowledge}
				<!-- Knowledge -->
				<button
					onclick={() => selectedNav = 'knowledge'}
					onmouseenter={() => { void loadKnowledgeTab(); }}
					onfocus={() => { void loadKnowledgeTab(); }}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'knowledge' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'knowledge' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
					</svg>
					<span>Knowledge</span>
				</button>
				{/if}

				{#if capabilities.viewContacts}
				<!-- Contacts -->
				<button
					onclick={() => selectedNav = 'contacts'}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'contacts' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'contacts' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
					</svg>
					<span>Contacts</span>
				</button>
				{/if}

				{#if capabilities.useSimulator}
				<!-- Simulate (Dev) -->
				<button
					onclick={() => selectedNav = 'simulate'}
					class="w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'simulate' ? 'bg-purple-50/80 text-purple-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<div class="flex items-center gap-3">
						<svg class="w-5 h-5 {selectedNav === 'simulate' ? 'text-purple-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
						</svg>
						<span>Simulate</span>
					</div>
					<span class="px-1.5 py-0.5 rounded text-[10px] font-medium {selectedNav === 'simulate' ? 'bg-purple-600 text-white' : 'bg-purple-100 text-purple-700'}">DEV</span>
				</button>
				{/if}

				<!-- Settings -->
				<button
					onclick={() => selectedNav = 'settings'}
					class="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-xl font-medium text-sm transition-all duration-150 {selectedNav === 'settings' ? 'bg-blue-50/80 text-blue-600' : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100/60'}"
				>
					<svg class="w-5 h-5 {selectedNav === 'settings' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
					</svg>
					<span>{capabilities.isManager ? 'Settings' : 'Preferences'}</span>
				</button>
			</nav>
		</div>

		<!-- User Workspace Card -->
		<div class="relative z-10 space-y-3">
			<!-- Workspace Switcher Pill -->
			<div class="relative">
				<button
					type="button"
					class="flex w-full items-center justify-between rounded-xl border border-slate-200 bg-white/90 p-2.5 text-left transition hover:bg-slate-50"
				onclick={() => showWorkspaceDropdown = !showWorkspaceDropdown}
					aria-expanded={showWorkspaceDropdown}
					aria-label="Toggle workspace menu"
				>
				<div class="flex items-center gap-3">
					<div class="w-8 h-8 rounded-xl bg-blue-600 text-white font-medium flex items-center justify-center text-sm">
						{accountName.charAt(0).toUpperCase()}
					</div>
					<div>
						<div class="text-xs font-medium text-slate-800 leading-tight truncate max-w-[100px]">{accountName}</div>
						<div class="text-[11px] text-slate-400 leading-tight capitalize">
							{inbox.currentUser?.role === 'manager' ? 'Manager' : inbox.currentUser?.role === 'agent' ? 'Agent' : inbox.currentUser ? inbox.currentUser.role || 'Agent' : 'Loading…'}
						</div>
					</div>
				</div>
				<svg class="w-4 h-4 text-slate-400 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
				</svg>
				</button>

				{#if showWorkspaceDropdown}
					<div class="absolute bottom-full left-0 right-0 mb-2 bg-white rounded-xl border border-slate-200 py-1.5 z-50">
						<div class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase">Current Account</div>
						<div class="px-3 py-1.5 text-xs font-medium text-slate-800">{accountName}</div>
						<div class="border-t border-slate-100 my-1"></div>
						<button
							onclick={handleLogout}
							class="w-full text-left px-3 py-1.5 text-xs font-medium hover:bg-rose-50 text-rose-600 flex items-center gap-1.5"
						>
							<span>Sign out</span>
						</button>
					</div>
				{/if}
			</div>
		</div>
	</aside>

	<!-- ================= MAIN CONTAINER (WHITE CANVAS EXTENDING TO EDGES) ================= -->
	<main class="flex-1 flex flex-col h-full min-h-0 overflow-hidden bg-white lg:border-l border-slate-200/80">
		
		<!-- --- Global Top Bar (Desktop) --- -->
		<header class="hidden lg:flex h-16 px-6 sm:px-8 border-b border-slate-100 items-center justify-between shrink-0">
			<!-- Search Bar -->
			<div class="relative w-80 sm:w-96">
				<span class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
					</svg>
				</span>
				<input
					type="text"
					bind:value={searchQuery}
					placeholder={selectedNav === 'settings' ? 'Search settings...' : selectedNav === 'knowledge' ? 'Search knowledge...' : selectedNav === 'leads' ? 'Search leads...' : selectedNav === 'automation' ? 'Search anything...' : 'Search conversations...'}
					class="w-full h-10 pl-10 pr-10 bg-white text-xs font-medium text-slate-700 placeholder-slate-400 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 cursor-text transition"
				/>
				<span class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
					<kbd class="text-[11px] font-medium text-slate-400 bg-slate-50 px-2 py-0.5 rounded-md border border-slate-200">/</kbd>
				</span>
			</div>

			<!-- Right Controls -->
			<div class="flex items-center gap-3">
				<!-- AI status reflects saved settings and provider configuration. -->
				<div
					class="h-10 flex items-center gap-2.5 px-3.5 bg-white rounded-xl border border-slate-200 text-xs font-medium text-slate-700"
					title={!aiProviderStatusLoaded ? 'Checking AI configuration' : !aiProviderConfigured ? 'Configure an AI provider during onboarding before enabling auto-replies' : undefined}
				>
					<span class="w-2 h-2 rounded-full {aiAutoReplyEnabled && aiProviderConfigured ? 'bg-emerald-500' : 'bg-slate-300'}"></span>
					<span class="text-slate-700">AI Auto-reply</span>
					<span class="px-1.5 py-0.5 rounded text-[10px] font-medium {aiAutoReplyEnabled && aiProviderConfigured ? 'bg-emerald-50 text-emerald-600 border border-emerald-200/60' : 'bg-slate-100 text-slate-500'}">
						{!aiProviderStatusLoaded ? 'CHECKING' : aiAutoReplyEnabled && aiProviderConfigured ? 'ON' : 'OFF'}
					</span>
				</div>

				<!-- User Name & Title Box (Outline only, matching height) -->
				{#if capabilities.showOperatorIdentity}
				<div data-testid="operator-identity" class="h-10 flex items-center gap-2.5 px-3.5 bg-white rounded-xl border border-slate-200 text-left">
					<div class="w-6 h-6 rounded-lg bg-blue-50 text-blue-600 border border-blue-100/80 flex items-center justify-center text-xs font-medium shrink-0">
						{(inbox.currentUser?.username || inbox.currentUser?.name || inbox.currentUser?.email || 'U').charAt(0).toUpperCase()}
					</div>
					<div class="flex flex-col">
						<span class="text-xs font-medium text-slate-800 leading-tight">
							{inbox.currentUser?.username || inbox.currentUser?.name || inbox.currentUser?.email?.split('@')[0] || 'User'}
						</span>
						<span class="text-[10px] text-slate-400 font-medium capitalize leading-tight">
							{inbox.currentUser?.role === 'manager' ? 'Manager' : inbox.currentUser?.role === 'agent' ? 'Agent' : inbox.currentUser?.role || 'Agent'}
						</span>
					</div>
				</div>
				{/if}
			</div>
		</header>

		<!-- --- Main Content View Switcher --- -->
		{#if selectedNav === 'inbox'}
			<!-- ================= 3-COLUMN INBOX DASHBOARD ================= -->
			<div class="flex-1 flex overflow-hidden min-h-0 h-full">
				
				<!-- ================= COLUMN 1: INBOX CONVERSATION LIST ================= -->
				<div class="{inbox.activeConvo ? 'hidden lg:flex' : 'flex'} w-full lg:w-[300px] xl:w-[320px] border-r border-slate-100 flex-col shrink-0 bg-white min-h-0 h-full pb-16 lg:pb-0">
					<!-- Header & Filter Pills -->
					<div class="p-4 pb-3 border-b border-slate-100 space-y-3">
						<div class="flex items-center justify-between">
							<h1 class="text-xl lg:text-lg font-medium text-slate-900 tracking-tight">Inbox</h1>
							{#if capabilities.leadTracking}
							<div class="relative">
								<button
									type="button"
									title="Filter options"
									aria-label="Filter options"
									aria-expanded={showInboxFilterMenu}
									onclick={() => showInboxFilterMenu = !showInboxFilterMenu}
									class="p-1.5 rounded-lg transition cursor-pointer flex items-center gap-1 {inbox.stateFilter ? 'text-blue-600 bg-blue-50 hover:bg-blue-100' : 'text-slate-400 hover:text-slate-600 hover:bg-slate-100'}"
								>
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
									</svg>
									{#if inbox.stateFilter}
										<span class="w-1.5 h-1.5 rounded-full bg-blue-600"></span>
									{/if}
								</button>

								{#if showInboxFilterMenu}
									<div class="absolute right-0 top-full mt-1.5 w-48 bg-white rounded-xl border border-slate-200 shadow-lg py-1.5 z-50 text-xs">
										<div class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase tracking-wider">Filter by Stage</div>
										<button
											type="button"
											onclick={() => changeInboxStateFilter('')}
											class="w-full px-3 py-1.5 text-left hover:bg-slate-50 font-medium flex items-center justify-between {inbox.stateFilter === '' ? 'text-blue-600 font-semibold' : 'text-slate-700'}"
										>
											<span>All stages</span>
											{#if inbox.stateFilter === ''}
												<svg class="w-3.5 h-3.5 text-blue-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
													<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
												</svg>
											{/if}
										</button>
										{#each availablePipelineStates as st}
											{@const isSelected = inbox.stateFilter === st.key}
											{@const stInfo = getLeadStateInfo(st.key)}
											<button
												type="button"
												onclick={() => changeInboxStateFilter(st.key)}
												class="w-full px-3 py-1.5 text-left hover:bg-slate-50 font-medium flex items-center justify-between {isSelected ? 'text-blue-600 font-semibold' : 'text-slate-700'}"
											>
												<div class="flex items-center gap-2">
													<span class="w-2 h-2 rounded-full {stInfo.dot}"></span>
													<span>{st.label || stInfo.label}</span>
												</div>
												{#if isSelected}
													<svg class="w-3.5 h-3.5 text-blue-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
														<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
													</svg>
												{/if}
											</button>
										{/each}
										{#if inbox.stateFilter}
											<div class="border-t border-slate-100 my-1"></div>
											<button
												type="button"
												onclick={() => changeInboxStateFilter('')}
												class="w-full px-3 py-1.5 text-left hover:bg-slate-50 text-slate-500 hover:text-slate-700 text-[11px]"
											>
												Clear stage filter
											</button>
										{/if}
									</div>
								{/if}
							</div>
							{/if}
						</div>

						<!-- Search bar (matching mobile mock) -->
						<div class="relative w-full">
							<span class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-slate-400">
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
								</svg>
							</span>
							<input
								type="text"
								bind:value={searchQuery}
								placeholder="Search conversations"
								class="w-full h-9 pl-9 pr-8 bg-slate-50 text-xs font-medium text-slate-700 placeholder-slate-400 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 transition"
							/>
						</div>

						{#if capabilities.leadTracking}
						<!-- Tabs: All / Unassigned / Mine -->
						<div class="flex items-center gap-1.5 text-xs overflow-x-auto pb-0.5">
							<button
								onclick={() => changeFilter('all')}
								class="tab-btn px-3 py-1 rounded-full font-medium flex items-center gap-1.5 transition cursor-pointer shrink-0 {inbox.filter === 'all' ? 'bg-blue-50 text-blue-600 border border-blue-200/60 active' : 'text-slate-500 hover:bg-slate-100'}"
							>
								<span>All</span>
								<span class="bg-blue-600 text-white text-[10px] font-medium px-1.5 py-0.2 rounded-full">{countAll}</span>
							</button>

							<button
								onclick={() => changeFilter('unassigned')}
								class="tab-btn px-2.5 py-1 rounded-full font-medium flex items-center gap-1.5 transition cursor-pointer shrink-0 {inbox.filter === 'unassigned' ? 'bg-blue-50 text-blue-600 border border-blue-200/60 active' : 'text-slate-500 hover:bg-slate-100'}"
							>
								<span>Unassigned</span>
								<span class="text-slate-400 text-[11px]">{countUnassigned}</span>
							</button>

							<button
								onclick={() => changeFilter('mine')}
								class="tab-btn px-2.5 py-1 rounded-full font-medium flex items-center gap-1.5 transition cursor-pointer shrink-0 {inbox.filter === 'mine' ? 'bg-blue-50 text-blue-600 border border-blue-200/60 active' : 'text-slate-500 hover:bg-slate-100'}"
							>
								<span>Mine</span>
								<span class="text-slate-400 text-[11px]">{countMine}</span>
							</button>
						</div>
						{/if}

						{#if capabilities.leadTracking && inbox.stateFilter}
							<div class="flex items-center gap-1.5 text-[11px] text-blue-700 bg-blue-50/80 px-2.5 py-1 rounded-lg border border-blue-200/60">
								<span class="text-slate-500">Stage:</span>
								<span class="font-medium">{getLeadStateInfo(inbox.stateFilter).label}</span>
								<button
									type="button"
									onclick={() => changeInboxStateFilter('')}
									class="ml-auto text-slate-400 hover:text-slate-700 p-0.5 cursor-pointer"
									title="Clear stage filter"
								>
									<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
									</svg>
								</button>
							</div>
						{/if}
					</div>

					<!-- Conversation List Items -->
					<div class="conversation-list flex-1 overflow-y-auto divide-y divide-slate-50">
						{#if filteredConversations.length === 0}
							<div class="p-8 text-center space-y-2">
								<div class="w-10 h-10 mx-auto rounded-full bg-blue-50 text-blue-600 flex items-center justify-center">
									<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
									</svg>
								</div>
								<div class="text-xs font-medium text-slate-800">No conversations yet</div>
								<p class="text-[11px] text-slate-400">Incoming messages from connected channels will show up here in real time.</p>
							</div>
						{:else}
							{#each filteredConversations as item (item.id)}
									{@const isSelected = (inbox.pendingConvoID || inbox.activeConvoID) === item.id}
								{@const contactName = getContactName(item)}
								{@const snippet = getSnippet(item)}
								{@const timeStr = formatTime(item.last_message_at || item.created_at)}
								<div
									role="button"
									tabindex="0"
									onclick={() => selectConvo(item.id)}
									onkeydown={(e) => { if (e.key === 'Enter') selectConvo(item.id); }}
									class="convo-item relative px-4 py-3.5 flex items-start gap-3 cursor-pointer transition text-left {isSelected ? 'bg-blue-50/50' : 'hover:bg-slate-50/80'}"
								>
									<!-- Blue indicator bar -->
									{#if isSelected}
										<div class="absolute left-0 top-0 bottom-0 w-1 bg-blue-600 rounded-r"></div>
									{/if}
									{#if !isSelected && item.unread}
										<div class="absolute left-1.5 top-6 w-1.5 h-1.5 rounded-full bg-blue-500"></div>
									{/if}

									<!-- Avatar + Channel Badge -->
									<div class="relative shrink-0 mt-0.5">
										<UserAvatar
											name={contactName}
											avatar={item.contact?.avatar_url}
											size="lg"
											channel={item.channel?.type || item.channel_type}
										/>
									</div>

									<!-- Details -->
									<div class="flex-1 min-w-0">
										<div class="flex items-center justify-between mb-0.5">
											<span class="font-medium text-xs text-slate-800 truncate">{contactName}</span>
											<span class="text-[10px] text-slate-400 shrink-0 font-medium tabular-nums">{timeStr}</span>
										</div>
										<p class="text-xs text-slate-500 truncate leading-snug">{snippet}</p>
										
										<!-- Badges -->
										<div class="flex items-center justify-between mt-1.5">
											<div>
										{#if capabilities.leadTracking && item.lead?.current_state_key}
													<LeadStateBadge stateKey={item.lead.current_state_key} size="xs" />
												{/if}
											</div>
											
											{#if item.unread}
												<span class="w-2 h-2 rounded-full bg-blue-600"></span>
											{/if}
										</div>
									</div>
								</div>
							{/each}
						{/if}
					</div>
				</div>

				<!-- ================= COLUMN 2: ACTIVE CHAT AREA ================= -->
				<div class="{inbox.activeConvo ? 'flex' : 'hidden lg:flex'} flex-1 flex-col bg-slate-50 border-r border-slate-100 min-h-0 overflow-hidden w-full">
					{#if !inbox.activeConvo}
						<div class="flex-1 flex flex-col items-center justify-center p-8 text-center space-y-3">
							<div class="w-12 h-12 rounded-2xl bg-blue-50 text-blue-600 flex items-center justify-center">
								<svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
								</svg>
							</div>
							<h3 class="text-sm font-medium text-slate-800">Select a conversation</h3>
							<p class="text-xs text-slate-400 max-w-xs">Pick a chat from the inbox on the left to view messages and respond.</p>
						</div>
					{:else}
						<!-- Chat Top Header (with back button on mobile matching mock) -->
						<div class="h-16 px-4 sm:px-6 bg-white border-b border-slate-100 flex items-center justify-between shrink-0">
							<div class="flex items-center gap-2.5 sm:gap-3 min-w-0">
								<button
									type="button"
									class="lg:hidden p-1.5 -ml-1 text-slate-500 hover:text-slate-900 rounded-lg active:bg-slate-100 transition cursor-pointer shrink-0"
									onclick={handleBackToConversations}
									aria-label="Back to conversations"
								>
									<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
										<path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
									</svg>
								</button>

								<UserAvatar
									name={getContactName(inbox.activeConvo)}
									avatar={inbox.activeConvo.contact?.avatar_url}
									size="lg"
									channel={inbox.activeConvo.channel?.type || inbox.activeConvo.channel_type}
								/>
								<div class="min-w-0">
									<h2 class="font-medium text-sm text-slate-900 leading-tight whitespace-nowrap truncate">{getContactName(inbox.activeConvo)}</h2>
									<p class="text-xs text-slate-400 leading-tight whitespace-nowrap truncate">{getContactHandle(inbox.activeConvo) || getChannelLabel(inbox.activeConvo.channel_type || inbox.activeConvo.channel?.type)}</p>
								</div>
							</div>

							<div class="flex items-center gap-2">
								{#if capabilities.manageAssignments}
								<!-- Assign button -->
								<div class="relative">
									<button
										title="Assign conversation"
										onclick={() => showAssignDropdown = !showAssignDropdown}
										class="w-8 h-8 rounded-lg border border-slate-200/80 flex items-center justify-center text-slate-500 hover:text-slate-800 hover:bg-slate-50 transition"
									>
										<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
											<path stroke-linecap="round" stroke-linejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
										</svg>
									</button>

									{#if showAssignDropdown}
										<div class="absolute right-0 top-full mt-1.5 w-48 bg-white rounded-xl border border-slate-200 py-1.5 z-50 text-xs">
											<div class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase">Assign Team Member</div>
											{#each inbox.users as u}
												{@const isAssigned = inbox.activeConvo.assigned_user_ids?.includes(u.id)}
												<button
													onclick={() => toggleUserAssignment(u.id)}
													class="w-full text-left px-3 py-1.5 hover:bg-slate-50 flex items-center justify-between text-slate-700 font-medium"
												>
													<span>{u.email.split('@')[0]}</span>
													{#if isAssigned}
														<span class="text-blue-600 font-medium">✓</span>
													{/if}
												</button>
											{/each}
										</div>
									{/if}
								</div>
								{/if}

								<!-- Status badge / action dropdown -->
								<div class="relative">
									<button
										type="button"
										onclick={() => showStatusDropdown = !showStatusDropdown}
										class="flex items-center gap-1.5 px-3 py-1.5 rounded-full {inbox.activeConvo.status === 'closed' ? 'bg-slate-100 text-slate-600 border-slate-200' : 'bg-blue-50 text-blue-600 border-blue-100'} text-xs font-medium border transition hover:opacity-90 cursor-pointer"
										title="Change conversation status"
									>
										<span class="w-1.5 h-1.5 rounded-full {inbox.activeConvo.status === 'closed' ? 'bg-slate-400' : 'bg-emerald-500'}"></span>
										<span>{inbox.activeConvo.status === 'closed' ? 'Closed' : 'In Conversation'}</span>
										<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
											<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
										</svg>
									</button>

									{#if showStatusDropdown}
										<div class="absolute right-0 top-full mt-1.5 w-48 bg-white rounded-xl border border-slate-200 py-1.5 z-50 text-xs shadow-lg">
											<div class="px-3 py-1 text-[11px] font-medium text-slate-400 uppercase">Conversation Status</div>
											{#if inbox.activeConvo.status !== 'closed'}
												<button
													type="button"
													onclick={async () => {
														showStatusDropdown = false;
														await inbox.closeConversation(inbox.activeConvo.id);
													}}
													class="w-full text-left px-3 py-2 hover:bg-slate-50 flex items-center gap-2 text-slate-700 font-medium cursor-pointer"
												>
													<svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
														<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
													</svg>
													<span>Close Conversation</span>
												</button>
											{:else}
												<div class="px-3 py-2 text-slate-500 flex items-center gap-2">
													<svg class="w-4 h-4 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
														<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
													</svg>
													<span>Conversation is closed</span>
												</div>
											{/if}
										</div>
									{/if}
								</div>
							</div>
						</div>

						<!-- Message Stream -->
						<div bind:this={messageContainer} class="flex-1 p-4 sm:p-6 overflow-y-auto space-y-4">
							<!-- Date Separator -->
							<div class="flex justify-center my-2">
								<span class="text-[11px] font-medium text-slate-400 bg-slate-100/60 px-3 py-0.5 rounded-full">Messages</span>
							</div>

							{#if displayMessages.length === 0}
								<div class="text-center py-12 text-slate-400 text-xs">
									No messages yet in this conversation. Send a reply below.
								</div>
							{:else}
								{#each displayMessages as msg (msg.id)}
									{@const isCustomer = msg.sender_type === 'contact' || msg.sender_type === 'customer' || msg.direction === 'inbound'}
									{@const textContent = msg.parsedContent.text || msg.parsedContent.caption || JSON.stringify(msg.parsedContent)}
									{@const timeStr = formatTime(msg.created_at)}

									{#if isCustomer}
										<!-- Customer Incoming Message -->
										<div class="message-row flex flex-col items-start max-w-md">
											<div class="msg-text bg-white p-3.5 rounded-2xl rounded-tl-sm border border-slate-200/70 text-xs text-slate-800 leading-relaxed whitespace-pre-wrap">
												{textContent}
											</div>
											<span class="text-[10px] text-slate-400 mt-1 ml-1 font-medium">{timeStr}</span>
										</div>
									{:else}
										<!-- Agent / AI Outgoing Message -->
										<div class="message-row outbound flex flex-col items-end ml-auto max-w-md">
											<div class="msg-text bg-blue-600 text-white p-3.5 rounded-2xl rounded-tr-sm text-xs leading-relaxed whitespace-pre-wrap">
												{textContent}
											</div>
											<div class="flex items-center gap-1 text-[10px] text-slate-400 mt-1 mr-1">
												<span>{timeStr}</span>
												<svg class="w-3.5 h-3.5 text-blue-500 inline-block" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
													<path d="M18 6L7 17l-5-5" />
													<path d="M22 10l-7.5 7.5-1.5-1.5" />
												</svg>
											</div>
										</div>
									{/if}
								{/each}
							{/if}
						</div>

						<!-- --- Chat Composer & AI Suggestion (matching mock) --- -->
						<div class="p-3 sm:p-4 bg-white border-t border-slate-100 shrink-0">
							{#if capabilities.useReplyDrafts && activeAIReplyDraft && appliedAIReplyDraftID !== activeAIReplyDraft.id && replyTab === 'reply'}
								<!-- AI Suggested Response Box (floating above composer matching mock) -->
								<div class="mb-3 p-3 rounded-xl bg-blue-50/60 border border-blue-100 flex flex-col gap-1.5 relative">
									<div class="flex items-center justify-between">
										<div class="flex items-center gap-1.5 text-xs font-medium text-blue-700">
											<svg class="w-3.5 h-3.5 text-blue-600" viewBox="0 0 24 24" fill="currentColor">
												<path d="M12 2L14.4 8.6L21 11L14.4 13.4L12 20L9.6 13.4L3 11L9.6 8.6L12 2Z" />
											</svg>
											<span>AI reply suggestion</span>
									</div>
									<button
										type="button"
										onclick={dismissAISuggestion}
										class="text-slate-400 hover:text-slate-600 p-0.5 text-xs"
										aria-label="Dismiss suggestion"
									>
										✕
									</button>
								</div>
								<div class="text-xs text-slate-700 leading-snug">
									{activeAIReplyDraft.draft_text}
								</div>
									<div class="flex justify-end pt-1">
										<button
											type="button"
											onclick={useAISuggestion}
											class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-lg transition shadow-2xs cursor-pointer"
										>
											Use this
										</button>
									</div>
								</div>
							{/if}

							<div class="bg-white rounded-2xl border border-slate-200/90 overflow-hidden">
								{#if capabilities.leadTracking}
								<!-- Tabs: Reply | Internal Note -->
								<div class="flex items-center gap-6 px-4 pt-2.5 border-b border-slate-100 text-xs font-medium">
									<button
										onclick={() => replyTab = 'reply'}
										class="pb-2 transition {replyTab === 'reply' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-slate-400 hover:text-slate-600'}"
									>
										Reply
									</button>
									<button
										onclick={() => replyTab = 'note'}
										class="pb-2 transition {replyTab === 'note' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-slate-400 hover:text-slate-600'}"
									>
										Internal Note
									</button>
								</div>
								{/if}

								{#if replyTab === 'reply'}
									<!-- Text Input Area -->
									<div class="px-4 py-2.5">
										<input
											type="text"
											bind:value={messageInput}
											onkeydown={handleKeydown}
											placeholder="Type a message..."
											class="compose-input w-full text-xs sm:text-sm text-slate-800 placeholder-slate-400 focus:outline-none bg-transparent"
										/>
									</div>

									<!-- Bottom Toolbar & Send Button (matching mock) -->
									<div class="flex items-center justify-between px-3 py-2 border-t border-slate-100/60 bg-slate-50/40">
										<!-- Left icon tools -->
										<div class="flex items-center gap-1 text-slate-400">
											<button title="Add attachment" class="p-1.5 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition">
												<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
													<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
												</svg>
											</button>
											<button title="Emoji picker" class="p-1.5 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition">
												<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
													<path stroke-linecap="round" stroke-linejoin="round" d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
												</svg>
											</button>
											<button title="Saved replies" class="p-1.5 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition">
												<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
													<path stroke-linecap="round" stroke-linejoin="round" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
												</svg>
											</button>
											<button title="Quick automation" class="p-1.5 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition">
												<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
													<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
												</svg>
											</button>
										</div>

										<!-- Right Action: Circular Send button -->
										<button
											onclick={handleSendMessage}
											disabled={!messageInput.trim() || !inbox.activeConvoID}
											class="send-btn w-8 h-8 bg-blue-600 hover:bg-blue-700 active:bg-blue-800 text-white rounded-full flex items-center justify-center transition disabled:cursor-not-allowed disabled:opacity-40 shadow-xs cursor-pointer"
											aria-label="Send message"
										>
											<svg class="w-4 h-4 rotate-45 -mt-0.5 ml-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
												<path stroke-linecap="round" stroke-linejoin="round" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
											</svg>
										</button>
									</div>
								{:else}
									<!-- Internal Note Tab Content -->
									<div class="p-3">
										<textarea
											bind:value={internalNoteInput}
											placeholder="Add an internal note visible only to your team..."
											class="w-full h-20 p-2.5 text-xs text-slate-800 placeholder-slate-400 bg-amber-50/40 rounded-xl border border-amber-200/60 focus:outline-none"
										></textarea>
										<div class="flex justify-end mt-2">
											<button
												onclick={postInternalNote}
												class="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-medium rounded-lg transition"
											>
												Post Internal Note
											</button>
										</div>
									</div>
								{/if}

							</div>
						</div>
					{/if}
				</div>

				{#if capabilities.showConversationSidePanel}
				<!-- ================= COLUMN 3: RIGHT DETAILS & AI SUMMARY ================= -->
				<div class="lead-panel hidden lg:flex w-[300px] xl:w-[320px] bg-white flex-col shrink-0 overflow-y-auto min-h-0 h-full">
					<!-- Header Tabs: Lead / Details / Activity -->
					<div class="flex items-center justify-around border-b border-slate-100 text-xs font-medium text-slate-400 pt-3 px-2">
						<button
							onclick={() => leadTab = 'lead'}
							class="pb-2.5 px-4 transition {leadTab === 'lead' ? 'text-blue-600 border-b-2 border-blue-600' : 'hover:text-slate-700'}"
						>
							Lead
						</button>
						<button
							onclick={() => leadTab = 'details'}
							class="pb-2.5 px-4 transition {leadTab === 'details' ? 'text-blue-600 border-b-2 border-blue-600' : 'hover:text-slate-700'}"
						>
							Details
						</button>
						<button
							onclick={() => leadTab = 'activity'}
							class="pb-2.5 px-4 transition {leadTab === 'activity' ? 'text-blue-600 border-b-2 border-blue-600' : 'hover:text-slate-700'}"
						>
							Activity
						</button>
					</div>

					<div class="p-4 space-y-5 flex-1">
						{#if !inbox.activeConvo}
							<div class="p-6 text-center text-xs text-slate-400 space-y-2">
								<p>Select a conversation to view {leadTab} details.</p>
								<p class="text-[11px] text-slate-400">To simulate customer inquiries, click <button onclick={() => selectedNav = 'simulate'} class="text-purple-600 font-medium underline">Simulate</button> in the left sidebar.</p>
							</div>
						{:else if leadTab === 'lead'}
							<!-- Lead State -->
							<div class="space-y-1.5 relative">
								<span class="text-xs font-medium text-slate-500">Lead State</span>
								<button
									type="button"
									onclick={() => showLeadStateDropdown = !showLeadStateDropdown}
									aria-label="Change lead state"
									class="lead-state-badge w-full flex items-center justify-between p-2.5 bg-amber-50/60 rounded-xl border border-amber-200/80 cursor-pointer hover:bg-amber-50 transition text-left"
								>
									<div class="flex items-center gap-2">
										<span class="w-2 h-2 rounded-full bg-amber-500"></span>
										<span class="text-xs font-medium text-amber-700">{inbox.activeConvo.lead?.current_state_key || 'New Lead'}</span>
									</div>
									<svg class="w-4 h-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
									</svg>
								</button>

								{#if showLeadStateDropdown}
									<div class="absolute top-full left-0 right-0 mt-1 bg-white rounded-xl border border-slate-200 py-1 z-50">
										{#each (pipelineStates.length > 0 ? pipelineStates : [{ key: 'new', label: 'New Lead' }, { key: 'interested', label: 'Interested' }, { key: 'follow_up', label: 'Follow-up' }, { key: 'quoted', label: 'Quoted' }, { key: 'closed_won', label: 'Closed Won' }]) as stateOption}
											<button
												onclick={() => updateLeadState(stateOption.key || stateOption.label)}
												aria-label={`Set lead state to ${stateOption.label || stateOption.key}`}
												class="w-full text-left px-3 py-1.5 text-xs font-medium hover:bg-slate-50 text-slate-700 flex items-center gap-2"
											>
												<span class="w-2 h-2 rounded-full bg-blue-500"></span>
												<span>{stateOption.label || stateOption.key}</span>
											</button>
										{/each}
									</div>
								{/if}
							</div>

							{#if capabilities.manageAssignments}
							<!-- Assigned to -->
							<div class="space-y-1.5">
								<span class="text-xs font-medium text-slate-500">Assigned to</span>
								<div class="flex items-center gap-2">
									{#if !inbox.activeConvo.assigned_user_ids || inbox.activeConvo.assigned_user_ids.length === 0}
										<span class="text-xs text-slate-400">Unassigned</span>
									{:else}
										{#each inbox.activeConvo.assigned_user_ids as uid}
											{@const usr = inbox.users.find(u => u.id === uid)}
											<div class="w-7 h-7 rounded-full bg-blue-600 text-white flex items-center justify-center text-[10px] font-medium ring-2 ring-white">
												{usr?.email ? usr.email.charAt(0).toUpperCase() : 'U'}
											</div>
										{/each}
									{/if}
									<button
										onclick={() => showAssignDropdown = !showAssignDropdown}
										title="Add assignee"
										class="w-7 h-7 rounded-full border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 hover:border-slate-400 flex items-center justify-center text-xs transition"
									>
										+
									</button>
								</div>
							</div>
							{/if}

							<!-- Tags -->
							<div class="space-y-1.5">
								<span class="text-xs font-medium text-slate-500">Tags</span>
								<div class="flex flex-wrap items-center gap-1.5">
									{#if inbox.activeConvo.lead?.tags && inbox.activeConvo.lead.tags.length > 0}
										{#each inbox.activeConvo.lead.tags as tag}
											<span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-purple-50 text-purple-600 text-xs font-medium border border-purple-100">
												<span>{tag}</span>
											</span>
										{/each}
									{:else}
										<span class="text-xs text-slate-400">No tags</span>
									{/if}

									{#if showAddTagInput}
										<div class="flex items-center gap-1">
											<input
												type="text"
												bind:value={newTagText}
												aria-label="Tag name"
												onkeydown={(e) => { if (e.key === 'Enter') addTag(); }}
												placeholder="Tag..."
												class="w-20 px-2 py-0.5 text-xs border border-purple-200 rounded-lg focus:outline-none"
											/>
											<button onclick={addTag} aria-label="Save tag" class="text-xs font-medium text-blue-600 px-1">✓</button>
										</div>
									{:else}
										<button
											onclick={() => showAddTagInput = true}
											title="Add tag"
											class="w-6 h-6 rounded-lg border border-dashed border-slate-300 text-slate-400 hover:text-slate-600 flex items-center justify-center text-xs transition"
										>
											+
										</button>
									{/if}
								</div>
							</div>

							<!-- Notes Card -->
							<div class="space-y-1.5">
								<span class="text-xs font-medium text-slate-500">Notes</span>
								{#if loadingNotes}
									<div class="p-3 bg-slate-50 rounded-xl text-xs text-slate-400">Loading notes...</div>
								{:else if notes.length === 0}
									<div class="p-3 bg-slate-50 rounded-xl text-xs text-slate-400">No notes yet. Use internal note to add one.</div>
								{:else}
									<div class="space-y-2">
										{#each notes as note}
											<div class="note-item p-3 rounded-2xl bg-white border border-slate-200/80 text-xs text-slate-600 leading-relaxed">
												{note.body}
												<div class="text-[10px] text-slate-400 mt-1">{formatTime(note.created_at)}</div>
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<!-- AI Assist (Beta) Section -->
							<div class="space-y-2 pt-1">
								<div class="flex items-center gap-1.5 text-xs font-medium text-slate-700">
									<svg class="w-3.5 h-3.5 text-purple-600" viewBox="0 0 24 24" fill="currentColor">
										<path d="M12 2L14.4 8.6L21 11L14.4 13.4L12 20L9.6 13.4L3 11L9.6 8.6L12 2Z" />
									</svg>
									<span>AI Assist</span>
									<span class="text-[10px] text-slate-400 font-normal">(Beta)</span>
								</div>

								<div class="space-y-2">
									<button class="w-full py-2 px-3 rounded-xl border border-blue-200 text-blue-600 text-xs font-medium hover:bg-blue-50/50 transition">
										Summarize conversation
									</button>
								</div>
							</div>

						{:else if leadTab === 'details'}
							<!-- Contact Details Tab -->
							<div class="space-y-4 text-xs">
								<div class="p-3.5 bg-slate-50 rounded-xl space-y-2.5">
									<div class="flex justify-between py-1 border-b border-slate-200/60">
										<span class="text-slate-400">Display Name</span>
										<span class="font-medium text-slate-800">{getContactName(inbox.activeConvo)}</span>
									</div>
									<div class="flex justify-between py-1 border-b border-slate-200/60">
										<span class="text-slate-400">Channel</span>
										<span class="font-medium text-slate-800">{getChannelLabel(inbox.activeConvo.channel_type || inbox.activeConvo.channel?.type)}</span>
									</div>
									<div class="flex justify-between py-1 border-b border-slate-200/60">
										<span class="text-slate-400">Identity</span>
										<span class="font-medium text-slate-800">{inbox.activeConvo.contact?.external_identity || 'N/A'}</span>
									</div>
									<div class="flex justify-between py-1 border-b border-slate-200/60">
										<span class="text-slate-400">Status</span>
										<span class="font-medium text-slate-800 capitalize">{inbox.activeConvo.status}</span>
									</div>
									<div class="flex justify-between py-1">
										<span class="text-slate-400">Created</span>
										<span class="font-medium text-slate-800">{formatTime(inbox.activeConvo.created_at)}</span>
									</div>
								</div>
							</div>

						{:else if leadTab === 'activity'}
							<!-- Activity Timeline Tab -->
							<div class="space-y-3 text-xs">
								{#if history.length === 0}
									<div class="text-slate-400 text-xs p-4 text-center">No state history recorded yet.</div>
								{:else}
									<div class="border-l-2 border-blue-200 ml-2 pl-3 space-y-4">
										{#each history as item}
											<div class="history-item relative">
												<span class="absolute -left-[19px] top-1 w-2.5 h-2.5 rounded-full bg-blue-600 ring-2 ring-white"></span>
												<div class="font-medium text-slate-800">Stage changed to {item.to_state}</div>
												<div class="text-[11px] text-slate-400">{formatTime(item.created_at)}</div>
											</div>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</div>
				</div>
				{/if}

			</div>

		{:else if selectedNav === 'leads'}
			<!-- ================= LEADS TAB DASHBOARD VIEW ================= -->
			<div class="flex-1 flex flex-col min-h-0 h-full overflow-hidden bg-white">
				
				<!-- Top Header Row: Title & Actions -->
				<div class="px-6 pt-5 pb-3 flex items-center justify-between shrink-0 bg-white">
					<div class="flex items-center gap-4">
						<h1 class="text-2xl font-medium text-slate-900 tracking-tight">Leads</h1>
					</div>

					<div class="flex items-center gap-2.5 relative">
						<!-- Filters Dropdown -->
						<div class="relative">
							<button
								type="button"
								onclick={() => showFiltersDropdown = !showFiltersDropdown}
								aria-expanded={showFiltersDropdown}
								aria-label="Filter leads"
								class="flex items-center gap-2 px-3.5 py-1.5 rounded-xl border text-xs font-medium transition cursor-pointer shadow-[0_1px_2px_rgba(0,0,0,0.02)] {activeLeadsFilterCount > 0 ? 'bg-blue-50 border-blue-200/90 text-blue-700 hover:bg-blue-100/70' : 'bg-white hover:bg-slate-50 border-slate-200/90 text-slate-700'}"
							>
								<svg class="w-3.5 h-3.5 {activeLeadsFilterCount > 0 ? 'text-blue-600' : 'text-slate-500'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
								</svg>
								<span>Filters</span>
								{#if activeLeadsFilterCount > 0}
									<span class="w-4 h-4 rounded-full bg-blue-600 text-white text-[10px] flex items-center justify-center font-semibold">{activeLeadsFilterCount}</span>
								{/if}
							</button>

							{#if showFiltersDropdown}
								<div class="absolute right-0 top-full mt-1.5 w-64 bg-white rounded-xl border border-slate-200 shadow-xl p-3 z-50 text-xs space-y-3">
									<!-- Channel Filter -->
									<div>
										<label for="leads-channel-filter" class="block text-[11px] font-medium text-slate-400 uppercase tracking-wider mb-1.5">Channel</label>
										<select
											id="leads-channel-filter"
											bind:value={leadsChannelFilter}
											class="w-full h-8 px-2.5 bg-slate-50 border border-slate-200 rounded-lg text-xs text-slate-700 focus:outline-none focus:border-blue-400"
										>
											<option value="all">All Channels</option>
											<option value="whatsapp">WhatsApp</option>
											<option value="instagram">Instagram</option>
											<option value="messenger">Messenger</option>
											<option value="telegram">Telegram</option>
											<option value="webchat">Webchat</option>
										</select>
									</div>

									{#if capabilities.manageAssignments}
									<!-- Assignee Filter -->
									<div>
										<label for="leads-assignee-filter" class="block text-[11px] font-medium text-slate-400 uppercase tracking-wider mb-1.5">Assignee</label>
										<select
											id="leads-assignee-filter"
											bind:value={leadsAssigneeFilter}
											class="w-full h-8 px-2.5 bg-slate-50 border border-slate-200 rounded-lg text-xs text-slate-700 focus:outline-none focus:border-blue-400"
										>
											<option value="all">All Assignees</option>
											<option value="unassigned">Unassigned</option>
											{#each inbox.users as user}
												<option value={user.id}>{user.name || user.email || 'User'}</option>
											{/each}
										</select>
									</div>
									{/if}

									<!-- Footer / Reset -->
									<div class="flex items-center justify-between pt-2 border-t border-slate-100">
										<button
											type="button"
											onclick={resetLeadsFilters}
											disabled={activeLeadsFilterCount === 0}
											class="text-[11px] font-medium text-slate-500 hover:text-slate-800 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
										>
											Reset filters
										</button>
										<button
											type="button"
											onclick={() => showFiltersDropdown = false}
											class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-xs font-medium cursor-pointer"
										>
											Close
										</button>
									</div>
								</div>
							{/if}
						</div>

						<!-- Sort Dropdown -->
						<div class="relative">
							<button
								onclick={() => showSortDropdown = !showSortDropdown}
								class="flex items-center gap-1.5 px-3.5 py-1.5 bg-white hover:bg-slate-50 rounded-xl border border-slate-200/90 text-xs font-medium text-slate-700 transition cursor-pointer shadow-[0_1px_2px_rgba(0,0,0,0.02)]"
							>
								<span>Sort: {leadsSort === 'newest' ? 'Newest' : leadsSort === 'oldest' ? 'Oldest' : 'Name'}</span>
								<svg class="w-3.5 h-3.5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
								</svg>
							</button>

							{#if showSortDropdown}
								<div class="absolute right-0 top-full mt-1.5 w-36 bg-white rounded-xl border border-slate-200 shadow-lg py-1 z-50 text-xs">
									<button
										onclick={() => { leadsSort = 'newest'; showSortDropdown = false; }}
										class="w-full px-3 py-1.5 text-left hover:bg-slate-50 font-medium {leadsSort === 'newest' ? 'text-blue-600' : 'text-slate-700'}"
									>
										Newest
									</button>
									<button
										onclick={() => { leadsSort = 'oldest'; showSortDropdown = false; }}
										class="w-full px-3 py-1.5 text-left hover:bg-slate-50 font-medium {leadsSort === 'oldest' ? 'text-blue-600' : 'text-slate-700'}"
									>
										Oldest
									</button>
									<button
										onclick={() => { leadsSort = 'name'; showSortDropdown = false; }}
										class="w-full px-3 py-1.5 text-left hover:bg-slate-50 font-medium {leadsSort === 'name' ? 'text-blue-600' : 'text-slate-700'}"
									>
										Name (A-Z)
									</button>
								</div>
							{/if}
						</div>
					</div>
				</div>

				<!-- Reusable Modular Leads View Component (docked full-height) -->
				<LeadsView
					leads={filteredLeads}
					counts={{
						all: totalLeadsCount,
						new: countNewLead,
						contacted: countContacted,
						follow_up: countFollowUp,
						interested: countInterested,
						converted: countConverted
					}}
					activeFilter={leadsFilterTab}
					selectedLeadId={selectedLeadId}
					selectedRowIds={selectedLeadRowIds}
					showDrawer={showLeadDrawer}
					activeLead={activeLead}
					pipelineStates={pipelineStates}
					users={inbox.users}
					notes={notes}
					assignedUserIds={inbox.activeConvo?.assigned_user_ids || []}
					canManageAssignments={capabilities.manageAssignments}
					onSelectFilter={(key) => leadsFilterTab = key}
					onSelectLead={handleSelectLeadRow}
					onToggleCheckbox={toggleLeadRowCheckbox}
					onToggleAllCheckboxes={toggleAllLeadRows}
					onCloseDrawer={handleCloseLeadDrawer}
					onOpenChat={(convoId) => { selectConvo(convoId); selectedNav = 'inbox'; }}
					onChangeState={handleDrawerStateChange}
					onToggleAssignee={toggleUserAssignment}
					onAddTag={handleDrawerAddTag}
					onRemoveTag={handleDrawerRemoveTag}
					onSaveNote={handleDrawerSaveNote}
				/>
			</div>

		{:else if selectedNav === 'automation'}
			<!-- ================= AUTOMATION VIEW ================= -->
			<div class="flex-1 flex flex-col overflow-y-auto p-6 space-y-6">
				<div class="flex items-center justify-between">
					<div>
						<h1 class="text-xl font-medium text-slate-900 tracking-tight">Automation & AI Workflows</h1>
						<p class="text-xs text-slate-500">Set up instant auto-replies, lead qualification triggers, and smart routing</p>
					</div>
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div class="p-4 rounded-2xl border border-slate-200/80 bg-slate-50/40 space-y-3">
						<div class="flex items-center justify-between">
							<div class="flex items-center gap-2">
								<span class="w-8 h-8 rounded-xl bg-blue-50 text-blue-600 flex items-center justify-center font-medium">⚡</span>
								<div>
									<h3 class="text-xs font-medium text-slate-900">Instant AI Auto-reply</h3>
									<p class="text-[11px] text-slate-400">Replies automatically to incoming customer questions</p>
								</div>
							</div>
							<span class="px-2 py-0.5 rounded-full text-[10px] font-medium {aiAutoReplyEnabled && aiProviderConfigured ? 'bg-emerald-50 text-emerald-600' : 'bg-slate-100 text-slate-500'}">
								{!aiProviderStatusLoaded ? 'CHECKING' : aiAutoReplyEnabled && aiProviderConfigured ? 'ACTIVE' : !aiProviderConfigured ? 'NOT CONFIGURED' : 'OFF'}
							</span>
						</div>
						<p class="text-xs text-slate-600">
							{aiAutoReplyEnabled && aiProviderConfigured
								? 'Uses your knowledge base to provide instant answers.'
								: !aiProviderConfigured
									? 'Connect an AI provider during onboarding before auto-replies can run.'
									: 'Auto-replies are currently disabled in workspace settings.'}
						</p>
					</div>
				</div>
			</div>

		{:else if selectedNav === 'knowledge'}
			<!-- ================= KNOWLEDGE BASE VIEW ================= -->
			<div class="flex-1 flex flex-col overflow-hidden">

				<!-- Header -->
				<div class="px-6 pt-6 pb-4 border-b border-slate-100 shrink-0">
					<div class="flex items-start justify-between">
						<div>
							<h1 class="text-xl font-medium text-slate-900 tracking-tight">Knowledge Base</h1>
							<p class="text-xs text-slate-500 mt-0.5">Train What Funnel AI on your pricing, FAQs, services, and policies</p>
						</div>
						<!-- Mining Run Status Card -->
						<div class="flex items-center gap-3">
							<div class="text-right">
								<div class="text-[11px] font-medium text-slate-400 uppercase tracking-wide">Last AI Audit</div>
								<div class="text-xs font-medium text-slate-700 mt-0.5">{kbFormatDate(kbLastRun?.run_at)}</div>
								{#if kbLastRun}
									<div class="text-[11px] text-slate-400">
										{kbLastRun.messages_scanned} scanned · {kbLastRun.clusters_found} clusters · {kbLastRun.suggestions_created} suggestions
									</div>
								{/if}
							</div>
							<button
								onclick={kbTriggerMining}
								disabled={kbMiningTriggerLoading}
								class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-slate-50 border border-slate-200 text-xs font-medium text-slate-700 hover:bg-slate-100 transition disabled:opacity-50"
							>
								{#if kbMiningTriggerLoading}
									<span class="w-3 h-3 border-2 border-slate-400 border-t-transparent rounded-full animate-spin"></span>
									<span>Scanning…</span>
								{:else}
									<svg class="w-3.5 h-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
									</svg>
									<span>Run Audit Now</span>
								{/if}
							</button>
						</div>
					</div>

					{#if kbMiningTriggerResult}
						<div class="mt-3 px-3 py-2 bg-blue-50 border border-blue-100 rounded-xl text-xs text-blue-700">
							Audit complete — {kbMiningTriggerResult.messages_scanned} messages scanned, {kbMiningTriggerResult.clusters_found} clusters found, {kbMiningTriggerResult.suggestions_created} suggestions created.
						</div>
					{/if}

					<!-- Sub-tabs -->
					<div class="flex gap-1 mt-4">
						{#each [{ k: 'concepts', l: 'KB Concepts', count: kbConcepts.length }, { k: 'patterns', l: 'Patterns', count: kbPatterns.length }, { k: 'suggestions', l: 'AI Suggestions', count: kbSuggestions.length }] as tab}
							<button
								onclick={() => kbActiveTab = tab.k as any}
								class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all {kbActiveTab === tab.k ? 'bg-blue-50 text-blue-600' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-50'}"
							>
								{tab.l}
								<span class="px-1.5 py-0.5 rounded-md text-[10px] font-medium {kbActiveTab === tab.k ? 'bg-blue-100 text-blue-600' : 'bg-slate-100 text-slate-500'}">{tab.count}</span>
							</button>
						{/each}
					</div>
				</div>

				<!-- Loading state -->
				{#if kbLoading}
					<div class="flex-1 flex items-center justify-center">
						<span class="w-5 h-5 border-2 border-blue-400 border-t-transparent rounded-full animate-spin"></span>
					</div>

				<!-- ── CONCEPTS TAB ── -->
				{:else if kbActiveTab === 'concepts'}
					<div class="flex-1 overflow-y-auto min-h-0 flex flex-col">
						<!-- Add by paste -->
						<div class="px-6 py-4 border-b border-slate-100 shrink-0">
							<div class="text-xs font-medium text-slate-700 mb-2">Add business knowledge</div>
							<textarea
								bind:value={kbPasteText}
								placeholder="Paste anything — pricing, policies, FAQs, hours, services… The AI will extract and structure it automatically."
								class="w-full h-20 p-3 text-xs text-slate-700 placeholder-slate-400 bg-slate-50 rounded-xl border border-slate-200 focus:outline-none focus:border-blue-400 resize-none leading-relaxed"
							></textarea>
							<div class="flex items-center justify-between mt-2">
								<div>
									{#if kbPasteResult?.added !== undefined}
										<span class="text-xs text-emerald-600 font-medium">✓ {kbPasteResult.added} concept{kbPasteResult.added !== 1 ? 's' : ''} added to KB</span>
									{:else if kbPasteResult?.queued !== undefined}
										<span class="text-xs text-amber-600 font-medium">⏳ {kbPasteResult.queued} concepts queued for review (AI Suggestions tab)</span>
									{:else if kbPasteResult?.error}
										<span class="text-xs text-rose-600 font-medium">✕ {kbPasteResult.error}</span>
									{/if}
								</div>
								<button
									onclick={kbCompilePaste}
									disabled={kbPasting || !kbPasteText.trim()}
									class="flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium transition disabled:opacity-50"
								>
									{#if kbPasting}
										<span class="w-3 h-3 border-2 border-white/40 border-t-white rounded-full animate-spin"></span>
										<span>Compiling…</span>
									{:else}
										<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
											<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
										</svg>
										<span>Compile with AI</span>
									{/if}
								</button>
							</div>
						</div>

						<!-- Concept list -->
						<div class="flex-1 overflow-y-auto px-6 py-4 space-y-2">
							{#if kbConcepts.length === 0}
								<div class="flex flex-col items-center justify-center py-16 text-center">
									<div class="w-10 h-10 rounded-2xl bg-slate-50 border border-slate-200 flex items-center justify-center mb-3">
										<svg class="w-5 h-5 text-slate-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
											<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
										</svg>
									</div>
									<div class="text-sm font-medium text-slate-600">No knowledge concepts yet</div>
									<div class="text-xs text-slate-400 mt-1">Paste your business info above and click "Compile with AI"</div>
								</div>
							{:else}
								{#each kbConcepts as concept (concept.id)}
									<div class="border border-slate-200/80 rounded-xl overflow-hidden">
										<button
											onclick={() => kbExpandedConcept = kbExpandedConcept === concept.id ? null : concept.id}
											class="w-full px-4 py-3 flex items-center justify-between text-left hover:bg-slate-50/60 transition"
										>
											<div class="flex items-center gap-2.5 min-w-0">
												<span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {kbTypeColor(concept.type)}">
													{kbTypeLabel(concept.type)}
												</span>
												<span class="text-xs font-medium text-slate-800 truncate">{concept.title}</span>
												{#if concept.source === 'owner_pasted'}
													<span class="text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">pasted</span>
												{/if}
											</div>
											<div class="flex items-center gap-2 shrink-0">
												<svg class="w-3.5 h-3.5 text-slate-400 transition-transform {kbExpandedConcept === concept.id ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
													<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
												</svg>
											</div>
										</button>

										{#if kbExpandedConcept === concept.id}
											<div class="px-4 pb-4 pt-1 border-t border-slate-100 bg-slate-50/40 text-xs space-y-2">
												<div class="text-slate-700 leading-relaxed whitespace-pre-wrap">{concept.body_markdown}</div>
												<div class="flex items-center justify-between pt-2 text-[11px] text-slate-400 border-t border-slate-100">
													<span>Added {kbFormatDate(concept.created_at)}</span>
													<button
														onclick={() => kbDeleteConcept(concept.id)}
														class="text-rose-500 hover:text-rose-700 transition"
													>Delete</button>
												</div>
											</div>
										{/if}
									</div>
								{/each}
							{/if}
						</div>
					</div>

				<!-- ── PATTERNS TAB ── -->
				{:else if kbActiveTab === 'patterns'}
					<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
						{#if kbPatterns.length === 0}
							<div class="flex flex-col items-center justify-center py-16 text-center">
								<div class="w-10 h-10 rounded-2xl bg-slate-50 border border-slate-200 flex items-center justify-center mb-3">
									<svg class="w-5 h-5 text-slate-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
										<path stroke-linecap="round" stroke-linejoin="round" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
									</svg>
								</div>
								<div class="text-sm font-medium text-slate-600">No conversation patterns mined yet</div>
								<div class="text-xs text-slate-400 mt-1">Run an AI audit above to scan customer messages for common question patterns</div>
							</div>
						{:else}
							{#each kbPatterns as pattern (pattern.id)}
								<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2">
									<div class="flex items-center justify-between">
										<span class="text-xs font-medium text-slate-800">{pattern.canonical_question}</span>
									</div>
									{#if pattern.trigger_phrases?.length}
										<div class="text-[11px] text-slate-400">Triggers: {pattern.trigger_phrases.join(', ')}</div>
									{/if}
									<div class="text-xs text-slate-600 leading-relaxed whitespace-pre-wrap">{pattern.answer_markdown}</div>
									<div class="flex justify-end pt-1 text-[11px]">
										<button onclick={() => kbDeletePattern(pattern.id)} class="text-rose-500 hover:text-rose-700 transition">Delete</button>
									</div>
								</div>
							{/each}
						{/if}
					</div>

				<!-- ── SUGGESTIONS TAB ── -->
				{:else if kbActiveTab === 'suggestions'}
					<div class="flex-1 overflow-y-auto px-6 py-4 space-y-3">
						{#if kbSuggestions.length === 0}
							<div class="flex flex-col items-center justify-center py-16 text-center">
								<div class="w-10 h-10 rounded-2xl bg-slate-50 border border-slate-200 flex items-center justify-center mb-3">
									<svg class="w-5 h-5 text-slate-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
										<path stroke-linecap="round" stroke-linejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
									</svg>
								</div>
								<div class="text-sm font-medium text-slate-600">No suggestions pending review</div>
								<div class="text-xs text-slate-400 mt-1">When AI audits find knowledge gaps in conversations, recommendations will appear here</div>
							</div>
						{:else}
							{#each kbSuggestions as sugg (sugg.id)}
								<div class="p-4 rounded-xl border border-slate-200/80 bg-white space-y-2.5">
									<div class="flex items-center justify-between">
										<div class="flex items-center gap-2">
											<span class="px-2 py-0.5 rounded text-[10px] font-medium border capitalize {kbTypeColor(sugg._payload?.type ?? sugg.type)}">
												{kbTypeLabel(sugg._payload?.type ?? sugg.type)}
											</span>
											<span class="text-xs font-medium text-slate-800">{sugg._payload?.title ?? sugg._payload?.canonical_question ?? '—'}</span>
										</div>
										<span class="text-[10px] text-slate-400">conf {((sugg.confidence ?? 0) * 100).toFixed(0)}%</span>
									</div>
									<div class="text-xs text-slate-600 bg-slate-50 p-2.5 rounded-lg leading-relaxed whitespace-pre-wrap">{sugg._payload?.body_markdown ?? sugg._payload?.answer_markdown ?? ''}</div>
									<div class="flex items-center justify-between pt-1 text-xs">
										<span class="text-[11px] text-slate-400">{sugg.type?.replace(/_/g, ' ')}</span>
										<div class="flex items-center gap-2">
											<button
												onclick={() => kbRejectSuggestion(sugg.id)}
												class="px-2.5 py-1 text-slate-500 hover:text-slate-700 hover:bg-slate-100 rounded-lg transition"
											>Dismiss</button>
											<button
												onclick={() => kbApproveSuggestion(sugg.id)}
												class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg transition"
											>Add to Knowledge Base</button>
										</div>
									</div>
								</div>
							{/each}
						{/if}
					</div>
				{/if}
			</div>

		{:else if selectedNav === 'contacts'}
			<ContactsView conversations={inbox.conversations} onOpenConversation={(id) => { void selectConvo(id); selectedNav = 'inbox'; }} />

		{:else if selectedNav === 'simulate'}
			<SimulatorView onBack={() => selectedNav = 'inbox'} />

		{:else if selectedNav === 'settings'}
			<!-- ================= SETTINGS VIEW ================= -->
			<div class="flex-1 flex flex-col min-h-0 overflow-y-auto pb-16 lg:pb-0">
				{#if capabilities.manageWorkspace}
					<SettingsView {inbox} {workspace} initialSection="general" onNavigate={(tab) => selectedNav = tab as any} />
				{:else}
					<PersonalPreferences {inbox} {workspace} />
				{/if}
			</div>
		{/if}

	</main>

	<!-- ================= MOBILE BOTTOM NAVIGATION (Matches mock) ================= -->
	{#if !inbox.activeConvo || selectedNav !== 'inbox'}
		<nav class="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-white/95 backdrop-blur-md border-t border-slate-200/90 py-1.5 px-2 flex items-center justify-around">
			<!-- 1. Inbox -->
			<a
				href="#inbox"
				role="button"
				onclick={(e) => { e.preventDefault(); selectedNav = 'inbox'; handleBackToConversations(); }}
				class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selectedNav === 'inbox' ? 'text-blue-600 font-medium' : 'text-slate-500 hover:text-slate-800'}"
			>
				<div class="relative">
					<svg class="w-5 h-5 {selectedNav === 'inbox' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
					</svg>
					{#if capabilities.leadTracking && countUnassigned > 0}
						<span class="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-blue-600"></span>
					{/if}
				</div>
				<span class="text-[11px] mt-0.5">Inbox</span>
			</a>

			{#if capabilities.leadTracking}
			<!-- 2. Leads -->
			<a
				href="#leads"
				role="button"
				onclick={(e) => { e.preventDefault(); selectedNav = 'leads'; }}
				class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selectedNav === 'leads' ? 'text-blue-600 font-medium' : 'text-slate-500 hover:text-slate-800'}"
			>
				<svg class="w-5 h-5 {selectedNav === 'leads' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
				</svg>
				<span class="text-[11px] mt-0.5">Leads</span>
			</a>
			{/if}

			{#if capabilities.manageAutomation}
			<!-- 3. Automate -->
			<a
				href="#automate"
				role="button"
				onclick={(e) => { e.preventDefault(); selectedNav = 'automation'; }}
				class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selectedNav === 'automation' ? 'text-blue-600 font-medium' : 'text-slate-500 hover:text-slate-800'}"
			>
				<svg class="w-5 h-5 {selectedNav === 'automation' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
				<span class="text-[11px] mt-0.5">Automate</span>
			</a>
			{/if}

			{#if capabilities.manageKnowledge}
			<!-- 4. Knowledge -->
			<a
				href="#knowledge"
				role="button"
				onclick={(e) => { e.preventDefault(); selectedNav = 'knowledge'; void loadKnowledgeTab(); }}
				class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selectedNav === 'knowledge' ? 'text-blue-600 font-medium' : 'text-slate-500 hover:text-slate-800'}"
			>
				<svg class="w-5 h-5 {selectedNav === 'knowledge' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
				</svg>
				<span class="text-[11px] mt-0.5">Knowledge</span>
			</a>
			{/if}

			<!-- 5. Settings -->
			<a
				href="#settings"
				role="button"
				onclick={(e) => { e.preventDefault(); selectedNav = 'settings'; }}
				class="flex flex-col items-center justify-center py-1 px-3 rounded-xl transition cursor-pointer {selectedNav === 'settings' ? 'text-blue-600 font-medium' : 'text-slate-500 hover:text-slate-800'}"
			>
				<svg class="w-5 h-5 {selectedNav === 'settings' ? 'text-blue-600' : 'text-slate-400'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
				<span class="text-[11px] mt-0.5">{capabilities.isManager ? 'Settings' : 'Preferences'}</span>
			</a>
		</nav>
	{/if}

</div>
