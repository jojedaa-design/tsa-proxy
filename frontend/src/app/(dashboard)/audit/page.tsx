"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type AuditEvent } from "@/lib/api";

const ACTION_COLORS: Record<string, string> = {
  "tenant.create":     "badge-green",
  "tenant.suspend":    "badge-yellow",
  "tenant.reactivate": "badge-green",
  "tenant.update":     "badge-gray",
  "credential.rotate": "badge-yellow",
  "credential.revoke": "badge-red",
  "quota.update":      "badge-gray",
  "admin.login":       "badge-gray",
  "admin.logout":      "badge-gray",
  "ip_allowlist.create": "badge-green",
};

export default function AuditPage() {
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo]     = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["audit-events", page, action, from, to],
    queryFn: () => api.listAuditEvents({ page, action, from: from || undefined, to: to || undefined }),
    refetchInterval: 4000,
  });

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Auditoría</h1>
        <p className="text-sm text-gray-500 mt-0.5">Registro de todas las acciones administrativas</p>
      </div>

      {/* Filtros */}
      <div className="flex gap-3 flex-wrap items-end">
        <div>
          <label className="label">Acción</label>
          <input
            value={action}
            onChange={(e) => { setAction(e.target.value); setPage(1); }}
            className="input"
            placeholder="ej: tenant.suspend"
          />
        </div>
        <div>
          <label className="label">Desde</label>
          <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} className="input" />
        </div>
        <div>
          <label className="label">Hasta</label>
          <input type="date" value={to} onChange={(e) => setTo(e.target.value)} className="input" />
        </div>
        <button
          onClick={() => { setAction(""); setFrom(""); setTo(""); setPage(1); }}
          className="btn-secondary"
        >
          Limpiar filtros
        </button>
      </div>

      {/* Tabla */}
      <div className="card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Fecha</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Actor</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Acción</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Entidad</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading ? (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-400">Cargando...</td></tr>
              ) : data?.data.length === 0 ? (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-400">Sin eventos</td></tr>
              ) : data?.data.map((event) => (
                <AuditRow key={event.id} event={event} />
              ))}
            </tbody>
          </table>
        </div>

        {/* Paginación */}
        {(data?.pagination.total_pages ?? 0) > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200">
            <span className="text-sm text-gray-500">
              {data?.pagination.total.toLocaleString()} eventos · Página {page} de {data?.pagination.total_pages}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn-secondary px-3 py-1 text-xs"
              >← Anterior</button>
              <button
                onClick={() => setPage(p => p + 1)}
                disabled={page >= (data?.pagination.total_pages ?? 1)}
                className="btn-secondary px-3 py-1 text-xs"
              >Siguiente →</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function AuditRow({ event }: { event: AuditEvent }) {
  const [expanded, setExpanded] = useState(false);
  const colorClass = ACTION_COLORS[event.action] ?? "badge-gray";

  return (
    <>
      <tr
        className="hover:bg-gray-50 cursor-pointer"
        onClick={() => event.changes && setExpanded(!expanded)}
      >
        <td className="px-4 py-3 text-gray-500 text-xs whitespace-nowrap">
          {new Date(event.occurred_at).toLocaleString("es-AR")}
        </td>
        <td className="px-4 py-3 text-gray-700">
          {event.actor_name || <span className="text-gray-400">sistema</span>}
        </td>
        <td className="px-4 py-3">
          <span className={`badge ${colorClass}`}>{event.action}</span>
        </td>
        <td className="px-4 py-3 text-gray-600 text-xs font-mono">
          {event.entity_type}
          {event.entity_id && <span className="text-gray-400"> · {event.entity_id.slice(0, 8)}…</span>}
        </td>
        <td className="px-4 py-3 text-gray-500 text-xs font-mono">
          {event.ip_address ?? "—"}
        </td>
      </tr>

      {/* Detalle de cambios */}
      {expanded && event.changes && (
        <tr>
          <td colSpan={5} className="px-4 py-3 bg-gray-50 border-t border-gray-100">
            <pre className="text-xs text-gray-600 font-mono overflow-x-auto">
              {JSON.stringify(event.changes, null, 2)}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}
