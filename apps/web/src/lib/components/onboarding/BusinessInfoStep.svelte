<script lang="ts">
	let { step, totalSteps, businessName = $bindable(), businessType = $bindable(), timezone = $bindable() }: { step: number; totalSteps: number; businessName: string; businessType: string; timezone: string } = $props();
</script>

						<div class="text-center lg:text-left mb-6">
							<div class="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Step {step} of {totalSteps}</div>
							<h2 class="text-2xl sm:text-3xl font-medium text-slate-900 tracking-tight mb-1">Let’s start with your business</h2>
							<p class="text-sm text-slate-500 font-normal">This helps us personalize your workspace.</p>
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
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
											<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
										</svg>
									</div>
								</div>
							</div>

							<div>
								<label for="timezone" class="block text-xs font-medium text-slate-700 mb-1.5">Time zone</label>
								<div class="relative w-full">
									<select id="timezone" class="w-full px-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-100 outline-none transition-all appearance-none cursor-pointer pr-10 font-normal" bind:value={timezone}>
										<option value="(GMT+00:00) UTC">(GMT+00:00) UTC</option>
										<option value="(GMT+05:30) Asia / Colombo">(GMT+05:30) Asia / Colombo</option>
										<option value="(GMT-05:00) Eastern Time (US & Canada)">(GMT-05:00) Eastern Time (US & Canada)</option>
										<option value="(GMT-08:00) Pacific Time (US & Canada)">(GMT-08:00) Pacific Time (US & Canada)</option>
										<option value="(GMT+01:00) Paris / Berlin">(GMT+01:00) Paris / Berlin</option>
										<option value="(GMT+08:00) Singapore / Beijing">(GMT+08:00) Singapore / Beijing</option>
										<option value="(GMT+09:00) Tokyo">(GMT+09:00) Tokyo</option>
									</select>
									<div class="absolute inset-y-0 right-0 pr-3.5 flex items-center pointer-events-none text-slate-400">
										<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
											<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
										</svg>
									</div>
								</div>
							</div>
						</div>
