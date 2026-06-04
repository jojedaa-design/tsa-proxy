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
  });

  const suspendMut = useMutation({
    mutationFn: (id: string) => api.suspendTenant(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tenants"] }),
  });

  const reactivateMut = useMutation({
    mutationFn: (id: string) => api.reactivateTenant(id),
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

function TenantRow({ tenant, onSuspend, onReactivate }: {
  tenant: Tenant;
  onSuspend: () => void;
  onReactivate: () => void;
}) {
  return (
    <tr className="hover:bg-gray-50">
      <td className="px-4 py-3">
        <Link href={`/tenants/${tenant.id}`} className="font-medium text-blue-600 hover:underline">
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
        </div>
      </td>
    </tr>
  );
}

// ── Wizard de creación ────────────────────────────────────────

type WizardStep = "info" | "ip" | "quota" | "result";

interface IPEntry { cidr: string; label: string; }

interface CreationResult {
  tenant: Tenant;
  credential: APICredential;
  ips: IPEntry[];
  monthlyLimit: number;
  burstPerMinute: number;
}

function StepIndicator({ current }: { current: WizardStep }) {
  const steps: { id: WizardStep; label: string }[] = [
    { id: "info",  label: "Datos" },
    { id: "ip",    label: "IPs" },
    { id: "quota", label: "Cuota" },
    { id: "result", label: "Listo" },
  ];
  const idx = steps.findIndex(s => s.id === current);
  return (
    <div className="flex items-center gap-1 mb-6">
      {steps.map((s, i) => (
        <div key={s.id} className="flex items-center gap-1">
          <div className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold transition-colors ${
            i < idx ? "bg-green-500 text-white" :
            i === idx ? "bg-blue-600 text-white" :
            "bg-gray-100 text-gray-400"
          }`}>
            {i < idx ? "✓" : i + 1}
          </div>
          <span className={`text-xs ${i === idx ? "text-blue-600 font-medium" : "text-gray-400"}`}>
            {s.label}
          </span>
          {i < steps.length - 1 && <div className="w-6 h-px bg-gray-200 mx-1" />}
        </div>
      ))}
    </div>
  );
}

function CreateTenantWizard({ onClose, onCreated }: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [step, setStep] = useState<WizardStep>("info");

  // Paso 1 — Info
  const [name, setName]   = useState("");
  const [slug, setSlug]   = useState("");
  const [email, setEmail] = useState("");
  const [credName, setCredName] = useState("");

  // Paso 2 — IP
  const [ipEntries, setIpEntries] = useState<IPEntry[]>([]);
  const [ipInput, setIpInput]     = useState("");
  const [ipLabel, setIpLabel]     = useState("");
  const [allowAllIPs, setAllowAllIPs] = useState(false);

  // Paso 3 — Cuota
  const [monthly, setMonthly]     = useState("1000");
  const [burst, setBurst]         = useState("10");
  const [hardLimit, setHardLimit] = useState(true);
  const [autoSuspend, setAutoSuspend] = useState(false);

  // Resultado
  const [result, setResult]   = useState<CreationResult | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError]     = useState("");

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
      // 1. Crear tenant
      const tenant = await api.createTenant({
        name,
        slug: slug || undefined,
        contact_email: email || undefined,
      });

      // 2. Crear credencial (genera URL de firma automáticamente)
      const credential = await api.createCredential(tenant.id, {
        name: credName || `${name} — Principal`,
      });

      // 3. Agregar IPs (en paralelo)
      if (!allowAllIPs && ipEntries.length > 0) {
        await Promise.all(
          ipEntries.map(entry =>
            api.createIPAllowlist(tenant.id, { cidr: entry.cidr, label: entry.label || undefined })
          )
        );
      }

      // 4. Configurar cuota
      await api.updateQuota(tenant.id, {
        monthly_limit:   parseInt(monthly)  || 1000,
        burst_per_minute: parseInt(burst)   || 10,
        hard_limit:      hardLimit,
        auto_suspend:    autoSuspend,
        reset_day:       1,
      });

      setResult({ tenant, credential, ips: ipEntries, monthlyLimit: parseInt(monthly), burstPerMinute: parseInt(burst) });
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
          <>
            <div className="flex items-center justify-between mb-2">
              <h2 className="text-lg font-semibold text-gray-900">Nuevo cliente</h2>
              <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
            </div>
            <StepIndicator current={step} />
          </>
        )}

        {/* ── Paso 1: Datos ── */}
        {step === "info" && (
          <div className="space-y-4">
            <div>
              <label className="label">Nombre *</label>
              <input value={name} onChange={e => setName(e.target.value)} className="input" placeholder="Acme Corp" autoFocus />
            </div>
            <div>
              <label className="label">Slug <span className="text-gray-400 font-normal">(se genera automáticamente)</span></label>
              <input value={slug} onChange={e => setSlug(e.target.value)} className="input font-mono" placeholder="acme-corp" />
            </div>
            <div>
              <label className="label">Email de contacto</label>
              <input value={email} onChange={e => setEmail(e.target.value)} className="input" type="email" placeholder="admin@acme.com" />
            </div>
            <div>
              <label className="label">Nombre de la credencial</label>
              <input value={credName} onChange={e => setCredName(e.target.value)} className="input" placeholder={`${name || "Cliente"} — Principal`} />
              <p className="text-xs text-gray-400 mt-1">Se creará automáticamente una URL de firma única para este cliente.</p>
            </div>

            {error && <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>}

            <div className="flex justify-end gap-3 pt-2">
              <button onClick={onClose} className="btn-secondary">Cancelar</button>
              <button
                onClick={() => { setError(""); setStep("ip"); }}
                disabled={!name}
                className="btn-primary"
              >
                Siguiente →
              </button>
            </div>
          </div>
        )}

        {/* ── Paso 2: Restricción de IP ── */}
        {step === "ip" && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-800 mb-1">Restricción de IP</h3>
              <p className="text-xs text-gray-500">
                Si agregás IPs, solo esas direcciones podrán usar los sellos. Si no agregás ninguna, se permite cualquier IP.
              </p>
            </div>

            <label className="flex items-center gap-3 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={allowAllIPs}
                onChange={e => { setAllowAllIPs(e.target.checked); if (e.target.checked) setIpEntries([]); }}
                className="w-4 h-4 rounded border-gray-300 text-blue-600"
              />
              <span className="text-sm text-gray-700">Sin restricción de IP (cualquier IP puede acceder)</span>
            </label>

            {!allowAllIPs && (
              <>
                <div className="flex gap-2">
                  <input
                    value={ipInput}
                    onChange={e => setIpInput(e.target.value)}
                    onKeyDown={e => e.key === "Enter" && addIP()}
                    className="input flex-1"
                    placeholder="203.0.113.45 o 10.0.0.0/24"
                    disabled={allowAllIPs}
                  />
                  <input
                    value={ipLabel}
                    onChange={e => setIpLabel(e.target.value)}
                    onKeyDown={e => e.key === "Enter" && addIP()}
                    className="input w-36"
                    placeholder="Etiqueta"
                    disabled={allowAllIPs}
                  />
                  <button onClick={addIP} className="btn-secondary px-3" disabled={!ipInput || allowAllIPs}>
                    +
                  </button>
                </div>

                {ipEntries.length === 0 ? (
                  <p className="text-xs text-amber-600 bg-amber-50 border border-amber-100 rounded p-2">
                    Sin IPs configuradas — se permitirá cualquier IP hasta que agregues una.
                  </p>
                ) : (
                  <div className="space-y-1 max-h-40 overflow-y-auto">
                    {ipEntries.map((e, i) => (
                      <div key={i} className="flex items-center justify-between bg-gray-50 rounded px-3 py-2">
                        <div>
                          <span className="font-mono text-sm text-gray-800">{e.cidr}</span>
                          {e.label && <span className="ml-2 text-xs text-gray-500">{e.label}</span>}
                        </div>
                        <button onClick={() => removeIP(i)} className="text-red-400 hover:text-red-600 text-xs">✕</button>
                      </div>
                    ))}
                  </div>
                )}
              </>
            )}

            <div className="flex justify-between gap-3 pt-2">
              <button onClick={() => setStep("info")} className="btn-secondary">← Atrás</button>
              <button onClick={() => setStep("quota")} className="btn-primary">Siguiente →</button>
            </div>
          </div>
        )}

        {/* ── Paso 3: Cuota ── */}
        {step === "quota" && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-800 mb-1">Cuota individual</h3>
              <p className="text-xs text-gray-500">Límites de uso para este cliente.</p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="label">Límite mensual *</label>
                <input
                  value={monthly}
                  onChange={e => setMonthly(e.target.value)}
                  className="input"
                  type="number"
                  min="1"
                />
                <p className="text-xs text-gray-400 mt-1">sellos / mes</p>
              </div>
              <div>
                <label className="label">Burst por minuto *</label>
                <input
                  value={burst}
                  onChange={e => setBurst(e.target.value)}
                  className="input"
                  type="number"
                  min="1"
                />
                <p className="text-xs text-gray-400 mt-1">requests / minuto</p>
              </div>
            </div>

            <div className="space-y-3 pt-1">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={hardLimit}
                  onChange={e => setHardLimit(e.target.checked)}
                  className="w-4 h-4 rounded border-gray-300 text-blue-600"
                />
                <div>
                  <span className="text-sm font-medium text-gray-700">Límite estricto</span>
                  <p className="text-xs text-gray-400">Rechaza requests al agotar la cuota mensual</p>
                </div>
              </label>
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={autoSuspend}
                  onChange={e => setAutoSuspend(e.target.checked)}
                  className="w-4 h-4 rounded border-gray-300 text-blue-600"
                />
                <div>
                  <span className="text-sm font-medium text-gray-700">Suspender automáticamente</span>
                  <p className="text-xs text-gray-400">Suspende el cliente al agotar la cuota</p>
                </div>
              </label>
            </div>

            {error && <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>}

            <div className="flex justify-between gap-3 pt-2">
              <button onClick={() => setStep("ip")} className="btn-secondary" disabled={creating}>← Atrás</button>
              <button
                onClick={create}
                disabled={!monthly || !burst || creating}
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
                ) : "Crear cliente ✓"}
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
            <div className="bg-blue-50 border border-blue-200 rounded-xl p-4">
              <p className="text-xs font-bold text-blue-700 uppercase tracking-wide mb-1">
                🖊 URL para software de firma
              </p>
              <p className="text-xs text-blue-600 mb-2">
                Configurá esta URL en DAVISIGN, Adobe, JSignPdf, etc. No requiere usuario ni contraseña.
              </p>
              <div className="bg-white border border-blue-200 rounded-lg p-3 font-mono text-xs text-blue-900 break-all">
                {result.credential.stamp_url}
              </div>
              <button
                onClick={() => navigator.clipboard.writeText(result.credential.stamp_url!)}
                className="w-full mt-2 px-3 py-2 text-sm bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors font-medium"
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
