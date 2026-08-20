<script lang="ts">
	import { formatTime, getChannelLabel, getContactName, getTagColor } from '$lib/inbox/presentation';
	let { conversations, onOpenConversation }: { conversations: any[]; onOpenConversation: (id: string) => void } = $props();
</script>

<div class="flex-1 flex flex-col overflow-hidden p-6 space-y-4">
	<div><h1 class="text-xl font-medium text-slate-900 tracking-tight">Contacts Directory</h1><p class="text-xs text-slate-500">Omni-channel client directory</p></div>
	<div class="flex-1 overflow-y-auto border border-slate-100 rounded-xl"><table class="w-full text-left text-xs"><thead class="bg-slate-50 text-slate-400 font-medium border-b border-slate-100"><tr><th class="p-3">Name</th><th class="p-3">Channel</th><th class="p-3">Handle / Identity</th><th class="p-3">Lead State</th><th class="p-3">Last Active</th></tr></thead><tbody class="divide-y divide-slate-100">{#each conversations as conversation}<tr class="hover:bg-slate-50/60 cursor-pointer" onclick={() => onOpenConversation(conversation.id)}><td class="p-3 font-medium text-slate-800">{getContactName(conversation)}</td><td class="p-3 capitalize font-medium text-slate-600">{getChannelLabel(conversation.channel_type || conversation.channel?.type)}</td><td class="p-3 text-slate-500">{conversation.contact?.external_identity || 'N/A'}</td><td class="p-3"><span class="px-2 py-0.5 rounded-md font-medium text-[10px] {getTagColor(conversation.lead?.current_state_key)}">{conversation.lead?.current_state_key || 'New'}</span></td><td class="p-3 text-slate-400">{formatTime(conversation.last_message_at)}</td></tr>{/each}</tbody></table></div>
</div>
