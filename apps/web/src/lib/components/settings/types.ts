export type SettingsSection =
  | "general"
  | "business_profile"
  | "ai_provider"
  | "users_permissions"
  | "channels"
  | "pipeline";

export interface WorkspaceSettingsForm {
  workspaceName: string;
  defaultTimeZone: string;
  language: string;
  dateFormat: string;
  timeFormat: "12" | "24";
  businessCategory: string;
  businessPhone: string;
  businessEmail: string;
  businessAddress: string;
  businessWebsite: string;
  businessHours: string;
  productMode: string;
  leadTracking: boolean;
  unassignedVisible: boolean;
}
