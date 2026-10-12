export interface PipelineStage {
	key: string;
	label: string;
	color: string;
}

const NEW_STAGE_COLORS = ['#C27AFF', '#0057D0', '#FB64B6', '#9AE600', '#7E22CE', '#0046A8'];

/** Builds the stage appended by "Add stage", cycling through the palette. */
export function newPipelineStage(stages: PipelineStage[]): PipelineStage {
	return {
		key: `stage_${Date.now()}_${stages.length + 1}`,
		label: 'New Stage',
		color: NEW_STAGE_COLORS[stages.length % NEW_STAGE_COLORS.length]
	};
}

/** Normalizes labels and colors and gives every stage a unique key before saving. */
export function sanitizePipelineStages(stages: PipelineStage[]): PipelineStage[] {
	const usedKeys = new Set<string>();
	return stages.map((s, idx) => {
		const slug = (s.label || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
		const baseKey = slug || s.key || `stage_${idx + 1}`;
		let key = baseKey;
		let counter = 1;
		while (usedKeys.has(key)) {
			counter++;
			key = `${baseKey}_${counter}`;
		}
		usedKeys.add(key);
		return {
			key,
			label: (s.label || '').trim() || 'Stage',
			color: s.color || '#3B82F6'
		};
	});
}
