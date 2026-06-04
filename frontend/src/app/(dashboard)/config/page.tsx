"use client";

import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { api, type TSAUpstream } from "@/lib/api";

export default function ConfigPage() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const { data: allUpstreams = [], isLoading } = useQuery({
    queryKey: ["upstreams"],
    queryFn: () => api.listUpstreams().then(res => res.data),
  });

  const upstreams = allUpstreams.filter(u => u.is_active);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Configuración</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Gestiona las TSA Externas y la seguridad de tu cuenta
          </p>
        </div>
        <button onClick={() => { setEditingId(null); setShowCreate(true); }} className="btn-primary">
          + Nueva TSA Externa
        </button>
      </div>

      {/* TSA Upstreams */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">TSA Externas (Timestamping Authorities)</h2>

        {isLoading ? (
          <p className="text-center text-gray-400 py-8">Cargando...</p>
        ) : upstreams.length === 0 ? (
          <div className="text-center py-8">
            <p className="text-gray-400 mb-3">No hay TSAs configuradas</p>
            <button
              onClick={() => { setEditingId(null); setShowCreate(true); }}
              className="btn-secondary text-sm"
            >
              Agregar TSA
            </button>
          </div>
        ) : (
          <div className="space-y-3">
            {upstreams.map((upstream) => (
              <UpstreamCard
                key={upstream.id}
                upstream={upstream}
                onEdit={() => { setEditingId(upstream.id); setShowCreate(true); }}
                onDelete={() => qc.invalidateQueries({ queryKey: ["upstreams"] })}
                onSetDefault={() => qc.invalidateQueries({ queryKey: ["upstreams"] })}
              />
            ))}
          </div>
        )}
      </div>

      {/* Seguridad — 2FA */}
      <TwoFactorSection />

      {showCreate && (
        <UpstreamModal
          upstreamId={editingId}
          onClose={() => { setShowCreate(false); setEditingId(null); }}
          onSuccess={() => {
            setShowCreate(false);
            setEditingId(null);
            qc.invalidateQueries({ queryKey: ["upstreams"] });
          }}
        />
      )}
    </div>
  );
}

/* ── Sección de 2FA ──────────────────────────────────────────── */

