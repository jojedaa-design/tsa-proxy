import jsPDF from "jspdf";
import autoTable from "jspdf-autotable";
import type { IPUsage, UserAgentStat, FailureBreakdownResponse, DailyUsage } from "./api";

// ── Colores de marca (BIGDAVI) ───────────────────────────────
const NAVY: [number, number, number] = [11, 31, 58];
const BLUE: [number, number, number] = [18, 116, 242];
const GRAY_TEXT: [number, number, number] = [75, 85, 99];
const GRAY_LIGHT: [number, number, number] = [156, 163, 175];
const WHITE: [number, number, number] = [255, 255, 255];

// Aspect ratio del logo (viewBox del SVG): 1762.58 x 814.2
const LOGO_VIEWBOX_RATIO = 1762.58 / 814.2;
const LOGO_RENDER_WIDTH = 900;
const LOGO_RENDER_HEIGHT = Math.round(LOGO_RENDER_WIDTH / LOGO_VIEWBOX_RATIO);

type BundleItem = {
  id: string;
  amount: number;
  consumed: number;
  note?: string;
  contracted_at: string;
};

export interface ConsumptionReportInput {
  tenantName: string;
  from: string;
  to: string;
  summary?: Record<string, unknown>;
  bundles?: BundleItem[] | null;
  dailyUsage?: DailyUsage[];
  ips?: IPUsage[];
  agents?: UserAgentStat[];
  failures?: FailureBreakdownResponse;
  showFailures: boolean;
  generatedByUsername?: string;
  /** Número correlativo del reporte (asignado por el backend). */
  reportNumber?: number;
}

const MAX_ROWS = 6;
const MAX_FAILURE_ROWS = 5;

function fmtDate(iso: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString("es-AR");
}

