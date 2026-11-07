import type { Priority, TicketStatus } from "@/lib/api/client";

// "canceled" joins the picker and list groups once the backend accepts it (board backend ticket).
export const STATUS_ORDER: TicketStatus[] = ["in_progress", "in_review", "todo", "backlog", "done"];
export const STATUS_NAME: Record<TicketStatus, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In Progress",
  in_review: "In Review",
  done: "Done",
  canceled: "Canceled",
};
export const PRIORITY_ORDER: Priority[] = ["urgent", "high", "medium", "low", "none"];
export const PRIORITY_NAME: Record<Priority, string> = {
  urgent: "Urgent",
  high: "High",
  medium: "Medium",
  low: "Low",
  none: "No priority",
};
