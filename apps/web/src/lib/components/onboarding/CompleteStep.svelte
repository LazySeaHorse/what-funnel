<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	let { productMode }: { productMode: string } = $props();
</script>

			<div class="flex flex-col items-center lg:items-start text-center lg:text-left max-w-lg lg:max-w-none mx-auto lg:mx-0 pb-6 sm:py-2">
				<div
					class="lg:hidden w-[calc(100%+2.5rem)] -mx-5 -mt-2 mb-6 flex items-center justify-center overflow-hidden"
					style="-webkit-mask-image: linear-gradient(to bottom, transparent, black 15%, black 85%, transparent), linear-gradient(to right, transparent, black 28%, black 72%, transparent); -webkit-mask-composite: source-in; mask-image: linear-gradient(to bottom, transparent, black 15%, black 85%, transparent), linear-gradient(to right, transparent, black 28%, black 72%, transparent); mask-composite: intersect;"
				>
					<img
						src="/images/onboarding-happy.webp"
						alt="Workspace Ready Mascot"
						class="w-full max-h-64 object-contain"
					/>
				</div>

				<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Setup Complete</div>
				<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-2">You're all set! 🎉</h2>
				<p class="text-sm text-slate-500 mb-8 font-normal leading-relaxed max-w-md lg:max-w-none">{productMode === 'chatbot_only' ? 'Your channels and AI assistant are ready to handle customer conversations.' : 'Your workspace is configured and ready to turn conversations into customers.'}</p>

				<div class="space-y-3 w-full text-left">
					<div class="p-3.5 sm:p-4 bg-blue-50/60 border border-blue-100 rounded-xl flex items-center gap-3.5">
						<div class="w-9 h-9 rounded-lg bg-blue-600 text-white flex items-center justify-center shrink-0">
							<Icon name="check" size={18} color="#FFFFFF" strokeWidth={2.5} />
						</div>
						<div>
							<div class="text-sm font-medium text-slate-900">Workspace is fully configured</div>
							<div class="text-xs text-slate-500 font-normal mt-0.5">{productMode === 'chatbot_only' ? 'Your business profile, channels, knowledge, and AI preferences are active.' : 'Your business profile, team, channels, and reply preferences are active.'}</div>
						</div>
					</div>

					<div class="p-3.5 sm:p-4 bg-white border border-slate-200 rounded-xl flex items-center gap-3.5">
						<div class="w-9 h-9 rounded-lg bg-slate-100 text-slate-600 flex items-center justify-center shrink-0">
							<Icon name="chat" size={18} color="currentColor" />
						</div>
						<div>
							<div class="text-sm font-medium text-slate-900">Omni-Channel Inbox</div>
							<div class="text-xs text-slate-500 font-normal mt-0.5">Manage live conversations from WhatsApp, Instagram, Messenger, and Telegram.</div>
						</div>
					</div>
				</div>
			</div>
