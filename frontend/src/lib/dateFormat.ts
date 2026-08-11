// Formato consistente dd/mm/aa [hh:mm:ss]

export function formatDate(date: string | Date, includeTime: boolean = false): string {
  const d = typeof date === "string" ? new Date(date) : date;
  if (isNaN(d.getTime())) return "—";

  const day = String(d.getDate()).padStart(2, "0");
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const year = String(d.getFullYear()).slice(-2);

  const dateStr = `${day}/${month}/${year}`;

  if (!includeTime) return dateStr;

  const hours = String(d.getHours()).padStart(2, "0");
  const mins = String(d.getMinutes()).padStart(2, "0");
  const secs = String(d.getSeconds()).padStart(2, "0");

  return `${dateStr} ${hours}:${mins}:${secs}`;
}

// Solo fecha
export function formatDateOnly(date: string | Date): string {
  return formatDate(date, false);
}

// Fecha con hora
export function formatDateTime(date: string | Date): string {
  return formatDate(date, true);
}
