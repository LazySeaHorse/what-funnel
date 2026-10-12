import type { UICapabilities } from "$lib/ui-capabilities";

export const DASHBOARD_SECTIONS = [
  "inbox",
  "leads",
  "knowledge",
  "contacts",
  "simulate",
  "settings",
] as const;

export type DashboardSection = (typeof DASHBOARD_SECTIONS)[number];

export function isDashboardSection(value: string): value is DashboardSection {
  return (DASHBOARD_SECTIONS as readonly string[]).includes(value);
}

/** Whether the signed-in user may see a dashboard section. */
export function canOpenSection(
  section: DashboardSection,
  capabilities: UICapabilities,
): boolean {
  switch (section) {
    case "leads":
      return capabilities.leadTracking;
    case "contacts":
      return capabilities.viewContacts;
    case "knowledge":
      return capabilities.manageKnowledge;
    case "simulate":
      return capabilities.useSimulator;
    default:
      return true;
  }
}
