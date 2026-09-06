export type SimulatorPlatform =
  | "whatsapp"
  | "instagram"
  | "messenger"
  | "telegram";

export interface TestContact {
  id: string;
  name: string;
  externalID: string;
  avatar: string;
  platform: SimulatorPlatform;
}

export interface PresetCategory {
  label: string;
  stageTag: string;
  badgeColor: string;
  prompts: string[];
}

export interface CascadeTelemetry {
  stageMatched: "pattern" | "embedding" | "llm_grounded" | "none";
  confidence: number | null;
  action: "auto_sent" | "drafted" | "flagged_human" | "none";
  draftText?: string;
  draftStatus?: string;
  lastInboundText?: string;
  lastPayload?: unknown;
  channelID?: string;
  channelType?: string;
  latencyMs?: number;
  timestamp?: string;
}

export interface ConversationSnapshot {
  contactID: string;
  platform: SimulatorPlatform;
  conversationID: string | null;
  messages: any[];
  telemetry: Partial<CascadeTelemetry> | null;
}
