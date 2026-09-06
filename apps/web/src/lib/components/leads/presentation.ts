export const defaultPipelineStages = [
  { key: "new", label: "New Lead" },
  { key: "contacted", label: "Contacted" },
  { key: "follow_up", label: "Follow-up" },
  { key: "interested", label: "Interested" },
  { key: "converted", label: "Converted" },
];

export function getLeadStateInfo(key: string, pipelineStates: any[] = []) {
  const matchedState = pipelineStates.find((state) => state.key === key);
  const label =
    matchedState?.label ||
    (
      {
        new: "New Lead",
        contacted: "Contacted",
        follow_up: "Follow-up",
        interested: "Interested",
        converted: "Converted",
        closed_won: "Converted",
      } as Record<string, string>
    )[key] ||
    key;
  const styles: Record<string, { color: string; dot: string; bg: string }> = {
    new: {
      color: "amber",
      dot: "bg-amber-500",
      bg: "bg-amber-50 text-amber-700 border border-amber-200/80",
    },
    contacted: {
      color: "blue",
      dot: "bg-blue-500",
      bg: "bg-blue-50 text-blue-700 border border-blue-200/80",
    },
    follow_up: {
      color: "purple",
      dot: "bg-purple-500",
      bg: "bg-purple-50 text-purple-700 border border-purple-200/80",
    },
    interested: {
      color: "green",
      dot: "bg-emerald-500",
      bg: "bg-emerald-50 text-emerald-700 border border-emerald-200/80",
    },
    converted: {
      color: "emerald",
      dot: "bg-teal-500",
      bg: "bg-teal-50 text-teal-700 border border-teal-200/80",
    },
    closed_won: {
      color: "emerald",
      dot: "bg-teal-500",
      bg: "bg-teal-50 text-teal-700 border border-teal-200/80",
    },
  };
  return { label, ...(styles[key] ?? styles.contacted) };
}
