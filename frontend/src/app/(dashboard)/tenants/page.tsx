"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api, type Tenant, type APICredential } from "@/lib/api";

export default function TenantsPage() {
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [showCreate, setShowCreate] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["tenants", page, search, status],
    queryFn: () => api.listTenants({ page, limit: 20, search, status }),
    refetchInterval: 4000,
  });

  const suspendMut = useMutation({
    mutationFn: (id: string) => api.suspendTenant(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });

  const reactivateMut = useMutation({
    mutationFn: (id: string) => api.reactivateTenant(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteTenant(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Clientes</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            {data?.pagination.total ?? 0} clientes registrados
          </p>
        </div>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + Nuevo cliente
        </button>
      </div>

      {/* Filtros */}
      <div className="flex gap-3 flex-wrap">
        <input
          type="text"
          placeholder="Buscar por nombre..."
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(1); }}
          className="input max-w-xs"
        />
        <select
          value={status}
          onChange={(e) => { setStatus(e.target.value); setPage(1); }}
          className="input max-w-[160px]"
        >
          <option value="">Todos los estados</option>
          <option value="active">Activos</option>
          <option value="suspended">Suspendidos</option>
        </select>
      </div>

      {/* Tabla */}
      <div className="card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Cliente</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Slug</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Estado</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Cuota mensual</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Creado</th>
                <th className="text-right px-4 py-3 font-medium text-gray-600">Acciones</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">Cargando...</td></tr>
              ) : data?.data.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">No hay clientes</td></tr>
              ) : data?.data.map((tenant) => (
                <TenantRow
                  key={tenant.id}
                  tenant={tenant}
                  onSuspend={() => suspendMut.mutate(tenant.id)}
                  onReactivate={() => reactivateMut.mutate(tenant.id)}
                  onDelete={() => deleteMut.mutate(tenant.id)}
                />
              ))}
            </tbody>
          </table>
        </div>

        {/* Paginación */}
        {(data?.pagination.total_pages ?? 0) > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200">
            <span className="text-sm text-gray-500">
              Página {page} de {data?.pagination.total_pages}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn-secondary px-3 py-1 text-xs"
              >
                ← Anterior
              </button>
              <button
                onClick={() => setPage(p => p + 1)}
                disabled={page >= (data?.pagination.total_pages ?? 1)}
                className="btn-secondary px-3 py-1 text-xs"
              >
                Siguiente →
              </button>
            </div>
          </div>
        )}
      </div>

      {showCreate && (
        <CreateTenantWizard
          onClose={() => setShowCreate(false)}
          onCreated={() => {
            setShowCreate(false);
            qc.invalidateQueries({ queryKey: ["tenants"] });
          }}
        />
      )}
    </div>
  );
}

// ── Tenant Row ────────────────────────────────────────────────

function TenantRow({ tenant, onSuspend, onReactivate, onDelete }: {
  tenant: Tenant;
  onSuspend: () => void;
  onReactivate: () => void;
  onDelete: () => void;
}) {
  return (
    <tr className="hover:bg-gray-50">
      <td className="px-4 py-3">
        <Link href={`/tenants/${tenant.id}`} className="font-medium text-primary-600 hover:underline">
          {tenant.name}
        </Link>
        {tenant.contact_email && (
          <p className="text-xs text-gray-400">{tenant.contact_email}</p>
        )}
      </td>
      <td className="px-4 py-3 text-gray-500 font-mono text-xs">{tenant.slug}</td>
      <td className="px-4 py-3">
        <span className={
          "badge " + (
            tenant.status === "active" ? "badge-green" :
            tenant.status === "suspended" ? "badge-yellow" :
            "badge-gray"
          )
        }>
          {tenant.status === "active" ? "Activo" :
           tenant.status === "suspended" ? "Suspendido" : "Eliminado"}
        </span>
      </td>
      <td className="px-4 py-3 text-gray-600">
        {tenant.quota ? (
          <span>{tenant.quota.monthly_limit.toLocaleString()} / mes</span>
        ) : (
          <span className="text-gray-400 text-xs">Sin cuota</span>
        )}
      </td>
      <td className="px-4 py-3 text-gray-500 text-xs">
        {new Date(tenant.created_at).toLocaleDateString("es-AR")}
      </td>
      <td className="px-4 py-3 text-right">
        <div className="flex justify-end gap-2">
          <Link href={`/tenants/${tenant.id}`} className="btn-secondary px-3 py-1 text-xs">
            Ver
          </Link>
          {tenant.status === "active" ? (
            <button
              onClick={onSuspend}
              className="btn px-3 py-1 text-xs bg-yellow-50 text-yellow-700 border border-yellow-200 hover:bg-yellow-100"
            >
              Suspender
            </button>
          ) : tenant.status === "suspended" ? (
            <button onClick={onReactivate} className="btn-primary px-3 py-1 text-xs">
              Reactivar
            </button>
          ) : null}
          <button
            onClick={() => {
              if (confirm(`⚠️ ¿Eliminar definitivamente el cliente "${tenant.name}"?\n\nEsta acción no se puede deshacer. Se eliminarán todas las credenciales y configuraciones asociadas.`)) {
                onDelete();
              }
            }}
            className="btn px-3 py-1 text-xs bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
          >
            Eliminar
          </button>
        </div>
      </td>
    </tr>
  );
}