function TwoFactorSection() {
  const [phase, setPhase] = useState<"idle" | "setup" | "disable">("idle");
  const [qrDataUrl, setQrDataUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const canvasRef = useRef<HTMLCanvasElement>(null);

  const { data: setup2fa, refetch: fetchSetup } = useQuery({
    queryKey: ["2fa-setup"],
    queryFn: () => api.setup2FA(),
    enabled: false,
  });

  const { data: me, refetch: refetchMe } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.me(),
  });

  const enableMut = useMutation({
    mutationFn: () => api.enable2FA(code),
    onSuccess: () => {
      setSuccess("Autenticación en dos pasos activada correctamente.");
      setPhase("idle");
      setCode("");
      refetchMe();
    },
    onError: () => setError("Código incorrecto. Verifica tu aplicación."),
  });

  const disableMut = useMutation({
    mutationFn: () => api.disable2FA(code),
    onSuccess: () => {
      setSuccess("Autenticación en dos pasos desactivada.");
      setPhase("idle");
      setCode("");
      refetchMe();
    },
    onError: () => setError("Código incorrecto. Verifica tu aplicación."),
  });

  async function startSetup() {
    setError(""); setSuccess("");
    const result = await fetchSetup();
    if (result.data) {
      setSecret(result.data.secret);
      // Generar QR como data URL
      try {
        const url = await QRCode.toDataURL(result.data.qr_url, { width: 200, margin: 1 });
        setQrDataUrl(url);
      } catch {
        setQrDataUrl("");
      }
      setPhase("setup");
    }
  }

  const totpEnabled = (me as any)?.totp_enabled ?? false;

  return (
    <div className="card p-6">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Seguridad — 2FA</h2>
          <p className="text-sm text-gray-500">
            Autenticación en dos pasos con aplicación TOTP (Google Authenticator, Authy, etc.)
          </p>
        </div>
        <span className={`badge ${totpEnabled ? "badge-green" : "badge-gray"}`}>
          {totpEnabled ? "Activo" : "Inactivo"}
        </span>
      </div>

      {success && (
        <div className="mb-4 rounded-md bg-green-50 p-3 text-sm text-green-700 border border-green-200">
          {success}
        </div>
      )}

      {phase === "idle" && (
        <div>
          {!totpEnabled ? (
            <button onClick={startSetup} className="btn-primary text-sm">
              Activar autenticación en dos pasos
            </button>
          ) : (
            <button
              onClick={() => { setPhase("disable"); setError(""); setSuccess(""); setCode(""); }}
              className="btn px-4 py-2 text-sm bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
            >
              Desactivar 2FA
            </button>
          )}
        </div>
      )}

      {phase === "setup" && (
        <div className="space-y-5">
          <div className="flex flex-col sm:flex-row gap-6 items-start">
            {qrDataUrl && (
              <div className="flex-shrink-0 p-3 bg-white border border-gray-200 rounded-lg">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={qrDataUrl} alt="QR Code 2FA" width={180} height={180} />
              </div>
            )}
            <div className="space-y-3">
              <p className="text-sm text-gray-700">
                <strong>1.</strong> Abrí tu aplicación de autenticación (Google Authenticator, Authy, etc.)
              </p>
              <p className="text-sm text-gray-700">
                <strong>2.</strong> Escaneá el código QR o ingresá el secreto manualmente:
              </p>
              <code className="block text-xs font-mono bg-gray-100 text-gray-800 p-2 rounded break-all select-all">
                {secret}
              </code>
              <p className="text-sm text-gray-700">
                <strong>3.</strong> Ingresá el código de 6 dígitos para confirmar:
              </p>
            </div>
          </div>

          <div className="max-w-xs space-y-3">
            <input
              type="text"
              inputMode="numeric"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              className="input text-center text-xl tracking-[0.4em] font-mono"
              placeholder="000000"
              autoFocus
            />
            {error && <p className="text-sm text-red-600">{error}</p>}
            <div className="flex gap-3">
              <button
                onClick={() => enableMut.mutate()}
                disabled={code.length < 6 || enableMut.isPending}
                className="btn-primary text-sm"
              >
                {enableMut.isPending ? "Verificando..." : "Confirmar y activar"}
              </button>
              <button onClick={() => { setPhase("idle"); setCode(""); setError(""); }} className="btn-secondary text-sm">
                Cancelar
              </button>
            </div>
          </div>
        </div>
      )}

      {phase === "disable" && (
        <div className="space-y-4 max-w-xs">
          <p className="text-sm text-gray-700">
            Ingresá el código actual de tu aplicación para confirmar la desactivación:
          </p>
          <input
            type="text"
            inputMode="numeric"
            maxLength={6}
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            className="input text-center text-xl tracking-[0.4em] font-mono"
            placeholder="000000"
            autoFocus
          />
          {error && <p className="text-sm text-red-600">{error}</p>}
          <div className="flex gap-3">
            <button
              onClick={() => disableMut.mutate()}
              disabled={code.length < 6 || disableMut.isPending}
              className="btn px-4 py-2 text-sm bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
            >
              {disableMut.isPending ? "Desactivando..." : "Confirmar desactivación"}
            </button>
            <button onClick={() => { setPhase("idle"); setCode(""); setError(""); }} className="btn-secondary text-sm">
              Cancelar
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function UpstreamCard({
  upstream,
  onEdit,
  onDelete,
  onSetDefault,
}: {
  upstream: TSAUpstream;
  onEdit: () => void;
  onDelete: () => void;
  onSetDefault: () => void;
}) {
  const setDefaultMut = useMutation({
    mutationFn: () => api.setDefaultUpstream(upstream.id),
    onSuccess: onSetDefault,
  });

  const deleteMut = useMutation({
    mutationFn: () => api.deleteUpstream(upstream.id),
    onSuccess: onDelete,
  });

  return (
    <div className="border border-gray-200 rounded-lg p-4 hover:bg-gray-50">
      <div className="flex items-start justify-between mb-3">
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold text-gray-900">{upstream.name}</h3>
            {upstream.is_default && (
              <span className="badge badge-blue text-xs">Por defecto</span>
            )}
            {!upstream.is_active && (
              <span className="badge badge-gray text-xs">Inactiva</span>
            )}
          </div>
          <p className="text-sm text-gray-600 mt-1 break-all">{upstream.url}</p>
          <div className="flex gap-6 mt-2 text-xs text-gray-500">
            <span>Timeout: {upstream.timeout_ms}ms</span>
            <span>Reintentos: {upstream.max_retries}</span>
            {upstream.username ? (
              <span>Usuario: <span className="font-mono">{upstream.username}</span> · Contraseña: {upstream.has_password ? "●●●●●●" : "—"}</span>
            ) : (
              <span className="text-gray-400">Sin credenciales</span>
            )}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between pt-3 border-t border-gray-100">
        <div className="flex gap-2">
          {!upstream.is_default && (
            <button
              onClick={() => setDefaultMut.mutate()}
              disabled={setDefaultMut.isPending}
              className="btn-secondary text-xs px-3 py-1"
              title="Establecer como TSA por defecto"
            >
              {setDefaultMut.isPending ? "..." : "Usar por defecto"}
            </button>
          )}
        </div>
        <div className="flex gap-2">
          <button
            onClick={onEdit}
            className="btn-secondary text-xs px-3 py-1"
          >
            Editar
          </button>
          <button
            onClick={() => deleteMut.mutate()}
            disabled={deleteMut.isPending}
            className="btn px-3 py-1 text-xs bg-red-50 text-red-700 border border-red-200 hover:bg-red-100"
          >
            {deleteMut.isPending ? "..." : "Eliminar"}
          </button>
        </div>
      </div>
    </div>
  );
}

function UpstreamModal({
  upstreamId,
  onClose,
  onSuccess,
}: {
  upstreamId: string | null;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [timeoutMs, setTimeoutMs] = useState("10000");
  const [maxRetries, setMaxRetries] = useState("1");
  const [error, setError] = useState("");

  // Cargar datos del upstream si está editando
  const { data: upstream } = useQuery({
    queryKey: ["upstream", upstreamId],
    queryFn: () => upstreamId ? api.listUpstreams().then(res => res.data.find(u => u.id === upstreamId)) : null,
    enabled: !!upstreamId,
  });

  // Llenar formulario cuando se carga el upstream
  useEffect(() => {
    if (upstream) {
      setName(upstream.name);
      setUrl(upstream.url);
      setUsername(upstream.username || "");
      setPassword(""); // nunca se pre-rellena por seguridad
      setTimeoutMs(String(upstream.timeout_ms));
      setMaxRetries(String(upstream.max_retries));
    }
  }, [upstream]);

  const saveMut = useMutation({
    mutationFn: () => {
      const payload: Record<string, unknown> = {
        name,
        url,
        timeout_ms: parseInt(timeoutMs),
        max_retries: parseInt(maxRetries),
      };
      // Solo enviar username/password si hay valor (al editar, vacío = sin cambio)
      if (username !== "") payload.username = username;
      if (password !== "") payload.password = password;
      // Al editar con username vacío explícitamente queremos borrar
      if (upstreamId && username === "" && upstream?.username) {
        payload.username = "";
        payload.password = "";
      }

      return upstreamId
        ? api.updateUpstream(upstreamId, payload as any)
        : api.createUpstream(payload as any);
    },
    onSuccess: onSuccess,
    onError: (err: any) => {
      setError(err.message || err.error || "Error al guardar");
    },
  });

  const passwordPlaceholder = upstreamId && upstream?.has_password
    ? "Dejar vacío para no cambiar"
    : "Contraseña (opcional)";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="card w-full max-w-md p-6 mx-4">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">
          {upstreamId ? "Editar TSA Externa" : "Nueva TSA Externa"}
        </h2>

        <div className="space-y-4">
          <div>
            <label className="label">Nombre *</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="input"
              placeholder="Camerfirma, Sectigo, FreeTSA…"
            />
          </div>

          <div>
            <label className="label">URL de la TSA *</label>
            <input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              className="input font-mono text-xs"
              placeholder="https://tsuq.camerfirma.com:5004/cmf/tsa"
            />
          </div>

          <div className="border-t border-gray-100 pt-4">
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-3">
              Credenciales HTTP Basic (opcional)
            </p>
            <div className="space-y-3">
              <div>
                <label className="label">Usuario</label>
                <input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="input"
                  placeholder="Usuario (opcional)"
                  autoComplete="off"
                />
              </div>
              <div>
                <label className="label">Contraseña</label>
                <div className="relative">
                  <input
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    type={showPassword ? "text" : "password"}
                    className="input pr-10"
                    placeholder={passwordPlaceholder}
                    autoComplete="new-password"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(v => !v)}
                    className="absolute inset-y-0 right-0 px-3 text-gray-400 hover:text-gray-600"
                    tabIndex={-1}
                  >
                    {showPassword ? "🙈" : "👁️"}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label">Timeout (ms)</label>
              <input
                value={timeoutMs}
                onChange={(e) => setTimeoutMs(e.target.value)}
                className="input"
                type="number"
                min="100"
              />
            </div>
            <div>
              <label className="label">Reintentos máx</label>
              <input
                value={maxRetries}
                onChange={(e) => setMaxRetries(e.target.value)}
                className="input"
                type="number"
                min="0"
              />
            </div>
          </div>

          {error && <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="btn-secondary">
            Cancelar
          </button>
          <button
            onClick={() => saveMut.mutate()}
            disabled={!name || !url || saveMut.isPending}
            className="btn-primary"
          >
            {saveMut.isPending ? "Guardando..." : upstreamId ? "Actualizar" : "Crear"}
          </button>
        </div>
      </div>
    </div>
  );
}
