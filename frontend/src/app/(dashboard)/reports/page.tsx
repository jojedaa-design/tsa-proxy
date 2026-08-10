"use client";

import { useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type FailureDetail, type DailyUsage } from "@/lib/api";
import { generateConsumptionReportPDF } from "@/lib/pdfReport";

// ── Helpers de categoría ──────────────────────────────────────

const CATEGORY_COLORS: Record<string, string> = {
  auth:     "bg-red-100 text-red-700 border-red-200",
  quota:    "bg-orange-100 text-orange-700 border-orange-200",
  rate:     "bg-yellow-100 text-yellow-700 border-yellow-200",
  upstream: "bg-purple-100 text-purple-700 border-purple-200",
  request:  "bg-gray-100 text-gray-600 border-gray-200",
  other:    "bg-gray-100 text-gray-600 border-gray-200",
};

const CATEGORY_LABELS: Record<string, string> = {
  auth:     "Autenticación",
  quota:    "Bolsa",
  rate:     "Límite de tasa",
  upstream: "Servidor Upstream",
  request:  "Solicitud",
  other:    "Otro",
};

function CategoryBadge({ category }: { category: string }) {
  const cls = CATEGORY_COLORS[category] ?? CATEGORY_COLORS.other;
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${cls}`}>
      {CATEGORY_LABELS[category] ?? category}
    </span>
  );
}

// ── Componente principal ──────────────────────────────────────

export default function ReportsPage() {
  const [from, setFrom] = useState(() => {
    const d = new Date();
    d.setDate(1);
    return d.toISOString().split("T")[0];
  });
  const [to, setTo]     = useState(() => new Date().toISOString().split("T")[0]);
  const [tenantId, setTenantId] = useState("");
  const [isAdminOrSuperAdmin, setIsAdminOrSuperAdmin] = useState(false);
  const [isViewer, setIsViewer] = useState(false);
  const [viewerTenant, setViewerTenant] = useState<{ id: string; name: string } | null>(null);
  const [username, setUsername] = useState("");
  const [exportingPDF, setExportingPDF] = useState(false);

  // Cargar rol del usuario para determinar qué puede ver.
  // El rol "viewer" está limitado a un único cliente (tenant_scope_detail) —
  // no puede listar /tenants (admin-only), así que su filtro de cliente se
  // resuelve directo desde /auth/me, sin ese endpoint.
  useEffect(() => {
    api.me().then((user) => {
      const roles = user.roles || [];
      const hasAdminAccess = roles.includes("admin") || roles.includes("superadmin");
      setIsAdminOrSuperAdmin(hasAdminAccess);
      setUsername(user.username);
      if (roles.includes("viewer")) {
        setIsViewer(true);
        const own = user.tenant_scope_detail?.[0] ?? null;
        setViewerTenant(own);
        setTenantId(own?.id ?? "");
      }
    }).catch(() => {
      setIsAdminOrSuperAdmin(false);
    });
  }, []);

  const { data, isLoading } = useQuery({
    queryKey: ["usage-report", from, to, tenantId],
    queryFn: () => api.getUsage({ from, to, tenant_id: tenantId || undefined }),
    refetchInterval: 4000,
  });

  const { data: tenants } = useQuery({
    queryKey: ["tenants-simple"],
    queryFn: () => api.listTenants({ limit: 200 }),
    enabled: !isViewer,
    refetchInterval: 4000,
  });

  const { data: ipsData, isLoading: ipsLoading } = useQuery({
    queryKey: ["top-ips", from, to],
    queryFn: () => api.getTopIPs({ from, to }),
    refetchInterval: 4000,
  });

  const { data: agentsData, isLoading: agentsLoading } = useQuery({
    queryKey: ["top-user-agents", from, to],
    queryFn: () => api.getTopUserAgents({ from, to }),
    refetchInterval: 4000,
  });

  const { data: failuresData, isLoading: failuresLoading } = useQuery({
    queryKey: ["failures", from, to, tenantId],
    queryFn: () => api.getFailureBreakdown({ from, to, tenant_id: tenantId || undefined }),
    refetchInterval: 4000,
  });

  const { data: bundleData, isLoading: bundleLoading } = useQuery({
    queryKey: ["bundle-report", tenantId],
    queryFn: () => tenantId ? api.getBundleReport(tenantId) : Promise.resolve(null),
    enabled: !!tenantId,
    refetchInterval: 4000,
  });

  const [consumptionView, setConsumptionView] = useState<"day" | "month" | "year">("day");
  const [consumptionMonth, setConsumptionMonth] = useState(() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
  });
  const [consumptionYear, setConsumptionYear] = useState(() => String(new Date().getFullYear()));

  // Rango de fechas propio de la sección de consumo, independiente del filtro global
  const consumptionFrom = (() => {
    if (consumptionView === "day") return `${consumptionMonth}-01`;
    if (consumptionView === "month") return `${consumptionYear}-01-01`;
    return "2019-01-01"; // vista año: toda la historia del tenant
  })();
  const consumptionTo = (() => {
    if (consumptionView === "day") {
      const [y, m] = consumptionMonth.split("-").map(Number);
      return `${consumptionMonth}-${String(new Date(y, m, 0).getDate()).padStart(2, "0")}`;
    }
    if (consumptionView === "month") return `${consumptionYear}-12-31`;
    return new Date().toISOString().split("T")[0];
  })();

  const { data: dailyData, isLoading: dailyLoading } = useQuery({
    queryKey: ["daily-usage", tenantId, consumptionFrom, consumptionTo],
    queryFn: () => api.getDailyUsage(tenantId, consumptionFrom, consumptionTo),
    enabled: !!tenantId,
    refetchInterval: 4000,
  });

  const currentYear = new Date().getFullYear();
  const yearOptions = Array.from({ length: 7 }, (_, i) => String(currentYear - i));

  async function exportPDF() {
    if (exportingPDF) return;
    setExportingPDF(true);
    try {
      const tenantName = isViewer
        ? (viewerTenant?.name ?? "—")
        : tenantId
          ? (tenants?.data.find((t) => t.id === tenantId)?.name ?? "—")
          : "Todos los clientes";

      // El número correlativo lo asigna el backend (secuencia en BD) — si la
      // llamada falla, el PDF igual se genera, mostrando "N° —".
      const reportNumber = await api.getNextReportNumber()
        .then((r) => r.number)
        .catch(() => undefined);

      await generateConsumptionReportPDF({
        tenantName,
        from,
        to,
        summary: data?.summary as Record<string, unknown> | undefined,
        bundles: bundleData,
        dailyUsage: dailyData ?? undefined,
        ips: ipsData?.data,
        agents: agentsData?.data,
        failures: failuresData,
        showFailures: isAdminOrSuperAdmin,
        generatedByUsername: username,
        reportNumber,
      });
    } finally {
      setExportingPDF(false);
    }
  }

  const hasFailures =
    (failuresData?.proxy_errors?.length ?? 0) > 0 ||
    (failuresData?.auth_failures?.length ?? 0) > 0;

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Reportes de consumo</h1>
          <p className="text-sm text-gray-500 mt-0.5">Análisis de uso por cliente y período</p>
        </div>
        <button onClick={exportPDF} disabled={exportingPDF} className="btn-secondary disabled:opacity-50">
          {exportingPDF ? "Generando..." : "↓ Exportar PDF"}
        </button>
      </div>

      {/* Filtro de Cliente */}
      <div className="flex gap-3 flex-wrap items-end">
        <div>
          <label className="label">Cliente</label>
          {isViewer ? (
            <p className="input bg-gray-50 text-gray-700">{viewerTenant?.name ?? "—"}</p>
          ) : (
            <select value={tenantId} onChange={(e) => setTenantId(e.target.value)} className="input">
              <option value="">Todos los clientes</option>
              {tenants?.data.map((t) => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
          )}
        </div>
      </div>

      {/* Summary cards */}
      {data?.summary && (
        <div className="grid grid-cols-3 gap-4">
          {Object.entries(data.summary as Record<string, unknown>).map(([k, v]) => (
            <div key={k} className="card p-4">
              <p className="text-xs text-gray-500 uppercase tracking-wide">{k.replace(/_/g, " ")}</p>
              <p className="text-2xl font-bold text-gray-900 mt-1">{String(v)}</p>
            </div>
          ))}
        </div>
      )}

      {/* Bolsas contratadas y consumo */}
      {tenantId && (
        <div className="card p-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">Bolsas contratadas y consumo</h2>
          {bundleLoading ? (
            <p className="text-gray-500">Cargando...</p>
          ) : !bundleData || bundleData.length === 0 ? (
            <p className="text-gray-500">Sin bolsas contratadas</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="border-b">
                  <tr>
                    <th className="text-left py-2 pr-4">Fecha de contratación</th>
                    <th className="text-right py-2 px-4">Cantidad</th>
                    <th className="text-right py-2 px-4">Consumido</th>
                    <th className="text-right py-2 px-4">Disponible</th>
                    <th className="text-center py-2 px-4">Alerta</th>
                    <th className="text-left py-2 pl-4">Referencia</th>
                  </tr>
                </thead>
                <tbody>
                  {bundleData.map((bundle) => {
                    const available = bundle.amount - bundle.consumed;
                    const pct = (bundle.consumed / bundle.amount) * 100;
                    return (
                      <tr key={bundle.id} className="border-b">
                        <td className="py-3 pr-4">{new Date(bundle.contracted_at).toLocaleDateString("es-AR")}</td>
                        <td className="text-right font-medium py-3 px-4">{bundle.amount.toLocaleString()}</td>
                        <td className="text-right py-3 px-4">
                          <div className="flex items-center justify-end gap-2">
                            <span>{bundle.consumed.toLocaleString()}</span>
                            <div className="w-16 h-2 bg-gray-200 rounded-full overflow-hidden">
                              <div className="h-full bg-primary-600" style={{ width: `${pct}%` }} />
                            </div>
                          </div>
                        </td>
                        <td className="text-right py-3 px-4 font-medium">{available.toLocaleString()}</td>
                        <td className="text-center py-3 px-4">
                          {bundle.alert_threshold_percent
                            ? <span className="badge badge-yellow">{bundle.alert_threshold_percent}%</span>
                            : <span className="text-gray-300 text-xs">—</span>}
                        </td>
                        <td className="text-gray-600 py-3 pl-4">{bundle.note || "—"}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Consumo de sellos por período */}
      {tenantId && (
        <div className="card p-6">
          <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
            <h2 className="text-lg font-semibold text-gray-900">Consumo de sellos por período</h2>
            <div className="flex items-center gap-2 flex-wrap">
              {/* Selector de mes (vista Día) */}
              {consumptionView === "day" && (
                <div className="flex items-center gap-1.5">
                  <label className="text-xs text-gray-500 whitespace-nowrap">Mes:</label>
                  <input
                    type="month"
                    value={consumptionMonth}
                    onChange={(e) => setConsumptionMonth(e.target.value)}
                    className="input py-1 text-sm"
                  />
                </div>
              )}
              {/* Selector de año (vista Mes) */}
              {consumptionView === "month" && (
                <div className="flex items-center gap-1.5">
                  <label className="text-xs text-gray-500 whitespace-nowrap">Año:</label>
                  <select
                    value={consumptionYear}
                    onChange={(e) => setConsumptionYear(e.target.value)}
                    className="input py-1 text-sm"
                  >
                    {yearOptions.map((y) => (
                      <option key={y} value={y}>{y}</option>
                    ))}
                  </select>
                </div>
              )}
              {/* Pestañas de vista */}
              <div className="flex gap-1 bg-gray-100 rounded-lg p-1">
                {(["day", "month", "year"] as const).map((v) => (
                  <button
                    key={v}
                    onClick={() => setConsumptionView(v)}
                    className={`px-3 py-1 text-sm rounded-md transition-colors ${
                      consumptionView === v
                        ? "bg-white text-primary-700 font-medium shadow-sm"
                        : "text-gray-500 hover:text-gray-700"
                    }`}
                  >
                    {v === "day" ? "Día" : v === "month" ? "Mes" : "Año"}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Subtítulo descriptivo del rango activo */}
          <p className="text-xs text-gray-400 mb-3">
            {consumptionView === "day"
              ? `Días del mes ${new Date(consumptionMonth + "-01T00:00:00").toLocaleDateString("es-AR", { month: "long", year: "numeric" })}`
              : consumptionView === "month"
              ? `Meses del año ${consumptionYear}`
              : "Todos los años con consumo registrado"}
          </p>

          {dailyLoading ? (
            <p className="text-gray-500 text-sm">Cargando...</p>
          ) : !dailyData || dailyData.length === 0 ? (
            <p className="text-gray-500 text-sm">Sin datos para el período seleccionado.</p>
          ) : (
            <ConsumptionTable data={dailyData} view={consumptionView} />
          )}
        </div>
      )}

      {/* ── Análisis de Fallos (solo Admin/SuperAdmin) ──────────────────────────────── */}
      {isAdminOrSuperAdmin && (
      <div className="card overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <div>
            <h2 className="font-semibold text-gray-900">Análisis de fallos</h2>
            <p className="text-xs text-gray-500 mt-0.5">
              Desglose de por qué fallaron los requests en el período seleccionado
            </p>
          </div>
          {(failuresData?.total_proxy_errors ?? 0) + (failuresData?.total_auth_failures ?? 0) > 0 && (
            <span className="text-sm font-semibold text-red-600">
              {((failuresData?.total_proxy_errors ?? 0) + (failuresData?.total_auth_failures ?? 0)).toLocaleString()} fallos totales
            </span>
          )}
        </div>

        {failuresLoading ? (
          <div className="px-4 py-8 text-center text-gray-400 text-sm">Cargando análisis de fallos...</div>
        ) : !hasFailures ? (
          <div className="px-4 py-8 text-center text-gray-400 text-sm">
            ✓ Sin fallos registrados en el período seleccionado
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 divide-y md:divide-y-0 md:divide-x divide-gray-100">

            {/* Errores del proxy (upstream_error, quota_exceeded) */}
            <div className="p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-gray-700">Errores</h3>
                <span className="text-xs text-gray-400">
                  {(failuresData?.total_proxy_errors ?? 0).toLocaleString()} errores
                </span>
              </div>
              <p className="text-xs text-gray-500 mb-3">
                Requests que llegaron al servidor pero no pudieron completarse
              </p>
              {(failuresData?.proxy_errors?.length ?? 0) === 0 ? (
                <p className="text-xs text-gray-400 italic">Sin errores de proxy</p>
              ) : (
                <FailureList items={failuresData!.proxy_errors} total={failuresData!.total_proxy_errors} />
              )}
            </div>

            {/* Conexiones rechazadas (auth, rate limit, etc.) */}
            <div className="p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-gray-700">Conexiones rechazadas</h3>
                <span className="text-xs text-gray-400">
                  {(failuresData?.total_auth_failures ?? 0).toLocaleString()} rechazos
                </span>
              </div>
              <p className="text-xs text-gray-500 mb-3">
                Requests bloqueados antes de procesar (autenticación, IP, cuota, rate limit)
              </p>
              {(failuresData?.auth_failures?.length ?? 0) === 0 ? (
                <p className="text-xs text-gray-400 italic">Sin conexiones rechazadas</p>
              ) : (
                <FailureList items={failuresData!.auth_failures} total={failuresData!.total_auth_failures} />
              )}
            </div>
          </div>
        )}
      </div>
      )}

      {/* Tabla de IPs más activas */}
      <div className="card overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="font-semibold text-gray-900">IPs más activas</h2>
          <p className="text-xs text-gray-500 mt-1">Direcciones IP que más sellos han consumido</p>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-gray-600">IP</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Cliente</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Total</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Exitosos</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Fallidos</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Último uso</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {ipsLoading ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">Cargando...</td></tr>
              ) : !ipsData?.data || ipsData.data.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">Sin datos de IPs para el período seleccionado</td></tr>
              ) : ipsData.data.map((ip, i) => (
                <tr key={i} className="hover:bg-gray-50">
                  <td className="px-4 py-3 font-mono text-xs text-gray-800 font-medium">{ip.ip}</td>
                  <td className="px-4 py-3 text-gray-700">{ip.tenant_name}</td>
                  <td className="px-4 py-3 text-right text-gray-800 font-medium">
                    {ip.requests.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-green-600">
                    {ip.success_count.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-red-500">
                    {ip.fail_count.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-xs text-gray-500">
                    {ip.last_used_at ? new Date(ip.last_used_at).toLocaleString("es-AR") : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Tabla de Software (User-Agent) */}
      <div className="card overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="font-semibold text-gray-900">Software utilizado</h2>
          <p className="text-xs text-gray-500 mt-1">DAVISIGN, Adobe, JSignPdf, y otros clientes</p>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Software</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Total</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Exitosos</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Fallidos</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">IPs únicas</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Latencia prom.</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {agentsLoading ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">Cargando...</td></tr>
              ) : !agentsData?.data || agentsData.data.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">Sin datos para el período seleccionado</td></tr>
              ) : agentsData.data.map((agent, i) => (
                <tr key={i} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-gray-800 text-xs max-w-md truncate">{agent.user_agent}</td>
                  <td className="px-4 py-3 text-right text-gray-800 font-medium">
                    {agent.requests.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-green-600">
                    {agent.success_count.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-red-500">
                    {agent.fail_count.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-600">
                    {agent.unique_ips}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-600">
                    {agent.avg_latency_ms.toFixed(0)} ms
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>


    </div>
  );
}

// ── Sub-componente: lista de motivos de fallo con barra de progreso ──

function FailureList({ items, total }: { items: FailureDetail[]; total: number }) {
  const maxCount = items[0]?.count ?? 1;
  return (
    <div className="space-y-3">
      {items.map((item) => {
        const pct = Math.round((item.count / maxCount) * 100);
        const barColor =
          item.category === "auth"     ? "bg-red-400" :
          item.category === "quota"    ? "bg-orange-400" :
          item.category === "rate"     ? "bg-yellow-400" :
          item.category === "upstream" ? "bg-purple-400" :
          "bg-gray-400";

        return (
          <div key={item.reason}>
            <div className="flex items-center justify-between mb-1">
              <div className="flex items-center gap-2 min-w-0">
                <CategoryBadge category={item.category} />
                <span className="text-sm text-gray-700 truncate">{item.label}</span>
              </div>
              <div className="flex items-center gap-2 ml-2 shrink-0">
                <span className="text-sm font-semibold text-gray-800">
                  {item.count.toLocaleString()}
                </span>
                <span className="text-xs text-gray-400">
                  {total > 0 ? `${Math.round((item.count / total) * 100)}%` : ""}
                </span>
              </div>
            </div>
            <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full ${barColor}`}
                style={{ width: `${pct}%` }}
              />
            </div>
            <p className="text-xs text-gray-400 mt-0.5">
              Último: {new Date(item.last_seen).toLocaleString("es-AR")}
            </p>
          </div>
        );
      })}
    </div>
  );
}

