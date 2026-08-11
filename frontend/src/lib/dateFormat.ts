// Formato consistente dd/mm/aa [hh:mm:ss] en hora de Lima (Perú).
//
// La zona se fija explícitamente en vez de usar getDate()/getHours(): esos
// métodos devuelven la hora del equipo que renderiza — el navegador del
// usuario o el contenedor en SSR — así que un cliente en otro huso vería
// horas distintas para el mismo sello. Con timeZone fija, la fecha mostrada
// es siempre la de Lima, sea quien sea que abra el panel.
const TIME_ZONE = "America/Lima";

const dateOnlyFormatter = new Intl.DateTimeFormat("es-PE", {
  timeZone: TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "2-digit",
});

const dateTimeFormatter = new Intl.DateTimeFormat("es-PE", {
  timeZone: TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

function partsOf(formatter: Intl.DateTimeFormat, d: Date): Record<string, string> {
  const parts: Record<string, string> = {};
  for (const { type, value } of formatter.formatToParts(d)) {
    parts[type] = value;
  }
  return parts;
}

export function formatDate(date: string | Date, includeTime: boolean = false): string {
  const d = typeof date === "string" ? new Date(date) : date;
  if (isNaN(d.getTime())) return "—";

  const p = partsOf(includeTime ? dateTimeFormatter : dateOnlyFormatter, d);
  const dateStr = `${p.day}/${p.month}/${p.year}`;

  if (!includeTime) return dateStr;

  // "24" en la medianoche es válido para Intl pero se lee mal; se normaliza a "00".
  const hour = p.hour === "24" ? "00" : p.hour;
  return `${dateStr} ${hour}:${p.minute}:${p.second}`;
}

// Solo fecha
export function formatDateOnly(date: string | Date): string {
  return formatDate(date, false);
}

// Fecha con hora
export function formatDateTime(date: string | Date): string {
  return formatDate(date, true);
}

// ── "Ahora" según el calendario de Lima ───────────────────────
// Para valores por defecto de filtros. No usar toISOString(): devuelve UTC,
// así que a partir de las 19:00 de Lima ya reporta el día siguiente.
const isoFormatter = new Intl.DateTimeFormat("en-CA", {
  timeZone: TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

/** Fecha de hoy en Lima como "YYYY-MM-DD". */
export function todayInLima(): string {
  return isoFormatter.format(new Date()); // en-CA ya emite YYYY-MM-DD
}

/** Mes en curso en Lima como "YYYY-MM". */
export function currentMonthInLima(): string {
  return todayInLima().slice(0, 7);
}

/** Año en curso en Lima como número. */
export function currentYearInLima(): number {
  return Number(todayInLima().slice(0, 4));
}
