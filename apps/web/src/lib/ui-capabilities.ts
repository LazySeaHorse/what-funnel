export type ProductMode = 'full_workspace' | 'chatbot_only';
export type WorkspaceRole = 'manager' | 'agent';

export interface UICapabilities {
	productMode: ProductMode;
	role: WorkspaceRole;
	isManager: boolean;
	leadTracking: boolean;
	viewContacts: boolean;
	manageAssignments: boolean;
	useReplyDrafts: boolean;
	showConversationSidePanel: boolean;
	manageWorkspace: boolean;
	manageTeam: boolean;
	manageChannels: boolean;
	managePipeline: boolean;
	manageAutomation: boolean;
	manageKnowledge: boolean;
	useSimulator: boolean;
	showOperatorIdentity: boolean;
}

/**
 * The UI is the intersection of product features and role permissions.
 * Backend authorization remains authoritative; this policy keeps navigation,
 * composition, and data loading consistent with it.
 */
export function getUICapabilities(account?: any, user?: any): UICapabilities {
	const productMode: ProductMode = account?.product_mode === 'chatbot_only' ? 'chatbot_only' : 'full_workspace';
	const role: WorkspaceRole = user?.role === 'manager' ? 'manager' : 'agent';
	const isManager = role === 'manager';
	const leadTracking = productMode === 'full_workspace';
	const manageAssignments = isManager && leadTracking;

	return {
		productMode,
		role,
		isManager,
		leadTracking,
		viewContacts: leadTracking,
		manageAssignments,
		useReplyDrafts: leadTracking,
		showConversationSidePanel: leadTracking,
		manageWorkspace: isManager,
		manageTeam: manageAssignments,
		manageChannels: isManager,
		managePipeline: isManager && leadTracking,
		manageAutomation: isManager,
		manageKnowledge: isManager,
		useSimulator: isManager,
		showOperatorIdentity: manageAssignments
	};
}
