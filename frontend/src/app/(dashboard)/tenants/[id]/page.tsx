"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api, BasicAuthCredential, AlertEmail } from "@/lib/api";
import { formatDateTime, formatDateOnly } from "@/lib/dateFormat";

type Tab = "credentials" | "ip-allowlist" | "quota" | "basic-auth" | "noauth" | "alerts";

export default function TenantDetailPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>("credentials");
  const [showNewApiKey, setShowNewApiKey] = useState<string | null>(null);
  const [showStampURL, setShowStampURL] = useState<string | null>(null);
  const [newCredName, setNewCredName] = useState("");
  const [newCIDR, setNewCIDR] = useState("");
  const [newCIDRLabel, setNewCIDRLabel] = useState("");
  const [error, setError] = useState("");
  const [newBAUsername, setNewBAUsername] = useState("");
  const [newBAName, setNewBAName] = useState("");
  const [showNewBAResult, setShowNewBAResult] = useState<BasicAuthCredential | null>(null);
  const [noauthName, setNoauthName] = useState("");

  const { data: tenant, isLoading } = useQuery({
    queryKey: ["tenant", id],
    queryFn: () => api.getTenant(id),
    refetchInterval: 4000,
  });

  const { data: creds } = useQuery({
    queryKey: ["credentials", id],
    queryFn: () => api.listCredentials(id),
    enabled: tab === "credentials",
    refetchInterval: 4000,
  });

  const { data: ips } = useQuery({
    queryKey: ["ip-allowlist", id],
    queryFn: () => api.listIPAllowlist(id),
    enabled: tab === "ip-allowlist",
    refetchInterval: 4000,
  });

  const { data: quota } = useQuery({
    queryKey: ["quota", id],
    queryFn: () => api.getQuota(id),
    enabled: tab === "quota",
    refetchInterval: 4000,
  });

  const { data: basicAuths } = useQuery({
    queryKey: ["basic-auth", id],
    queryFn: () => api.listBasicAuth(id),
    enabled: tab === "basic-auth",
    refetchInterval: 4000,
  });

  const { data: noauthData, isLoading: noauthLoading } = useQuery({
    queryKey: ["noauth", id],
    queryFn: () => api.getNoAuth(id),
    enabled: tab === "noauth",
    refetchInterval: 4000,
  });

  const { data: alertEmails } = useQuery({
    queryKey: ["alert-emails", id],
    queryFn: () => api.listAlertEmails(id),
    enabled: tab === "alerts",
  });

  const [newAlertEmail, setNewAlertEmail] = useState("");
  const [newAlertLabel, setNewAlertLabel] = useState("");
  const [editContactEmail, setEditContactEmail] = useState<string | null>(null);
  const [alertSuccess, setAlertSuccess] = useState("");
  const [testResult, setTestResult] = useState<string[] | null>(null);

  const createCredMut = useMutation({
    mutationFn: () => api.createCredential(id, { name: newCredName || undefined }),
    onSuccess: (data) => {
      setShowNewApiKey(data.api_key ?? null);
      setShowStampURL(data.stamp_url ?? null);
      setNewCredName("");
      qc.invalidateQueries({ queryKey: ["credentials", id] });
    },
    onError: () => setError("Error al crear la credencial"),
  });

  const revokeCredMut = useMutation({
    mutationFn: (credId: string) => api.revokeCredential(credId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["credentials", id] }),
  });

  const rotateCredMut = useMutation({
    mutationFn: (credId: string) => api.rotateCredential(credId),
    onSuccess: (data) => {
      setShowNewApiKey(data.api_key ?? null);
      setShowStampURL(data.stamp_url ?? null);
      qc.invalidateQueries({ queryKey: ["credentials", id] });
    },
  });

  const createIPMut = useMutation({
    mutationFn: () => api.createIPAllowlist(id, { cidr: newCIDR, label: newCIDRLabel || undefined }),
    onSuccess: () => {
      setNewCIDR(""); setNewCIDRLabel("");
      qc.invalidateQueries({ queryKey: ["ip-allowlist", id] });
    },
    onError: () => setError("CIDR inválido o ya existe"),
  });

  const deleteIPMut = useMutation({
    mutationFn: (entryId: string) => api.deleteIPAllowlist(entryId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ip-allowlist", id] }),
  });

  const createBAMut = useMutation({
    mutationFn: () => api.createBasicAuth(id, { username: newBAUsername, name: newBAName || undefined }),
    onSuccess: (data) => {
      setShowNewBAResult(data);
      setNewBAUsername("");
      setNewBAName("");
      qc.invalidateQueries({ queryKey: ["basic-auth", id] });
    },
    onError: () => setError("Error al crear credencial Basic Auth. ¿El usuario ya existe?"),
  });

  const revokeBAMut = useMutation({
    mutationFn: (credId: string) => api.revokeBasicAuth(credId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["basic-auth", id] }),
  });

  const enableNoAuthMut = useMutation({
    mutationFn: () => api.enableNoAuth(id, noauthName || undefined),
    onSuccess: () => {
      setNoauthName("");
      qc.invalidateQueries({ queryKey: ["noauth", id] });
    },
    onError: () => setError("Error al habilitar acceso TSP"),
  });

  const disableNoAuthMut = useMutation({
    mutationFn: () => api.disableNoAuth(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["noauth", id] }),
    onError: () => setError("Error al deshabilitar acceso TSP"),
  });

  const deleteNoAuthMut = useMutation({
    mutationFn: () => api.deleteNoAuth(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["noauth", id] }),
    onError: () => setError("Error al eliminar acceso TSP"),
  });

  const addAlertEmailMut = useMutation({
    mutationFn: () => api.addAlertEmail(id, { email: newAlertEmail, label: newAlertLabel || undefined }),
    onSuccess: () => {
      setNewAlertEmail("");
      setNewAlertLabel("");
      qc.invalidateQueries({ queryKey: ["alert-emails", id] });
    },
    onError: () => setError("Error al agregar correo. ¿Ya existe?"),
  });

  const deleteAlertEmailMut = useMutation({
    mutationFn: (emailId: string) => api.deleteAlertEmail(id, emailId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alert-emails", id] }),
  });

  const updateContactEmailMut = useMutation({
    mutationFn: () => api.updateTenant(id, {
      name: tenant?.name ?? "",
      description: tenant?.description,
      contact_email: editContactEmail !== null ? (editContactEmail || undefined) : (tenant?.contact_email || undefined),
    }),
    onSuccess: () => {
      setEditContactEmail(null);
      setAlertSuccess("Correo de contacto actualizado");
      setTimeout(() => setAlertSuccess(""), 3000);
      qc.invalidateQueries({ queryKey: ["tenant", id] });
    },
    onError: () => setError("Error al actualizar el correo de contacto"),
  });

  const testAlertMut = useMutation({
    mutationFn: () => api.testAlertEmails(id),
    onSuccess: (data) => {
      setTestResult(data.sent_to);
      setTimeout(() => setTestResult(null), 6000);
    },
    onError: () => setError("Error al enviar correo de prueba. Verifica la configuración de Brevo."),
  });

  if (isLoading) return <div className="text-gray-400 py-8 text-center">Cargando...</div>;
  if (!tenant) return <div className="text-red-600">Cliente no encontrado</div>;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <Link href="/tenants" className="text-sm text-gray-400 hover:text-gray-600">← Clientes</Link>
          </div>
          <h1 className="text-2xl font-bold text-gray-900">{tenant.name}</h1>
          <p className="text-sm text-gray-500 font-mono">{tenant.slug}</p>
        </div>
        <span className={`badge ${tenant.status === "active" ? "badge-green" : "badge-yellow"}`}>
          {tenant.status === "active" ? "Activo" : "Suspendido"}
        </span>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <div className="flex gap-6">
          {(["credentials", "basic-auth", "noauth", "ip-allowlist", "quota", "alerts"] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`pb-3 text-sm font-medium border-b-2 transition-colors ${
                tab === t
                  ? "border-primary-500 text-primary-600"
                  : "border-transparent text-gray-500 hover:text-gray-700"
              }`}
            >
              {t === "credentials"  ? "Credenciales API" :
               t === "basic-auth"   ? "Acceso TSA Privado" :
               t === "noauth"       ? "Acceso TSP" :
               t === "ip-allowlist" ? "IP Allowlist" :
               t === "quota"        ? "Bolsa Contratada" : "Alertas"}
            </button>
          ))}
        </div>
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded p-3">
          {error}
          <button onClick={() => setError("")} className="ml-3 text-red-400 hover:text-red-600">✕</button>
        </div>
      )}

      {/* API Key + URL de firma Modal */}
      {showNewApiKey && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="card w-full max-w-xl p-6 mx-4">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 bg-green-100 rounded-full flex items-center justify-center">
                <svg className="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <div>
                <h2 className="text-lg font-semibold text-gray-900">Credencial creada</h2>
                <p className="text-sm text-red-600 font-medium">⚠ Guarda esta información ahora. La API Key no se podrá ver nuevamente.</p>
              </div>
            </div>

            {/* API Key */}
            <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">API Key (para integración de sistemas)</p>
            <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-green-400 break-all">
              {showNewApiKey}
            </div>
            <button
              onClick={() => { navigator.clipboard.writeText(showNewApiKey); }}
              className="btn-secondary w-full mt-2 text-sm"
            >
              Copiar API Key
            </button>

            {/* URL de firma */}
            {showStampURL && (
              <div className="mt-4">
                <p className="text-xs font-semibold text-primary-700 uppercase tracking-wide mb-1">
                  🖊 URL para software de firma (DAVISIGN, Adobe, JSignPdf, etc.)
                </p>
                <p className="text-xs text-gray-500 mb-2">
                  Configura esta URL en el campo "URL de la TSA". No necesitas usuario ni contraseña.
                </p>
                <div className="bg-primary-50 border border-primary-200 rounded-lg p-3 font-mono text-xs text-primary-800 break-all">
                  {showStampURL}
                </div>
                <button
                  onClick={() => { navigator.clipboard.writeText(showStampURL); }}
                  className="w-full mt-2 px-3 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
                >
                  Copiar URL de firma
                </button>
              </div>
            )}

            <button
              onClick={() => { setShowNewApiKey(null); setShowStampURL(null); }}
              className="btn-primary w-full mt-3"
            >
              Entendido, ya guardé esta información
            </button>
          </div>
        </div>
      )}

      {/* Basic Auth result modal */}
      {showNewBAResult && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="card w-full max-w-xl p-6 mx-4">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 bg-green-100 rounded-full flex items-center justify-center">
                <svg className="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <div>
                <h2 className="text-lg font-semibold text-gray-900">Credencial Basic Auth creada</h2>
                <p className="text-sm text-red-600 font-medium">Guarda esta contraseña ahora. No se podrá recuperar.</p>
              </div>
            </div>

            <div className="space-y-3">
              <div>
                <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">URL del endpoint TSA</p>
                <div className="bg-primary-50 border border-primary-200 rounded-lg p-3 font-mono text-xs text-primary-800 break-all">
                  {showNewBAResult.tsa_endpoint}
                </div>
                <button
                  onClick={() => { if (showNewBAResult.tsa_endpoint) navigator.clipboard.writeText(showNewBAResult.tsa_endpoint!); }}
                  className="w-full mt-2 px-3 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
                >
                  Copiar URL
                </button>
              </div>

              <div>
                <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Usuario</p>
                <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-green-400">
                  {showNewBAResult.username}
                </div>
              </div>

              <div>
                <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Contraseña (solo visible ahora)</p>
                <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-yellow-400 break-all">
                  {showNewBAResult.password}
                </div>
                <button
                  onClick={() => { if (showNewBAResult.password) navigator.clipboard.writeText(showNewBAResult.password!); }}
                  className="btn-secondary w-full mt-2 text-sm"
                >
                  Copiar contraseña
                </button>
              </div>

              <p className="text-xs text-gray-500 bg-gray-50 border rounded p-2">
                Configura esta URL, usuario y contraseña en el campo de TSA de tu software de firma (Adobe Reader, DAVISIGN, JSignPdf).
              </p>
            </div>

            <button
              onClick={() => setShowNewBAResult(null)}
              className="btn-primary w-full mt-4"
            >
              Entendido, ya guardé esta información
            </button>
          </div>
        </div>
      )}

      {/* Tab: Credenciales */}
      {tab === "credentials" && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <input
              value={newCredName}
              onChange={(e) => setNewCredName(e.target.value)}
              className="input flex-1"
              placeholder="Nombre de la credencial (ej: Producción 2025)"
            />
            <button onClick={() => createCredMut.mutate()} className="btn-primary">
              {createCredMut.isPending ? "Creando..." : "+ Crear credencial"}
            </button>
          </div>

          <div className="card overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Clave</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Nombre</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">URL de firma</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Estado</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Último uso</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600">Acciones</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {creds?.data.map((c) => (
                  <tr key={c.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-mono text-xs text-gray-600">{c.key_prefix}</td>
                    <td className="px-4 py-3 text-gray-700">{c.name ?? <span className="text-gray-400">Sin nombre</span>}</td>
                    <td className="px-4 py-3">
                      {c.url_token && c.status === "active" && creds?.tsa_endpoint ? (
                        <div className="flex items-center gap-1">
                          <span className="font-mono text-xs text-primary-700 bg-primary-50 border border-primary-100 rounded px-2 py-1 max-w-[180px] truncate" title={`${creds.tsa_endpoint}/${c.url_token}`}>
                            /ts/{c.url_token.slice(0,8)}…
                          </span>
                          <button
                            onClick={() => navigator.clipboard.writeText(`${creds.tsa_endpoint}/${c.url_token}`)}
                            title="Copiar URL de firma"
                            className="text-primary-400 hover:text-primary-600"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                            </svg>
                          </button>
                        </div>
                      ) : <span className="text-gray-300 text-xs">—</span>}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`badge ${c.status === "active" ? "badge-green" : "badge-red"}`}>
                        {c.status === "active" ? "Activa" : c.status === "revoked" ? "Revocada" : "Expirada"}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs">
                      {c.last_used_at
                        ? formatDateTime(c.last_used_at)
                        : "Nunca usada"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {c.status === "active" && (
                        <div className="flex justify-end gap-2">
                          <button
                            onClick={() => rotateCredMut.mutate(c.id)}
                            className="btn-secondary px-3 py-1 text-xs"
                          >
                            Rotar
                          </button>
                          <button
                            onClick={() => { if (confirm("¿Revocar esta credencial?")) revokeCredMut.mutate(c.id); }}
                            className="btn px-3 py-1 text-xs bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
                          >
                            Revocar
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tab: Acceso TSA Privado (Basic Auth) */}
      {tab === "basic-auth" && (
        <div className="space-y-4">
          <div className="bg-primary-50 border border-primary-200 rounded-lg p-4 text-sm text-primary-800">
            <strong>Acceso TSA Privado con HTTP Basic Auth</strong><br />
            Cada cliente obtiene su propio usuario y contraseña para la URL <code className="bg-primary-100 px-1 rounded">POST {basicAuths?.tsa_endpoint || "https://tsa.bigdavi.com/ts"}</code>.
            Compatible con Adobe Reader, DAVISIGN y JSignPdf.
          </div>

          <div className="flex gap-3">
            <input
              value={newBAUsername}
              onChange={(e) => setNewBAUsername(e.target.value)}
              className="input flex-1"
              placeholder="Usuario (ej: acme-prod)"
            />
            <input
              value={newBAName}
              onChange={(e) => setNewBAName(e.target.value)}
              className="input flex-1"
              placeholder="Descripción (opcional)"
            />
            <button
              onClick={() => createBAMut.mutate()}
              disabled={createBAMut.isPending || !newBAUsername}
              className="btn-primary"
            >
              {createBAMut.isPending ? "Creando..." : "+ Crear acceso"}
            </button>
          </div>

          <div className="card overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Usuario</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Contraseña (prefijo)</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Descripción</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Estado</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600">Creado</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600">Acciones</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {basicAuths?.data?.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-6 text-center text-gray-400 text-sm">
                      Sin credenciales Basic Auth. Crea la primera arriba.
                    </td>
                  </tr>
                )}
                {basicAuths?.data?.map((c) => (
                  <tr key={c.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-mono text-sm text-gray-800">{c.username}</td>
                    <td className="px-4 py-3 font-mono text-xs text-gray-500">{c.key_prefix}</td>
                    <td className="px-4 py-3 text-gray-600">{c.name ?? <span className="text-gray-400">—</span>}</td>
                    <td className="px-4 py-3">
                      <span className={`badge ${c.status === "active" ? "badge-green" : "badge-red"}`}>
                        {c.status === "active" ? "Activa" : "Revocada"}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs">
                      {formatDateTime(c.created_at)}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {c.status === "active" && (
                        <button
                          onClick={() => { if (confirm(`¿Revocar acceso de "${c.username}"?`)) revokeBAMut.mutate(c.id); }}
                          className="btn px-3 py-1 text-xs bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
                        >
                          Revocar
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tab: IP Allowlist */}
      {tab === "ip-allowlist" && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <input
              value={newCIDR}
              onChange={(e) => setNewCIDR(e.target.value)}
              className="input flex-1"
              placeholder="IP o CIDR (ej: 203.0.113.1 o 10.0.0.0/24)"
            />
            <input
              value={newCIDRLabel}
              onChange={(e) => setNewCIDRLabel(e.target.value)}
              className="input flex-1"
              placeholder="Etiqueta (opcional)"
            />
            <button onClick={() => createIPMut.mutate()} className="btn-primary">
              + Agregar
            </button>
          </div>

          {ips?.data.length === 0 && (
            <div className="text-sm text-gray-400 bg-primary-50 border border-primary-100 rounded p-3">
              Sin restricciones de IP configuradas — se permite cualquier IP.
            </div>
          )}

          <div className="space-y-2">
            {ips?.data.filter((e) => e.is_active).map((entry) => (
              <div key={entry.id} className="card flex items-center justify-between px-4 py-3">
                <div>
                  <span className="font-mono text-sm text-gray-800">{entry.cidr}</span>
                  {entry.label && <span className="ml-3 text-xs text-gray-500">{entry.label}</span>}
                </div>
                <button
                  onClick={() => { if (confirm("¿Eliminar esta IP?")) deleteIPMut.mutate(entry.id); }}
                  className="text-red-400 hover:text-red-600 text-sm"
                >
                  Eliminar
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Tab: Acceso TSP (sin credenciales) */}
      {tab === "noauth" && (
        <div className="space-y-5">
          {/* Info box */}
          <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 text-sm text-amber-900">
            <p className="font-semibold mb-1">Acceso TSP sin credenciales (por IP)</p>
            <p>
              Permite que clientes como <strong>DAVISIGN</strong> con EU DSS envíen requests al endpoint{" "}
              <code className="bg-amber-100 px-1 rounded font-mono">POST {basicAuths?.tsa_endpoint?.replace("/ts", "/tsp") || "https://tsa.bigdavi.com/tsp"}</code> sin
              usuario ni contraseña. La autenticación se realiza validando la <strong>IP de origen</strong> contra
              el IP Allowlist del cliente.
            </p>
            <p className="mt-2 text-amber-700">
              ⚠ <strong>Por defecto BLOQUEADO</strong> — aunque esté habilitado, si no hay IPs configuradas en
              el Allowlist, todos los requests serán rechazados.
            </p>
          </div>

          {noauthLoading ? (
            <div className="text-gray-400 py-4 text-center text-sm">Cargando...</div>
          ) : noauthData?.enabled ? (
            /* ── Acceso habilitado ─────────────────────────── */
            <div className="space-y-4">
              <div className="card p-5">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <span className={`badge ${noauthData.access?.status === "active" ? "badge-green" : "badge-yellow"}`}>
                      {noauthData.access?.status === "active" ? "Activo" : "Suspendido"}
                    </span>
                    <span className="text-sm text-gray-700 font-medium">
                      {noauthData.access?.name ?? "Acceso TSP sin credenciales"}
                    </span>
                  </div>
                  <div className="flex gap-2">
                    {noauthData.access?.status === "active" ? (
                      <button
                        onClick={() => { if (confirm("¿Suspender el acceso TSP sin credenciales?")) disableNoAuthMut.mutate(); }}
                        disabled={disableNoAuthMut.isPending}
                        className="btn px-3 py-1.5 text-xs bg-yellow-50 text-yellow-700 border border-yellow-200 hover:bg-yellow-100"
                      >
                        {disableNoAuthMut.isPending ? "Suspendiendo..." : "Suspender"}
                      </button>
                    ) : (
                      <button
                        onClick={() => enableNoAuthMut.mutate()}
                        disabled={enableNoAuthMut.isPending}
                        className="btn-primary px-3 py-1.5 text-xs"
                      >
                        {enableNoAuthMut.isPending ? "Activando..." : "Reactivar"}
                      </button>
                    )}
                    <button
                      onClick={() => { if (confirm("¿Eliminar definitivamente el acceso TSP? Esta acción no se puede deshacer.")) deleteNoAuthMut.mutate(); }}
                      disabled={deleteNoAuthMut.isPending}
                      className="btn px-3 py-1.5 text-xs bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
                    >
                      Eliminar
                    </button>
                  </div>
                </div>

                {/* URL del endpoint */}
                <div>
                  <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">
                    URL para configurar en DAVISIGN / EU DSS
                  </p>
                  <div className="flex items-center gap-2">
                    <div className="flex-1 bg-primary-50 border border-primary-200 rounded-lg px-3 py-2 font-mono text-sm text-primary-800">
                      {basicAuths?.tsa_endpoint?.replace("/ts", "/tsp") || "https://tsa.bigdavi.com/tsp"}
                    </div>
                    <button
                      onClick={() => navigator.clipboard.writeText(basicAuths?.tsa_endpoint?.replace("/ts", "/tsp") || "https://tsa.bigdavi.com/tsp")}
                      className="px-3 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
                    >
                      Copiar
                    </button>
                  </div>
                  <p className="text-xs text-gray-400 mt-1">
                    En DAVISIGN: <em>Configuración → TSA → URL del servicio de sellado</em>. No ingreses usuario ni contraseña.
                  </p>
                </div>
              </div>

              {/* Recordatorio IP Allowlist */}
              <div className="bg-primary-50 border border-primary-100 rounded-lg p-4 text-sm text-primary-800">
                <p className="font-medium mb-1">Paso obligatorio: agregar la IP del cliente</p>
                <p>
                  Dirígete a la pestaña <strong>IP Allowlist</strong> y agrega la IP pública del servidor o
                  máquina que ejecuta DAVISIGN. Sin esa IP, los requests serán rechazados aunque el acceso
                  esté habilitado.
                </p>
              </div>
            </div>
          ) : (
            /* ── Acceso no habilitado ──────────────────────── */
            <div className="card p-6 max-w-lg">
              <h3 className="text-base font-semibold text-gray-900 mb-1">Habilitar acceso TSP sin credenciales</h3>
              <p className="text-sm text-gray-500 mb-4">
                Una vez habilitado, el cliente podrá enviar requests a{" "}
                <code className="font-mono text-xs bg-gray-100 px-1 rounded">{basicAuths?.tsa_endpoint?.replace("/ts", "/tsp") || "https://tsa.bigdavi.com/tsp"}</code>{" "}
                sin usuario ni contraseña. Deberás agregar su IP en el Allowlist para que funcione.
              </p>
              <div className="flex gap-3">
                <input
                  value={noauthName}
                  onChange={(e) => setNoauthName(e.target.value)}
                  className="input flex-1"
                  placeholder="Descripción opcional (ej: DAVISIGN producción)"
                />
                <button
                  onClick={() => enableNoAuthMut.mutate()}
                  disabled={enableNoAuthMut.isPending}
                  className="btn-primary"
                >
                  {enableNoAuthMut.isPending ? "Habilitando..." : "Habilitar"}
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Tab: Cuota */}
      {tab === "quota" && quota && (
        <QuotaForm tenantId={id} initialQuota={quota} />
      )}

      {/* Tab: Alertas */}
      {tab === "alerts" && (
        <div className="space-y-6">
          {alertSuccess && (
            <div className="text-sm text-green-700 bg-green-50 border border-green-200 rounded p-3">
              ✓ {alertSuccess}
            </div>
          )}
          {testResult && (
            <div className="text-sm text-blue-700 bg-blue-50 border border-blue-200 rounded p-3">
              <strong>Correo de prueba enviado a:</strong>
              <ul className="mt-1 list-disc list-inside">
                {testResult.map((addr) => <li key={addr}>{addr}</li>)}
              </ul>
            </div>
          )}

          {/* Correo de contacto principal */}
          <div className="card p-6 space-y-4">
            <div>
              <h2 className="text-base font-semibold text-gray-900">Correo de contacto principal</h2>
              <p className="text-sm text-gray-500 mt-1">
                Este es el correo registrado del cliente. Recibirá todas las alertas de consumo de bolsa.
              </p>
            </div>
            <div className="flex gap-3 items-center">
              <input
                type="email"
                value={editContactEmail !== null ? editContactEmail : (tenant.contact_email ?? "")}
                onChange={(e) => setEditContactEmail(e.target.value)}
                className="input flex-1"
                placeholder="correo@ejemplo.com"
              />
              <button
                onClick={() => updateContactEmailMut.mutate()}
                disabled={updateContactEmailMut.isPending}
                className="btn-primary whitespace-nowrap"
              >
                {updateContactEmailMut.isPending ? "Guardando..." : "Guardar"}
              </button>
            </div>
          </div>

          {/* Correos adicionales */}
          <div className="card p-6 space-y-4">
            <div>
              <h2 className="text-base font-semibold text-gray-900">Correos adicionales</h2>
              <p className="text-sm text-gray-500 mt-1">
                Agrega destinatarios extra que también recibirán las alertas.
              </p>
            </div>

            <div className="flex gap-3">
              <input
                type="email"
                value={newAlertEmail}
                onChange={(e) => setNewAlertEmail(e.target.value)}
                className="input flex-1"
                placeholder="correo@ejemplo.com"
              />
              <input
                type="text"
                value={newAlertLabel}
                onChange={(e) => setNewAlertLabel(e.target.value)}
                className="input w-40"
                placeholder="Etiqueta (opcional)"
              />
              <button
                onClick={() => addAlertEmailMut.mutate()}
                disabled={addAlertEmailMut.isPending || !newAlertEmail}
                className="btn-primary whitespace-nowrap"
              >
                {addAlertEmailMut.isPending ? "Agregando..." : "+ Agregar"}
              </button>
            </div>

            {(!alertEmails || alertEmails.length === 0) ? (
              <p className="text-sm text-gray-400 bg-gray-50 border border-gray-100 rounded p-3">
                Sin correos adicionales. Las alertas solo llegarán al correo de contacto principal.
              </p>
            ) : (
              <div className="divide-y divide-gray-100 border rounded-lg overflow-hidden">
                {alertEmails.map((ae: AlertEmail) => (
                  <div key={ae.id} className="flex items-center justify-between px-4 py-3 bg-white hover:bg-gray-50">
                    <div>
                      <span className="text-sm text-gray-800">{ae.email}</span>
                      {ae.label && <span className="ml-3 text-xs text-gray-400">{ae.label}</span>}
                    </div>
                    <button
                      onClick={() => { if (confirm(`¿Eliminar ${ae.email}?`)) deleteAlertEmailMut.mutate(ae.id); }}
                      className="text-red-400 hover:text-red-600 text-sm"
                    >
                      Eliminar
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Prueba de envío */}
          <div className="card p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-1">Probar envío de alertas</h2>
            <p className="text-sm text-gray-500 mb-4">
              Envía un correo de prueba a todos los destinatarios configurados para verificar que las alertas llegan correctamente.
            </p>
            <button
              onClick={() => testAlertMut.mutate()}
              disabled={testAlertMut.isPending}
              className="btn-primary"
            >
              {testAlertMut.isPending ? "Enviando..." : "Enviar correo de prueba"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function QuotaForm({ tenantId, initialQuota }: {
  tenantId: string;
  initialQuota: { burst_per_minute: number };
}) {
  const qc = useQueryClient();
  const [burst, setBurst] = useState(String(initialQuota.burst_per_minute));
  const [bundles, setBundles] = useState<any[]>([]);
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [alertThreshold, setAlertThreshold] = useState("");
  const [showModal, setShowModal] = useState(false);
  const [saved, setSaved] = useState(false);
  // edición inline de umbral de alerta: bundleId → valor en edición
  const [editingAlert, setEditingAlert] = useState<Record<string, string>>({});

  useEffect(() => {
    api.listBundles(tenantId).then(setBundles);
  }, [tenantId]);

  const refreshBundles = () => api.listBundles(tenantId).then(setBundles);

  const updateBurst = useMutation({
    mutationFn: () => api.updateQuota(tenantId, { burst_per_minute: parseInt(burst) }),
    onSuccess: () => {
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
      qc.invalidateQueries({ queryKey: ["quota", tenantId] });
    },
  });

  const addBundle = useMutation({
    mutationFn: () => api.addBundle(tenantId, {
      amount: parseInt(amount),
      note: note || undefined,
      alert_threshold_percent: alertThreshold ? parseInt(alertThreshold) : undefined,
    }),
    onSuccess: () => {
      setShowModal(false);
      setAmount(""); setNote(""); setAlertThreshold("");
      refreshBundles();
      qc.invalidateQueries({ queryKey: ["quota", tenantId] });
    },
  });

  const updateAlertMut = useMutation({
    mutationFn: ({ bundleId, value }: { bundleId: string; value: string }) =>
      api.updateBundleAlert(tenantId, bundleId, value ? parseInt(value) : null),
    onSuccess: (_, { bundleId }) => {
      setEditingAlert((prev) => { const n = { ...prev }; delete n[bundleId]; return n; });
      refreshBundles();
    },
  });

  const contracted = bundles.reduce((sum, b) => sum + b.amount, 0);

  return (
    <div className="space-y-6">
      <div className="card p-6 space-y-4">
        <h2 className="text-base font-semibold text-gray-900">Bolsa de sellos</h2>
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <p className="text-2xl font-bold text-blue-900">{contracted.toLocaleString()}</p>
          <p className="text-sm text-blue-700">sellos contratados en total</p>
        </div>
        <button onClick={() => setShowModal(true)} className="btn-primary">
          Añadir bolsa
        </button>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg p-6 max-w-sm w-full space-y-3">
            <h3 className="text-lg font-semibold">Nueva bolsa</h3>
            <div>
              <label className="label">Cantidad de sellos *</label>
              <input type="number" placeholder="1000" value={amount}
                onChange={(e) => setAmount(e.target.value)} className="input w-full" min="1" />
            </div>
            <div>
              <label className="label">Referencia</label>
              <input type="text" placeholder="Nº factura, descripción…" value={note}
                onChange={(e) => setNote(e.target.value)} className="input w-full" />
            </div>
            <div>
              <label className="label">Alerta de consumo (%)</label>
              <input type="number" placeholder="80" value={alertThreshold}
                onChange={(e) => setAlertThreshold(e.target.value)}
                className="input w-full" min="1" max="99" />
              <p className="text-xs text-gray-400 mt-1">
                Envía correo cuando se consuma este % de la bolsa (opcional)
              </p>
            </div>
            <div className="flex gap-3 pt-1">
              <button onClick={() => setShowModal(false)} className="btn flex-1">Cancelar</button>
              <button onClick={() => addBundle.mutate()} disabled={!amount || addBundle.isPending}
                className="btn-primary flex-1">
                {addBundle.isPending ? "Guardando..." : "Guardar"}
              </button>
            </div>
          </div>
        </div>
      )}

      {bundles.length > 0 && (
        <div className="card p-6">
          <h3 className="text-base font-semibold mb-4">Histórico de bolsas</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="border-b">
                <tr>
                  <th className="text-left py-3 pr-4">Fecha</th>
                  <th className="text-right py-3 px-4">Cantidad</th>
                  <th className="text-left py-3 pl-4">Referencia</th>
                  <th className="text-center py-3 px-4">Alerta (%)</th>
                </tr>
              </thead>
              <tbody>
                {bundles.map((b) => {
                  const isEditing = b.id in editingAlert;
                  const editVal = editingAlert[b.id] ?? "";
                  return (
                    <tr key={b.id} className="border-b hover:bg-gray-50">
                      <td className="py-3 pr-4">{formatDateOnly(b.contracted_at)}</td>
                      <td className="text-right font-medium py-3 px-4">{b.amount.toLocaleString()}</td>
                      <td className="text-gray-600 py-3 pl-4">{b.note || "—"}</td>
                      <td className="py-2 px-4">
                        {isEditing ? (
                          <div className="flex items-center gap-1 justify-center">
                            <input
                              type="number" min="1" max="99"
                              value={editVal}
                              onChange={(e) => setEditingAlert((p) => ({ ...p, [b.id]: e.target.value }))}
                              className="input w-16 py-1 text-center text-sm"
                              placeholder="1-99"
                            />
                            <button
                              onClick={() => updateAlertMut.mutate({ bundleId: b.id, value: editVal })}
                              disabled={updateAlertMut.isPending}
                              className="btn-primary px-2 py-1 text-xs"
                            >✓</button>
                            <button
                              onClick={() => setEditingAlert((p) => { const n = { ...p }; delete n[b.id]; return n; })}
                              className="btn px-2 py-1 text-xs text-gray-500"
                            >✕</button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-2 justify-center">
                            {b.alert_threshold_percent
                              ? <span className="badge badge-yellow">{b.alert_threshold_percent}%</span>
                              : <span className="text-gray-300 text-xs">—</span>}
                            <button
                              onClick={() => setEditingAlert((p) => ({ ...p, [b.id]: String(b.alert_threshold_percent ?? "") }))}
                              title="Editar umbral"
                              className="text-gray-400 hover:text-primary-600 text-xs"
                            >✎</button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="card p-6 space-y-4">
        <h3 className="text-base font-semibold">Límite de ritmo</h3>
        <div>
          <label className="label">Sellos por minuto</label>
          <input value={burst} onChange={(e) => setBurst(e.target.value)} className="input" type="number" min="1" />
          <p className="text-xs text-gray-400 mt-1">máximo de requests por minuto</p>
        </div>
        <button onClick={() => updateBurst.mutate()} disabled={updateBurst.isPending} className="btn-primary">
          {updateBurst.isPending ? "Guardando..." : "Guardar"}
        </button>
        {saved && <span className="text-sm text-green-600">✓ Guardado</span>}
      </div>
    </div>
  );
}
