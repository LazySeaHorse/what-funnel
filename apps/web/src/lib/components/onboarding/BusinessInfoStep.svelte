<script lang="ts">
	import { ChevronDownIcon } from '@fvilers/heroicons-svelte/24/outline';
	import { formatTimeZoneLabel, supportedTimeZones, normalizeSavedTimeZone } from '$lib/timezones';

	let { step, totalSteps, businessName = $bindable(), businessType = $bindable(), timezone = $bindable() }: { step: number; totalSteps: number; businessName: string; businessType: string; timezone: string } = $props();

	$effect(() => {
		const normalized = normalizeSavedTimeZone(timezone);
		if (normalized !== timezone && supportedTimeZones.includes(normalized)) {
			timezone = normalized;
		}
	});
</script>

						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Enter business information</h2>
							<p class="text-sm text-slate-500 font-normal">The system uses this information to configure your workspace.</p>
						</div>

						<div class="space-y-5 w-full max-w-xl lg:max-w-none mx-auto lg:mx-0">
							<div>
								<label for="business-name" class="block text-xs font-medium text-slate-700 mb-1.5">Business name</label>
								<input
									id="business-name"
									type="text"
									class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all font-normal"
									placeholder="Enter your business name"
									bind:value={businessName}
								/>
							</div>

							<div>
								<label for="business-type" class="block text-xs font-medium text-slate-700 mb-1.5">Business type</label>
								<div class="relative w-full">
									<select id="business-type" class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all appearance-none cursor-pointer pr-10 font-normal" bind:value={businessType}>
										<option value="" disabled>Select business type</option>
										<option value="Salon / Beauty">Salon / Beauty</option>
										<option value="Photography">Photography</option>
										<option value="Tutoring / Education">Tutoring / Education</option>
										<option value="Home Services">Home Services</option>
										<option value="E-commerce / Retail">E-commerce / Retail</option>
										<option value="Consulting / Agency">Consulting / Agency</option>
										<option value="Other">Other</option>
									</select>
									<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center pointer-events-none text-slate-400">
										<ChevronDownIcon class="w-4 h-4" />
									</div>
								</div>
							</div>

							<div>
								<label for="timezone" class="block text-xs font-medium text-slate-700 mb-1.5">Time zone</label>
								<div class="relative w-full">
									<select id="timezone" class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all appearance-none cursor-pointer pr-10 font-normal" bind:value={timezone}>
										{#if timezone && !supportedTimeZones.includes(timezone)}
											<option value={timezone}>{formatTimeZoneLabel(timezone)}</option>
										{/if}
										{#each supportedTimeZones as zone}
											<option value={zone}>{formatTimeZoneLabel(zone)}</option>
										{/each}
									</select>
									<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center pointer-events-none text-slate-400">
										<ChevronDownIcon class="w-4 h-4" />
									</div>
								</div>
							</div>
						</div>
