"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import QRCode from "qrcode";
import { api } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [expiredNotice, setExpiredNotice] = useState(false);

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("expired") === "1") {
      setExpiredNotice(true);
    }
  }, []);

  // Estado 2FA (login normal, usuario ya tiene 2FA activo)
  const [mfaRequired, setMfaRequired] = useState(false);
  const [mfaToken, setMfaToken] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const totpRef = useRef<HTMLInputElement>(null);

  // Estado de configuración obligatoria de 2FA (primer login, sin 2FA todavía)
  const [setupRequired, setSetupRequired] = useState(false);
  const [setupToken, setSetupToken] = useState("");
  const [qrDataUrl, setQrDataUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [setupCode, setSetupCode] = useState("");
  const setupCodeRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mfaRequired) totpRef.current?.focus();
  }, [mfaRequired]);

  useEffect(() => {
    if (setupRequired) setupCodeRef.current?.focus();
  }, [setupRequired]);

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const result = await api.login(username, password);
      if (result.mfa_required && result.mfa_token) {
        setMfaToken(result.mfa_token);
        setMfaRequired(true);
      } else if (result.totp_setup_required && result.setup_token) {
        setSetupToken(result.setup_token);
        const setup = await api.setupTOTPForLogin(result.setup_token);
        setSecret(setup.secret);
        const url = await QRCode.toDataURL(setup.qr_url, { width: 200, margin: 1 });
        setQrDataUrl(url);
        setSetupRequired(true);
      } else {
        router.push("/");
      }
    } catch {
      setError("Credenciales incorrectas. Intenta nuevamente.");
    } finally {
      setLoading(false);
    }
  }

  async function handleTOTP(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await api.verifyTOTP(mfaToken, totpCode.replace(/\s/g, ""));
      router.push("/");
    } catch {
      setError("Código incorrecto. Verifica tu aplicación de autenticación.");
      setTotpCode("");
      totpRef.current?.focus();
    } finally {
      setLoading(false);
    }
  }

  async function handleCompleteSetup(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await api.completeTOTPSetup(setupToken, setupCode.replace(/\s/g, ""));
      router.push("/");
    } catch {
      setError("Código incorrecto. Verifica tu aplicación de autenticación.");
      setSetupCode("");
      setupCodeRef.current?.focus();
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-8">
      <div className={`w-full ${setupRequired ? "max-w-xl" : "max-w-md"}`}>
        {/* Logo / título */}
        <div className="text-center mb-8">
          <Image
            src="/brand/logo-color.svg"
            alt="BIGDAVI"
            width={550}
            height={253}
            className="h-[210px] w-auto mx-auto mb-5"
            priority
            unoptimized
          />
        </div>

        <div className="card p-8">
          {setupRequired ? (
            /* ── Configuración obligatoria de 2FA (primer login) ── */
            <form onSubmit={handleCompleteSetup} className="space-y-5">
              <div className="text-center mb-2">
                <div className="mx-auto w-12 h-12 bg-primary-50 rounded-full flex items-center justify-center mb-3">
                  <svg className="w-6 h-6 text-primary-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                      d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                  </svg>
                </div>
                <h2 className="text-lg font-semibold text-gray-900">Configura la autenticación en dos pasos</h2>
                <p className="text-sm text-gray-500 mt-1">
                  Por seguridad, todos los usuarios deben activar 2FA antes de continuar.
                  Este paso solo se pide una vez.
                </p>
              </div>

              <p className="text-sm text-gray-700 bg-gray-50 border border-gray-200 rounded-md p-3">
                Usá una app de autenticación como <strong>Google Authenticator</strong>,{" "}
                <strong>Microsoft Authenticator</strong> o <strong>Authy</strong> en tu celular.
                Si no tenés ninguna instalada, instalá una antes de continuar.
              </p>

              <div className="flex flex-col sm:flex-row gap-6 items-start">
                {qrDataUrl && (
                  <div className="flex-shrink-0 p-3 bg-white border border-gray-200 rounded-lg mx-auto sm:mx-0">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img src={qrDataUrl} alt="QR Code 2FA" width={180} height={180} />
                  </div>
                )}
                <div className="space-y-3">
                  <p className="text-sm text-gray-700"><strong>1.</strong> Abrí la app y escaneá el código QR.</p>
                  <p className="text-sm text-gray-700"><strong>2.</strong> ¿No podés escanear? Ingresá este código manualmente:</p>
                  <code className="block text-xs font-mono bg-gray-100 text-gray-800 p-2 rounded break-all select-all">
                    {secret}
                  </code>
                  <p className="text-sm text-gray-700"><strong>3.</strong> Ingresá el código de 6 dígitos que te muestra la app:</p>
                </div>
              </div>

              <div className="max-w-xs mx-auto space-y-3">
                <input
                  id="setup-totp"
                  ref={setupCodeRef}
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9 ]{6,7}"
                  maxLength={7}
                  value={setupCode}
                  onChange={(e) => setSetupCode(e.target.value)}
                  className="input text-center text-2xl tracking-[0.5em] font-mono"
                  placeholder="000000"
                  required
                  autoComplete="one-time-code"
                />
              </div>

              {error && <ErrorBox message={error} />}

              <button
                type="submit"
                disabled={loading || setupCode.replace(/\s/g, "").length < 6}
                className="btn-primary w-full"
              >
                {loading ? <Spinner label="Activando..." /> : "Confirmar y continuar"}
              </button>

              <p className="text-xs text-gray-400 text-center">
                Una vez confirmado, vas a entrar directamente al panel. Este código se te va a
                pedir en cada inicio de sesión a partir de ahora.
              </p>
            </form>
          ) : !mfaRequired ? (
            /* ── Step 1: usuario + contraseña ── */
            <form onSubmit={handleLogin} className="space-y-5">
              {expiredNotice && (
                <div className="rounded-md bg-amber-50 p-3 text-sm text-amber-700 border border-amber-200">
                  Tu sesión se cerró por inactividad. Iniciá sesión nuevamente.
                </div>
              )}
              <div>
                <label htmlFor="username" className="label">Usuario</label>
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="input"
                  placeholder="admin"
                  required
                  autoFocus
                  autoComplete="username"
                />
              </div>

              <div>
                <label htmlFor="password" className="label">Contraseña</label>
                <div className="relative">
                  <input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="input pr-10"
                    placeholder="••••••••"
                    required
                    autoComplete="current-password"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((v) => !v)}
                    className="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 hover:text-gray-600"
                    tabIndex={-1}
                    aria-label={showPassword ? "Ocultar contraseña" : "Mostrar contraseña"}
                  >
                    {showPassword ? (
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                          d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                      </svg>
                    ) : (
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                          d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                      </svg>
                    )}
                  </button>
                </div>
              </div>

              {error && <ErrorBox message={error} />}

              <button type="submit" disabled={loading} className="btn-primary w-full">
                {loading ? <Spinner label="Iniciando sesión..." /> : "Iniciar sesión"}
              </button>
            </form>
          ) : (
            /* ── Step 2: código TOTP ── */
            <form onSubmit={handleTOTP} className="space-y-5">
              <div className="text-center mb-2">
                <div className="mx-auto w-12 h-12 bg-primary-50 rounded-full flex items-center justify-center mb-3">
                  <svg className="w-6 h-6 text-primary-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                      d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                  </svg>
                </div>
                <h2 className="text-lg font-semibold text-gray-900">Verificación en dos pasos</h2>
                <p className="text-sm text-gray-500 mt-1">
                  Ingresá el código de 6 dígitos de tu aplicación de autenticación
                </p>
              </div>

              <div>
                <label htmlFor="totp" className="label">Código de autenticación</label>
                <input
                  id="totp"
                  ref={totpRef}
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9 ]{6,7}"
                  maxLength={7}
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  className="input text-center text-2xl tracking-[0.5em] font-mono"
                  placeholder="000000"
                  required
                  autoComplete="one-time-code"
                />
              </div>

              {error && <ErrorBox message={error} />}

              <button type="submit" disabled={loading || totpCode.replace(/\s/g, "").length < 6} className="btn-primary w-full">
                {loading ? <Spinner label="Verificando..." /> : "Verificar código"}
              </button>

              <button
                type="button"
                onClick={() => { setMfaRequired(false); setError(""); setTotpCode(""); }}
                className="w-full text-sm text-gray-500 hover:text-gray-700"
              >
                ← Volver al inicio de sesión
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}

function ErrorBox({ message }: { message: string }) {
  return (
    <div className="rounded-md bg-red-50 p-3 text-sm text-red-700 border border-red-200">
      {message}
    </div>
  );
}

function Spinner({ label }: { label: string }) {
  return (
    <span className="flex items-center justify-center gap-2">
      <svg className="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      {label}
    </span>
  );
}