// ── Wizard de creación ────────────────────────────────────────

type WizardStep = "create" | "result";

interface IPEntry { cidr: string; label: string; }

interface CreationResult {
  tenant: Tenant;
  credential: APICredential;
  ips: IPEntry[];
  monthlyLimit: number;
  burstPerMinute: number;
}

function CreateTenantWizard({ onClose, onCreated }: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [step, setStep] = useState<WizardStep>("create");

  // Campos de entrada
  const [name, setName] = useState("");
  const [ipEntries, setIpEntries] = useState<IPEntry[]>([]);
  const [ipInput, setIpInput] = useState("");
  const [ipLabel, setIpLabel] = useState("");
  const [allowAllIPs, setAllowAllIPs] = useState(false);

  // Resultado
  const [result, setResult] = useState<CreationResult | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  function addIP() {
    const cidr = ipInput.trim();
    if (!cidr) return;
    // Normalizar: si es IP sin máscara, agregar /32
    const normalized = /\/\d+$/.test(cidr) ? cidr : cidr.includes(":") ? cidr + "/128" : cidr + "/32";
    setIpEntries(prev => [...prev, { cidr: normalized, label: ipLabel.trim() }]);
    setIpInput("");
    setIpLabel("");
  }

  function removeIP(idx: number) {
    setIpEntries(prev => prev.filter((_, i) => i !== idx));
  }

  async function create() {
    setCreating(true);
    setError("");
    try {
      // 1. Crear tenant (con slug automático)
      const tenant = await api.createTenant({
        name,
        slug: undefined,
        contact_email: undefined,
      });

      // 2. Crear credencial (genera URL de firma automáticamente)
      const credential = await api.createCredential(tenant.id, {
        name: `${name} — Principal`,
      });

      // 3. Agregar IPs (en paralelo)
      if (!allowAllIPs && ipEntries.length > 0) {
        await Promise.all(
          ipEntries.map(entry =>
            api.createIPAllowlist(tenant.id, { cidr: entry.cidr, label: entry.label || undefined })
          )
        );
      }

      // 4. Configurar cuota con valores por defecto
      const monthlyLimit = 1000;
      const burstPerMinute = 10;
      await api.updateQuota(tenant.id, {
        monthly_limit: monthlyLimit,
        burst_per_minute: burstPerMinute,
        hard_limit: true,
        auto_suspend: false,
        reset_day: 1,
      });

      setResult({ tenant, credential, ips: ipEntries, monthlyLimit, burstPerMinute });
      setStep("result");
    } catch (err: any) {
      setError(err?.message || err?.error || "Error al crear el cliente. Verificá los datos e intentá nuevamente.");
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="card w-full max-w-lg p-6 mx-4 max-h-[90vh] overflow-y-auto">

        {step !== "result" && (
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-gray-900">Nuevo cliente</h2>
            <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
          </div>
        )}

        {/* ── Crear Cliente: Nombre + Restricción de IP ── */}
        {step === "create" && (
          <div className="space-y-4">
            {/* Nombre del cliente */}
            <div>
              <label className="label">Nombre del cliente *</label>
              <input
                value={name}
                onChange={e => setName(e.target.value)}
                className="input"
                placeholder="Ej: Acme Corp"
                autoFocus
              />
            </div>

            {/* Restricción de IP */}
            <div className="border-t pt-4">
              <h3 className="text-sm font-semibold text-gray-800 mb-2">Restricción de IP</h3>
              <p className="text-xs text-gray-500 mb-3">
                Dejá vacío para permitir cualquier IP, o agregá las direcciones autorizadas.
              </p>

              <label className="flex items-center gap-3 cursor-pointer select-none mb-3">
                <input
                  type="checkbox"
                  checked={allowAllIPs}
                  onChange={e => { setAllowAllIPs(e.target.checked); if (e.target.checked) setIpEntries([]); }}
                  className="w-4 h-4 rounded border-gray-300 text-primary-600"
                />
                <span className="text-sm text-gray-700">Permitir cualquier IP</span>
              </label>

              {!allowAllIPs && (
                <>
                  <div className="flex gap-2 mb-2">
                    <input
                      value={ipInput}
                      onChange={e => setIpInput(e.target.value)}
                      onKeyDown={e => e.key === "Enter" && addIP()}
                      className="input flex-1"
                      placeholder="203.0.113.45 o 10.0.0.0/24"
                      disabled={allowAllIPs}
                    />
                    <button onClick={addIP} className="btn-secondary px-3" disabled={!ipInput || allowAllIPs}>
                      +
                    </button>
                  </div>

                  {ipEntries.length > 0 && (
                    <div className="space-y-1 max-h-32 overflow-y-auto mb-2">
                      {ipEntries.map((e, i) => (
                        <div key={i} className="flex items-center justify-between bg-primary-50 rounded px-3 py-2">
                          <span className="font-mono text-sm text-primary-900">{e.cidr}</span>
                          <button onClick={() => removeIP(i)} className="text-red-400 hover:text-red-600 text-sm">✕</button>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>

            {error && <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>}

            <div className="flex justify-end gap-3 pt-2">
              <button onClick={onClose} className="btn-secondary">Cancelar</button>
              <button
                onClick={create}
                disabled={!name || creating}
                className="btn-primary min-w-[120px]"
              >
                {creating ? (
                  <span className="flex items-center gap-2">
                    <svg className="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/>
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z"/>
                    </svg>
                    Creando…
                  </span>
                ) : "Crear cliente"}
              </button>
            </div>
          </div>
        )}

        {/* ── Paso 4: Resultado ── */}
        {step === "result" && result && (
          <div className="space-y-5">
            {/* Header */}
            <div className="flex items-center gap-3">
              <div className="w-12 h-12 bg-green-100 rounded-full flex items-center justify-center flex-shrink-0">
                <svg className="w-6 h-6 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7"/>
                </svg>
              </div>
              <div>
                <h2 className="text-lg font-semibold text-gray-900">Cliente creado</h2>
                <p className="text-sm text-gray-500">{result.tenant.name}</p>
              </div>
            </div>

            {/* URL de firma — el dato más importante */}
            <div className="bg-primary-50 border border-primary-200 rounded-xl p-4">
              <p className="text-xs font-bold text-primary-700 uppercase tracking-wide mb-1">
                🖊 URL para software de firma
              </p>
              <p className="text-xs text-primary-600 mb-2">
                Configurá esta URL en DAVISIGN, Adobe, JSignPdf, etc. No requiere usuario ni contraseña.
              </p>
              <div className="bg-white border border-primary-200 rounded-lg p-3 font-mono text-xs text-primary-900 break-all">
                {result.credential.stamp_url}
              </div>
              <button
                onClick={() => navigator.clipboard.writeText(result.credential.stamp_url!)}
                className="w-full mt-2 px-3 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors font-medium"
              >
                Copiar URL de firma
              </button>
            </div>

            {/* API Key */}
            <div className="bg-gray-900 rounded-xl p-4">
              <p className="text-xs font-bold text-yellow-400 uppercase tracking-wide mb-1">
                ⚠ API Key — guardar ahora, no se vuelve a mostrar
              </p>
              <div className="font-mono text-xs text-green-400 break-all mt-2">
                {result.credential.api_key}
              </div>
              <button
                onClick={() => navigator.clipboard.writeText(result.credential.api_key!)}
                className="w-full mt-2 px-3 py-1.5 text-xs bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors"
              >
                Copiar API Key
              </button>
            </div>

            {/* Resumen */}
            <div className="grid grid-cols-2 gap-3 text-sm">
              <div className="bg-gray-50 rounded-lg p-3">
                <p className="text-xs text-gray-500 font-medium mb-1">IPs autorizadas</p>
                {result.ips.length === 0 ? (
                  <p className="text-gray-600">Cualquier IP</p>
                ) : (
                  <ul className="space-y-0.5">
                    {result.ips.map((ip, i) => (
                      <li key={i} className="font-mono text-xs text-gray-700">{ip.cidr}</li>
                    ))}
                  </ul>
                )}
              </div>
              <div className="bg-gray-50 rounded-lg p-3">
                <p className="text-xs text-gray-500 font-medium mb-1">Cuota</p>
                <p className="text-gray-700">{result.monthlyLimit.toLocaleString()} sellos/mes</p>
                <p className="text-gray-500 text-xs">{result.burstPerMinute} req/min burst</p>
              </div>
            </div>

            <div className="flex gap-3 pt-1">
              <Link
                href={`/tenants/${result.tenant.id}`}
                className="btn-secondary flex-1 text-center text-sm"
                onClick={onCreated}
              >
                Ver detalle del cliente
              </Link>
              <button onClick={onCreated} className="btn-primary flex-1 text-sm">
                Finalizar
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
