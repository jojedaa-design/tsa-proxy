"use client";

import { useQuery } from "@tanstack/react-query";
import { api, type DashboardSummary, type HardwareMetrics } from "@/lib/api";

function StatCard({ title, value, sub, color }: {
  title: string;
  value: string | number;
  sub?: string;
  color?: string;
}) {
  return (
    <div className="card p-5">
      <p className="text-sm text-gray-500 font-medium">{title}</p>
      <p className={`text-3xl font-bold mt-1 ${color || "text-gray-900"}`}>
        {typeof value === "number" ? value.toLocaleString("es-AR") : value}
      </p>
      {sub && <p className="text-xs text-gray-400 mt-1">{sub}</p>}
    </div>
  );
}

export default function DashboardPage() {
  const { data: summary, isLoading, error } = useQuery<DashboardSummary>({
    queryKey: ["dashboard-summary"],
    queryFn: () => api.getUsageSummary(),
    refetchInterval: 4000,
  });

  // Failure breakdown para las últimas 24h
  const yesterday = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString().split("T")[0];
  const today     = new Date().toISOString().split("T")[0];
  const { data: failures } = useQuery({
    queryKey: ["failures-24h"],
    queryFn: () => api.getFailureBreakdown({ from: yesterday, to: today }),
    refetchInterval: 4000,
  });

  const allFailures = [
    ...(failures?.proxy_errors  ?? []),
    ...(failures?.auth_failures ?? []),
  ].sort((a, b) => b.count - a.count).slice(0, 5);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-400">Cargando dashboard...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-md bg-red-50 p-4 text-red-700 text-sm">
        Error cargando el dashboard. Intenta recargar la página.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Panel de Control</h1>
        <p className="text-sm text-gray-500 mt-1">Resumen del sistema en tiempo real</p>
      </div>

      {/* Stats grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Clientes activos"
          value={summary?.active_tenants ?? 0}
          sub="tenants con status activo"
          color="text-primary-600"
        />
        <StatCard
          title="Sellos hoy"
          value={summary?.today_requests ?? 0}
          sub="timestamps generados hoy"
          color="text-green-600"
        />
        <StatCard
          title="Sellos este mes"
          value={summary?.month_requests ?? 0}
          sub="acumulado mes actual"
        />
        <StatCard
          title="Errores (24h)"
          value={summary?.errors_last_24h ?? 0}
          sub={`Latencia prom: ${summary?.avg_latency_ms?.toFixed(0) ?? 0} ms`}
          color={(summary?.errors_last_24h ?? 0) > 0 ? "text-red-600" : "text-gray-900"}
        />
      </div>

      {/* Top tenants + Failure reasons */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="card p-5">
          <h2 className="text-base font-semibold text-gray-900 mb-4">
            Top clientes — mes actual
          </h2>
          {(summary?.top_tenants?.length ?? 0) === 0 ? (
            <p className="text-sm text-gray-400">Sin datos todavía.</p>
          ) : (
            <div className="space-y-3">
              {summary?.top_tenants?.map((t, i) => {
                const max = summary.top_tenants[0].requests || 1;
                const pct = Math.round((t.requests / max) * 100);
                return (
                  <div key={t.tenant_id}>
                    <div className="flex justify-between text-sm mb-1">
                      <span className="font-medium text-gray-700">
                        <span className="text-gray-400 mr-2">#{i + 1}</span>
                        {t.tenant_name}
                      </span>
                      <span className="text-gray-500">{t.requests.toLocaleString()}</span>
                    </div>
                    <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-primary-500 rounded-full"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Motivos de fallo — últimas 24h */}
        <div className="card p-5">
          <h2 className="text-base font-semibold text-gray-900 mb-1">
            Motivos de fallo — últimas 24 h
          </h2>
          <p className="text-xs text-gray-400 mb-4">Por qué fallaron los requests recientes</p>
          {allFailures.length === 0 ? (
            <p className="text-sm text-gray-400 text-center py-4">✓ Sin fallos en las últimas 24 h</p>
          ) : (
            <div className="space-y-3">
              {allFailures.map((f) => {
                const max = allFailures[0].count || 1;
                const pct = Math.round((f.count / max) * 100);
                const barColor =
                  f.category === "auth"     ? "bg-red-400" :
                  f.category === "quota"    ? "bg-orange-400" :
                  f.category === "rate"     ? "bg-yellow-400" :
                  f.category === "upstream" ? "bg-purple-400" :
                  "bg-gray-400";
                return (
                  <div key={f.reason}>
                    <div className="flex justify-between text-sm mb-1">
                      <span className="text-gray-700 truncate">{f.label}</span>
                      <span className="font-semibold text-gray-800 ml-2">{f.count.toLocaleString()}</span>
                    </div>
                    <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                      <div className={`h-full rounded-full ${barColor}`} style={{ width: `${pct}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* Estado del sistema */}
      <div className="card p-5">
        <h2 className="text-base font-semibold text-gray-900 mb-4">Estado del sistema</h2>
        <SystemStatus />
      </div>
    </div>
  );
}

function gaugeColor(pct: number) {
  if (pct >= 85) return "bg-red-500";
  if (pct >= 65) return "bg-yellow-400";
  return "bg-green-500";
}

function fmtKBPS(kbps: number) {
  if (kbps >= 1024) return `${(kbps / 1024).toFixed(1)} MB/s`;
  return `${kbps.toFixed(0)} KB/s`;
}

function HardwareGauge({ label, pct, detail }: { label: string; pct: number; detail: string }) {
  const p = Math.round(pct);
  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span className="text-gray-600 font-medium">{label}</span>
        <span className="text-gray-500 text-xs">{detail}</span>
      </div>
      <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-700 ${gaugeColor(p)}`}
          style={{ width: `${Math.min(p, 100)}%` }}
        />
      </div>
      <p className="text-xs text-gray-400 mt-0.5 text-right">{p}%</p>
    </div>
  );
}

function SystemStatus() {
  const { data: health } = useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      const res = await fetch("/ready");
      return res.json();
    },
    refetchInterval: 4000,
  });

  const { data: hw } = useQuery<HardwareMetrics>({
    queryKey: ["system-hardware"],
    queryFn: () => api.getHardwareMetrics(),
    refetchInterval: 3000,
  });

  const services = [
    { label: "API Backend",   status: health?.status === "ready" ? "ok" : "checking" },
    { label: "PostgreSQL",    status: health?.postgres ?? "checking" },
    { label: "Redis",         status: health?.redis ?? "checking" },
    { label: "Autoridad TSA", status: "ok" },
  ];

  return (
    <div className="space-y-6">
      {/* Servicios */}
      <div className="space-y-3">
        {services.map((item) => (
          <div key={item.label} className="flex items-center justify-between">
            <span className="text-sm text-gray-600">{item.label}</span>
            <span className={`badge ${
              item.status === "ok"       ? "badge-green" :
              item.status === "checking" ? "badge-yellow" :
              "badge-red"
            }`}>
              {item.status === "ok"       ? "Operativo" :
               item.status === "checking" ? "Verificando..." :
               "Error"}
            </span>
          </div>
        ))}
      </div>

      {/* Hardware */}
      <div>
        <h3 className="text-sm font-semibold text-gray-700 mb-3">Recursos del servidor</h3>
        {hw ? (
          <div className="space-y-3">
            <HardwareGauge
              label="CPU"
              pct={hw.cpu_percent}
              detail={`${hw.cpu_percent.toFixed(1)}%`}
            />
            <HardwareGauge
              label="Memoria"
              pct={hw.mem_percent}
              detail={`${hw.mem_used_mb.toLocaleString()} / ${hw.mem_total_mb.toLocaleString()} MB`}
            />
            <HardwareGauge
              label="Almacenamiento"
              pct={hw.disk_percent}
              detail={`${hw.disk_used_gb.toFixed(1)} / ${hw.disk_total_gb.toFixed(1)} GB`}
            />
            <div>
              <p className="text-sm text-gray-600 font-medium mb-1">Red</p>
              <div className="flex gap-4 text-xs text-gray-500">
                <span>↓ {fmtKBPS(hw.net_rx_kbps)}</span>
                <span>↑ {fmtKBPS(hw.net_tx_kbps)}</span>
              </div>
            </div>
          </div>
        ) : (
          <p className="text-xs text-gray-400">Cargando métricas...</p>
        )}
      </div>
    </div>
  );
}