function fmtLabel(key: string): string {
  return key
    .replace(/_/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

function fmtValue(v: unknown): string {
  if (typeof v === "number") return v.toLocaleString("es-AR");
  return String(v);
}

function fmtReportNumber(n?: number): string {
  if (n == null) return "N° —";
  return `N° ${String(n).padStart(6, "0")}`;
}

/**
 * Rasteriza el logo blanco de BIGDAVI (SVG, fondo transparente) a PNG para
 * poder incrustarlo en el PDF. Se le inyecta width/height explícitos al SVG
 * antes de rasterizarlo para forzar una resolución alta y nítida — sin esto,
 * el navegador podría rasterizarlo internamente a un tamaño por defecto bajo.
 */
async function loadLogoAsPng(): Promise<{ dataUrl: string; aspectRatio: number } | null> {
  try {
    const res = await fetch("/brand/logo-white.svg");
    if (!res.ok) return null;
    const svgText = await res.text();
    const sizedSvg = svgText.replace(
      "<svg ",
      `<svg width="${LOGO_RENDER_WIDTH}" height="${LOGO_RENDER_HEIGHT}" `
    );
    const blob = new Blob([sizedSvg], { type: "image/svg+xml" });
    const url = URL.createObjectURL(blob);
    try {
      const img = await new Promise<HTMLImageElement>((resolve, reject) => {
        const el = new Image();
        el.onload = () => resolve(el);
        el.onerror = () => reject(new Error("logo load failed"));
        el.src = url;
      });
      const canvas = document.createElement("canvas");
      canvas.width = LOGO_RENDER_WIDTH;
      canvas.height = LOGO_RENDER_HEIGHT;
      const ctx = canvas.getContext("2d");
      if (!ctx) return null;
      ctx.drawImage(img, 0, 0, LOGO_RENDER_WIDTH, LOGO_RENDER_HEIGHT);
      return { dataUrl: canvas.toDataURL("image/png"), aspectRatio: LOGO_RENDER_WIDTH / LOGO_RENDER_HEIGHT };
    } finally {
      URL.revokeObjectURL(url);
    }
  } catch {
    return null;
  }
}

/**
 * Genera un PDF ejecutivo de una sola hoja con el reporte de consumo actual
 * y dispara la descarga en el navegador.
 */
export async function generateConsumptionReportPDF(input: ConsumptionReportInput): Promise<void> {
  const doc = new jsPDF({ orientation: "portrait", unit: "mm", format: "a4" });
  const pageWidth = doc.internal.pageSize.getWidth();
  const pageHeight = doc.internal.pageSize.getHeight();
  const margin = 12;
  const contentWidth = pageWidth - margin * 2;

  const now = new Date();
  const generatedAt = now.toLocaleString("es-AR", {
    day: "2-digit", month: "2-digit", year: "numeric",
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  });

  const logo = await loadLogoAsPng();

  // ── Encabezado ──────────────────────────────────────────────
  // Logo al 400% (aumentado en 300%) del tamaño original de 9mm de alto.
  const logoHeight = 9 * 4;
  const headerHeight = 56;
  doc.setFillColor(...NAVY);
  doc.rect(0, 0, pageWidth, headerHeight, "F");

  if (logo) {
    const logoWidth = logoHeight * logo.aspectRatio;
    doc.addImage(logo.dataUrl, "PNG", margin, 6, logoWidth, logoHeight);
  } else {
    doc.setTextColor(...WHITE);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(32);
    doc.text("BIGDAVI", margin, 28);
  }

  doc.setTextColor(...WHITE);
  doc.setFont("helvetica", "normal");
  doc.setFontSize(10);
  doc.text("Reporte de Consumo de Sellos de Tiempo", margin, headerHeight - 9);
  doc.setFontSize(8);
  doc.setTextColor(200, 210, 225);
  doc.text(fmtReportNumber(input.reportNumber), margin, headerHeight - 4);

  doc.setFontSize(8.5);
  doc.setTextColor(...WHITE);
  const infoLines = [
    `Generado: ${generatedAt}`,
    `Cliente: ${input.tenantName}`,
    `Período: ${fmtDate(input.from)} – ${fmtDate(input.to)}`,
  ];
  if (input.generatedByUsername) infoLines.push(`Usuario: ${input.generatedByUsername}`);
  const infoStartY = (headerHeight - infoLines.length * 5) / 2 + 4;
  infoLines.forEach((line, i) => {
    doc.text(line, pageWidth - margin, infoStartY + i * 5, { align: "right" });
  });

  let y = headerHeight + 8;

  // ── Resumen del período ─────────────────────────────────────
  const summaryEntries = Object.entries(input.summary ?? {});
  if (summaryEntries.length > 0) {
    doc.setTextColor(...NAVY);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(11);
    doc.text("Resumen del período", margin, y);
    y += 3;

    autoTable(doc, {
      startY: y,
      margin: { left: margin, right: margin },
      head: [summaryEntries.map(([k]) => fmtLabel(k))],
      body: [summaryEntries.map(([, v]) => fmtValue(v))],
      theme: "grid",
      headStyles: { fillColor: NAVY, textColor: WHITE, fontSize: 8, halign: "center" },
      bodyStyles: { fontSize: 11, halign: "center", fontStyle: "bold", textColor: BLUE },
      styles: { cellPadding: 3 },
    });
    y = (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY + 8;
  }

  // ── Bolsas contratadas y consumo ────────────────────────────
  if (input.bundles && input.bundles.length > 0) {
    doc.setTextColor(...NAVY);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(11);
    doc.text("Bolsas contratadas y consumo", margin, y);
    y += 3;

    const shown = input.bundles.slice(0, MAX_ROWS);
    autoTable(doc, {
      startY: y,
      margin: { left: margin, right: margin },
      head: [["Fecha", "Cantidad", "Consumido", "Disponible", "Referencia"]],
      body: shown.map((b) => [
        fmtDate(b.contracted_at),
        b.amount.toLocaleString("es-AR"),
        b.consumed.toLocaleString("es-AR"),
        (b.amount - b.consumed).toLocaleString("es-AR"),
        b.note || "—",
      ]),
      theme: "striped",
      headStyles: { fillColor: BLUE, textColor: WHITE, fontSize: 8, halign: "center" },
      bodyStyles: { fontSize: 8, textColor: GRAY_TEXT, halign: "center" },
    });
    y = (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY;
    if (input.bundles.length > MAX_ROWS) {
      y += 4;
      doc.setFontSize(7.5);
      doc.setTextColor(...GRAY_LIGHT);
      doc.setFont("helvetica", "italic");
      doc.text(`+${input.bundles.length - MAX_ROWS} bolsa(s) adicional(es) — ver detalle completo en el panel.`, margin, y);
    }
    y += 8;
  }

  // ── Consumo por día (máx 30 filas para no romper el PDF) ───────
  if (input.dailyUsage && input.dailyUsage.length > 0) {
    const MAX_DAILY = 30;
    const shown = input.dailyUsage.slice(-MAX_DAILY); // últimos N días del período

    doc.setTextColor(...NAVY);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(11);
    doc.text("Consumo de sellos por día", margin, y);
    y += 3;

    autoTable(doc, {
      startY: y,
      margin: { left: margin, right: margin },
      head: [["Fecha", "Total", "Exitosos", "Errores", "Rechazados", "% Éxito"]],
      body: shown.map((d) => {
        const pct = d.total_requests > 0 ? Math.round(d.successful_requests * 100 / d.total_requests) : 0;
        return [
          d.date,
          d.total_requests.toLocaleString("es-AR"),
          d.successful_requests.toLocaleString("es-AR"),
          d.failed_requests.toLocaleString("es-AR"),
          d.rejected_requests.toLocaleString("es-AR"),
          `${pct}%`,
        ];
      }),
      theme: "striped",
      headStyles: { fillColor: BLUE, textColor: WHITE, fontSize: 7, halign: "center" },
      bodyStyles: { fontSize: 7, textColor: GRAY_TEXT, halign: "center" },
    });
    y = (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY;
    if (input.dailyUsage.length > MAX_DAILY) {
      y += 4;
      doc.setFontSize(7.5);
      doc.setTextColor(...GRAY_LIGHT);
      doc.setFont("helvetica", "italic");
      doc.text(`Solo se muestran los últimos ${MAX_DAILY} días. Ver detalle completo en el panel.`, margin, y);
    }
    y += 8;
  }

  // ── IPs más activas / Software utilizado (dos columnas) ─────
  const halfWidth = (contentWidth - 6) / 2;
  const rightX = margin + halfWidth + 6;
  const sideStartY = y;
  let maxFinalY = y;

  if (input.ips && input.ips.length > 0) {
    doc.setTextColor(...NAVY);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(10);
    doc.text("IPs más activas", margin, y);

    autoTable(doc, {
      startY: y + 3,
      margin: { left: margin, right: pageWidth - margin - halfWidth },
      tableWidth: halfWidth,
      head: [["IP", "Total", "Fallidos"]],
      body: input.ips.slice(0, MAX_ROWS).map((ip) => [
        ip.ip,
        ip.requests.toLocaleString("es-AR"),
        ip.fail_count.toLocaleString("es-AR"),
      ]),
      theme: "striped",
      headStyles: { fillColor: NAVY, textColor: WHITE, fontSize: 7, halign: "center" },
      bodyStyles: { fontSize: 7, textColor: GRAY_TEXT, halign: "center" },
    });
    maxFinalY = Math.max(maxFinalY, (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY);
  }

  if (input.agents && input.agents.length > 0) {
    doc.setTextColor(...NAVY);
    doc.setFont("helvetica", "bold");
    doc.setFontSize(10);
    doc.text("Software utilizado", rightX, sideStartY);

    autoTable(doc, {
      startY: sideStartY + 3,
      margin: { left: rightX, right: margin },
      tableWidth: halfWidth,
      head: [["Software", "Total", "Fallidos"]],
      body: input.agents.slice(0, MAX_ROWS).map((a) => [
        a.user_agent.length > 28 ? a.user_agent.slice(0, 28) + "…" : a.user_agent,
        a.requests.toLocaleString("es-AR"),
        a.fail_count.toLocaleString("es-AR"),
      ]),
      theme: "striped",
      headStyles: { fillColor: NAVY, textColor: WHITE, fontSize: 7, halign: "center" },
      bodyStyles: { fontSize: 7, textColor: GRAY_TEXT, halign: "center" },
    });
    maxFinalY = Math.max(maxFinalY, (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY);
  }

  y = maxFinalY + 8;

  // ── Análisis de fallos (solo admin/superadmin) ──────────────
  if (input.showFailures && input.failures) {
    const hasProxyErrors = input.failures.proxy_errors.length > 0;
    const hasAuthFailures = input.failures.auth_failures.length > 0;

    if (hasProxyErrors || hasAuthFailures) {
      const failStartY = y;
      let failMaxY = y;

      doc.setTextColor(...NAVY);
      doc.setFont("helvetica", "bold");
      doc.setFontSize(10);
      doc.text("Errores", margin, y);

      if (hasProxyErrors) {
        autoTable(doc, {
          startY: y + 3,
          margin: { left: margin, right: pageWidth - margin - halfWidth },
          tableWidth: halfWidth,
          head: [["Motivo", "Casos"]],
          body: input.failures.proxy_errors.slice(0, MAX_FAILURE_ROWS).map((f) => [
            f.label, f.count.toLocaleString("es-AR"),
          ]),
          theme: "striped",
          headStyles: { fillColor: [185, 28, 28], textColor: WHITE, fontSize: 7, halign: "center" },
          bodyStyles: { fontSize: 7, textColor: GRAY_TEXT, halign: "center" },
        });
        failMaxY = Math.max(failMaxY, (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY);
      } else {
        doc.setFontSize(8);
        doc.setFont("helvetica", "italic");
        doc.setTextColor(...GRAY_LIGHT);
        doc.text("Sin errores de proxy", margin, y + 6);
        failMaxY = Math.max(failMaxY, y + 6);
      }

      doc.setTextColor(...NAVY);
      doc.setFont("helvetica", "bold");
      doc.setFontSize(10);
      doc.text("Conexiones rechazadas", rightX, failStartY);

      if (hasAuthFailures) {
        autoTable(doc, {
          startY: failStartY + 3,
          margin: { left: rightX, right: margin },
          tableWidth: halfWidth,
          head: [["Motivo", "Casos"]],
          body: input.failures.auth_failures.slice(0, MAX_FAILURE_ROWS).map((f) => [
            f.label, f.count.toLocaleString("es-AR"),
          ]),
          theme: "striped",
          headStyles: { fillColor: [217, 119, 6], textColor: WHITE, fontSize: 7, halign: "center" },
          bodyStyles: { fontSize: 7, textColor: GRAY_TEXT, halign: "center" },
        });
        failMaxY = Math.max(failMaxY, (doc as unknown as { lastAutoTable: { finalY: number } }).lastAutoTable.finalY);
      } else {
        doc.setFontSize(8);
        doc.setFont("helvetica", "italic");
        doc.setTextColor(...GRAY_LIGHT);
        doc.text("Sin conexiones rechazadas", rightX, failStartY + 6);
        failMaxY = Math.max(failMaxY, failStartY + 6);
      }

      y = failMaxY + 8;
    }
  }

  // ── Pie de página ────────────────────────────────────────────
  const footerY = pageHeight - 12;
  doc.setDrawColor(...GRAY_LIGHT);
  doc.setLineWidth(0.2);
  doc.line(margin, footerY - 4, pageWidth - margin, footerY - 4);

  doc.setFont("helvetica", "normal");
  doc.setFontSize(7);
  doc.setTextColor(...GRAY_LIGHT);
  doc.text("Documento generado automáticamente. Confidencial, uso interno.", margin, footerY);
  doc.text("Página 1 de 1", pageWidth - margin, footerY, { align: "right" });

  // ── Descarga ──────────────────────────────────────────────────
  const stamp = now.toISOString().replace(/[:.]/g, "-").slice(0, 16);
  const safeTenant = input.tenantName.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || "todos";
  doc.save(`reporte-consumo-${safeTenant}-${stamp}.pdf`);
}