// ── Tabla de consumo por período ──────────────────────────────

type MonthRow  = { key: string; label: string; total: number; successful: number; failed: number; rejected: number };
type YearRow   = MonthRow;

function ConsumptionTable({ data, view }: { data: DailyUsage[]; view: "day" | "month" | "year" }) {
  const rows: MonthRow[] = (() => {
    if (view === "day") {
      return data.map((d) => ({
        key:        d.date,
        label:      new Date(d.date + "T00:00:00").toLocaleDateString("es-AR", { day: "2-digit", month: "short", year: "numeric" }),
        total:      d.total_requests,
        successful: d.successful_requests,
        failed:     d.failed_requests,
        rejected:   d.rejected_requests,
      }));
    }
    const map = new Map<string, MonthRow>();
    data.forEach((d) => {
      const [y, m] = d.date.split("-");
      const key   = view === "month" ? `${y}-${m}` : y;
      const label = view === "month"
        ? new Date(`${y}-${m}-01T00:00:00`).toLocaleDateString("es-AR", { month: "long", year: "numeric" })
        : y;
      const existing = map.get(key) ?? { key, label, total: 0, successful: 0, failed: 0, rejected: 0 };
      existing.total      += d.total_requests;
      existing.successful += d.successful_requests;
      existing.failed     += d.failed_requests;
      existing.rejected   += d.rejected_requests;
      map.set(key, existing);
    });
    // Vista año: solo filas con datos, orden descendente (más reciente primero)
    const all = Array.from(map.values());
    return view === "year"
      ? all.filter((r) => r.total > 0).sort((a, b) => b.key.localeCompare(a.key))
      : all;
  })();

  const grandTotal = rows.reduce((s, r) => s + r.total, 0);

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead className="border-b bg-gray-50">
          <tr>
            <th className="text-left py-2 px-3 font-medium text-gray-600">
              {view === "day" ? "Fecha" : view === "month" ? "Mes" : "Año"}
            </th>
            <th className="text-right py-2 px-3 font-medium text-gray-600">Total</th>
            <th className="text-right py-2 px-3 font-medium text-gray-600">Exitosos</th>
            <th className="text-right py-2 px-3 font-medium text-gray-600">Errores</th>
            <th className="text-right py-2 px-3 font-medium text-gray-600">Rechazados</th>
            <th className="text-right py-2 px-3 font-medium text-gray-600">% Éxito</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {rows.map((row) => {
            const pct = row.total > 0 ? Math.round(row.successful * 100 / row.total) : 0;
            return (
              <tr key={row.key} className="hover:bg-gray-50">
                <td className="py-2 px-3 text-gray-800">{row.label}</td>
                <td className="text-right py-2 px-3 font-semibold">{row.total.toLocaleString("es-AR")}</td>
                <td className="text-right py-2 px-3 text-green-700">{row.successful.toLocaleString("es-AR")}</td>
                <td className="text-right py-2 px-3 text-red-600">{row.failed.toLocaleString("es-AR")}</td>
                <td className="text-right py-2 px-3 text-orange-600">{row.rejected.toLocaleString("es-AR")}</td>
                <td className="text-right py-2 px-3">
                  <span className={`font-medium ${pct >= 95 ? "text-green-700" : pct >= 80 ? "text-yellow-600" : "text-red-600"}`}>
                    {pct}%
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
        <tfoot className="border-t-2 border-gray-200 bg-gray-50">
          <tr>
            <td className="py-2 px-3 font-semibold text-gray-700">Total período</td>
            <td className="text-right py-2 px-3 font-bold">{grandTotal.toLocaleString("es-AR")}</td>
            <td className="text-right py-2 px-3 font-semibold text-green-700">
              {rows.reduce((s, r) => s + r.successful, 0).toLocaleString("es-AR")}
            </td>
            <td className="text-right py-2 px-3 font-semibold text-red-600">
              {rows.reduce((s, r) => s + r.failed, 0).toLocaleString("es-AR")}
            </td>
            <td className="text-right py-2 px-3 font-semibold text-orange-600">
              {rows.reduce((s, r) => s + r.rejected, 0).toLocaleString("es-AR")}
            </td>
            <td className="text-right py-2 px-3 font-bold">
              {grandTotal > 0 ? Math.round(rows.reduce((s, r) => s + r.successful, 0) * 100 / grandTotal) : 0}%
            </td>
          </tr>
        </tfoot>
      </table>
    </div>
  );
}

