"use client";

import { useState, useRef, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api, type Tenant, type APICredential } from "@/lib/api";

export default function TenantsPage() {
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [showDelete2FA, setShowDelete2FA] = useState(false);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);

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
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tenants"] });
      setPendingDeleteId(null);
    },
  });

  const handleDeleteClick = (id: string) => {
    setPendingDeleteId(id);
    setShowDelete2FA(true);
  };

  const handleDeleteCancel = () => {
    setShowDelete2FA(false);
    setPendingDeleteId(null);
  };

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
                <th className="text-left px-4 py-3 font-medium text-gray-600">Identificador</th>
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
                  onDelete={() => handleDeleteClick(tenant.id)}
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

      {showDelete2FA && pendingDeleteId && (
        <Delete2FAModal
          tenantId={pendingDeleteId}
          tenantName={data?.data.find(t => t.id === pendingDeleteId)?.name || ""}
          onConfirm={async (totpCode) => {
            try {
              await api.verifyAndDeleteTenant(pendingDeleteId, totpCode);
              setShowDelete2FA(false);
              setPendingDeleteId(null);
              qc.invalidateQueries({ queryKey: ["tenants"] });
            } catch (err) {
              throw err;
            }
          }}
          onCancel={handleDeleteCancel}
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
            onClick={onDelete}
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
  const [initialBundleAmount, setInitialBundleAmount] = useState("1000");
  const [burstPerMinute, setBurstPerMinute] = useState("10");

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

      // 4. Crear bolsa inicial y configurar ritmo
      const bundleAmount = parseInt(initialBundleAmount) || 1000;
      const burst = parseInt(burstPerMinute) || 10;
      await api.addBundle(tenant.id, {
        amount: bundleAmount,
        note: "Bolsa inicial",
      });
      await api.updateQuota(tenant.id, {
        burst_per_minute: burst,
      });

      setResult({ tenant, credential, ips: ipEntries, monthlyLimit: bundleAmount, burstPerMinute: burst });
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

            {/* Configuración de bolsa y ritmo */}
            <div className="border-t pt-4">
              <h3 className="text-sm font-semibold text-gray-800 mb-4">Configuración de bolsa</h3>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="label">Sellos iniciales *</label>
                  <input
                    type="number"
                    value={initialBundleAmount}
                    onChange={e => setInitialBundleAmount(e.target.value)}
                    className="input"
                    placeholder="1000"
                    min="1"
                  />
                  <p className="text-xs text-gray-400 mt-1">cantidad de sellos en la bolsa inicial</p>
                </div>
                <div>
                  <label className="label">Sellos por minuto *</label>
                  <input
                    type="number"
                    value={burstPerMinute}
                    onChange={e => setBurstPerMinute(e.target.value)}
                    className="input"
                    placeholder="10"
                    min="1"
                  />
                  <p className="text-xs text-gray-400 mt-1">máximo por minuto</p>
                </div>
              </div>
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

// ── Modal de verificación 2FA para eliminar ────────────────────

function Delete2FAModal({
  tenantId,
  tenantName,
  isLoading,
  onConfirm,
  onCancel,
}: {
  tenantId: string;
  tenantName: string;
  isLoading: boolean;
  onConfirm: (totpCode: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [totpCode, setTotpCode] = useState("");
  const [error, setError] = useState("");
  const [localLoading, setLocalLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    const cleanCode = totpCode.replace(/\s/g, "");

    // Si hay código ingresado, validar que tenga 6 dígitos
    if (cleanCode && cleanCode.length !== 6) {
      setError("El código debe tener 6 dígitos");
      return;
    }

    setLocalLoading(true);
    try {
      // Enviar código (puede estar vacío si no hay 2FA configurado)
      await onConfirm(cleanCode);
    } catch (err: any) {
      // Prioridad: fields.totp_code > message > error > default
      let errorMsg = "Error al verificar. Intenta de nuevo.";
      if (err?.fields?.totp_code) {
        errorMsg = err.fields.totp_code;
      } else if (err?.message) {
        errorMsg = err.message;
      } else if (err?.error) {
        errorMsg = err.error;
      }
      setError(errorMsg);
      setTotpCode("");
      inputRef.current?.focus();
    } finally {
      setLocalLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-lg max-w-md w-full mx-4 p-6 space-y-4">
        <div>
          <h2 className="text-lg font-bold text-gray-900">Confirmar eliminación</h2>
          <p className="text-sm text-gray-500 mt-1">
            ¿Eliminar permanentemente el cliente <strong>{tenantName}</strong>?
          </p>
          <p className="text-xs text-gray-400 mt-2">
            Ingresa tu código de autenticación de dos factores para proceder.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Código de autenticación
            </label>
            <input
              ref={inputRef}
              type="text"
              value={totpCode}
              onChange={(e) => {
                setTotpCode(e.target.value);
                setError("");
              }}
              placeholder="000000"
              maxLength={6}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-center text-lg font-mono focus:outline-none focus:ring-2 focus:ring-primary-500"
              disabled={isLoading || localLoading}
            />
            {error && <p className="text-xs text-red-600 mt-1">{error}</p>}
          </div>

          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={onCancel}
              disabled={isLoading || localLoading}
              className="flex-1 px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200 disabled:opacity-50"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={isLoading || localLoading || totpCode.length < 6}
              className="flex-1 px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50"
            >
              {isLoading || localLoading ? "Verificando..." : "Eliminar cliente"}
            </button>
          </div>
        </form>

        <p className="text-xs text-gray-400 text-center">
          Esta acción no se puede deshacer. Se eliminarán todas las credenciales y configuraciones.
        </p>
      </div>
    </div>
  );
}
