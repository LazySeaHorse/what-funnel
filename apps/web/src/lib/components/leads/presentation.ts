export const defaultPipelineStages = [
  { key: "new", label: "New Lead", color: "#64748B" },
  { key: "contacted", label: "Contacted", color: "#0057D0" },
  { key: "follow_up", label: "Follow-up", color: "#C27AFF" },
  { key: "won", label: "Won", color: "#9AE600" },
  { key: "lost", label: "Lost", color: "#EF4444" },
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
        won: "Won",
        converted: "Converted",
        closed_won: "Won",
        lost: "Lost",
        closed_lost: "Lost",
      } as Record<string, string>
    )[key] ||
    key;
  const styles: Record<string, { color: string; dot: string; bg: string }> = {
    new: {
      color: "slate",
      dot: "bg-slate-400",
      bg: "bg-slate-100 text-slate-700 border border-slate-200",
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
      color: "purple",
      dot: "bg-purple-500",
      bg: "bg-purple-50 text-purple-700 border border-purple-200/80",
    },
    won: {
      color: "green",
      dot: "bg-emerald-500",
      bg: "bg-emerald-50 text-emerald-700 border border-emerald-200/80",
    },
    converted: {
      color: "green",
      dot: "bg-emerald-500",
      bg: "bg-emerald-50 text-emerald-700 border border-emerald-200/80",
    },
    closed_won: {
      color: "green",
      dot: "bg-emerald-500",
      bg: "bg-emerald-50 text-emerald-700 border border-emerald-200/80",
    },
    lost: {
      color: "rose",
      dot: "bg-rose-500",
      bg: "bg-rose-50 text-rose-700 border border-rose-200/80",
    },
    closed_lost: {
      color: "rose",
      dot: "bg-rose-500",
      bg: "bg-rose-50 text-rose-700 border border-rose-200/80",
    },
  };
  return { label, ...(styles[key] ?? styles.contacted) };
}
