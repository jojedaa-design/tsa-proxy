/**
 * Cliente HTTP centralizado para la API admin.
 * Maneja auth, refresh automático y errores.
 */

const BASE_URL =
  typeof window !== "undefined"
    ? "" // browser: relative URL, Nginx enruta
    : process.env.NEXT_PUBLIC_API_URL || "http://backend:8080";

// ── Types ─────────────────────────────────────────────────────

export interface ApiError {
  error: string;
  message?: string;
  request_id?: string;
  fields?: Record<string, string>;
}

export interface Pagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  description?: string;
  status: "active" | "suspended" | "deleted";
  contact_email?: string;
  created_at: string;
  updated_at: string;
  quota?: TenantQuota;
}

export interface TenantQuota {
  id: string;
  tenant_id: string;
  burst_per_minute: number;
  created_at: string;
  updated_at: string;
}

export interface QuotaBundle {
  id: string;
  tenant_id: string;
  amount: number;
  note?: string;
  created_by?: string;
  contracted_at: string;
  alert_threshold_percent?: number;
}

export interface AlertEmail {
  id: string;
  tenant_id: string;
  email: string;
  label?: string;
  created_at: string;
}

export interface APICredential {
  id: string;
  tenant_id: string;
  key_id: string;
  key_prefix: string;
  url_token?: string;  // token para URL de software de firma (siempre visible)
  name?: string;
  status: "active" | "revoked" | "expired";
  expires_at?: string;
  last_used_at?: string;
  last_used_ip?: string;
  created_at: string;
  api_key?: string;   // solo en creación/rotación
  stamp_url?: string; // URL lista para software de firma (solo en creación/rotación)
  warning?: string;
}

export interface BasicAuthCredential {
  id: string;
  tenant_id: string;
  username: string;
  key_prefix: string;
  name?: string;
  status: "active" | "revoked";
  created_at: string;
  // Solo en creación:
  password?: string;
  tsa_endpoint?: string;
  warning?: string;
}

export interface IPAllowlistEntry {
  id: string;
  tenant_id: string;
  cidr: string;
  label?: string;
  is_active: boolean;
  created_at: string;
}

export interface NoAuthAccess {
  id: string;
  tenant_id: string;
  name?: string;
  status: "active" | "suspended";
  created_at: string;
  updated_at: string;
}

export interface MonthlyAggregate {
  tenant_id: string;
  tenant_name: string;
  year: number;
  month: number;
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  rejected_requests: number;
  avg_latency_ms: number;
  monthly_limit?: number;
  exceeded_requests?: number;
}

export interface IPUsage {
  ip: string;
  tenant_id: string;
  tenant_name: string;
  requests: number;
  success_count: number;
  fail_count: number;
  last_used_at?: string;
}

export interface UserAgentStat {
  user_agent: string;
  requests: number;
  success_count: number;
  fail_count: number;
  unique_ips: number;
  unique_tenants: number;
  avg_latency_ms: number;
  last_used_at?: string;
}

export interface CountryStat {
  country: string;
  country_name?: string;
  requests: number;
  success_count: number;
  fail_count: number;
  unique_ips: number;
  unique_tenants: number;
  avg_latency_ms: number;
  last_used_at?: string;
}

export interface DashboardSummary {
  active_tenants: number;
  today_requests: number;
  month_requests: number;
  errors_last_24h: number;
  avg_latency_ms: number;
  top_tenants: Array<{ tenant_id: string; tenant_name: string; requests: number }>;
}

export interface FailureDetail {
  reason: string;
  label: string;
  /** "auth" | "quota" | "rate" | "upstream" | "request" | "other" */
  category: string;
  count: number;
  last_seen: string;
}

export interface FailureBreakdownResponse {
  /** Errores que llegaron al proxy (upstream_error, quota_exceeded) */
  proxy_errors: FailureDetail[];
  /** Rechazos previos al proxy (clave inválida, IP bloqueada, etc.) */
  auth_failures: FailureDetail[];
  total_proxy_errors: number;
  total_auth_failures: number;
}

export interface AuditEvent {
  id: string;
  actor_id?: string;
  actor_type: string;
  actor_name: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  changes?: Record<string, unknown>;
  ip_address?: string;
  occurred_at: string;
}

export interface LoginResponse {
  access_token?: string;
  expires_in?: number;
  user?: { id: string; username: string; email: string; roles: string[]; tenant_scope?: string[] };
  mfa_required?: boolean;
  mfa_token?: string;
  totp_setup_required?: boolean;
  setup_token?: string;
}

export interface PlatformUser {
  id: string;
  username: string;
  email: string;
  is_active: boolean;
  totp_enabled: boolean;
  role: string;
  tenant_scope: string[]; // vacío = todos los tenants
  tenant_name?: string;   // solo para rol viewer con un tenant asignado
  created_at: string;
  last_login_at?: string;
}

export interface HardwareMetrics {
  cpu_percent: number;
  mem_percent: number;
  mem_used_mb: number;
  mem_total_mb: number;
  disk_percent: number;
  disk_used_gb: number;
  disk_total_gb: number;
  net_rx_kbps: number;
  net_tx_kbps: number;
}

export interface DailyUsage {
  date: string; // YYYY-MM-DD
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  rejected_requests: number;
}

export interface Role {
  id: string;
  name: string;
  description?: string;
}

export interface TOTP2FASetup {
  secret: string;
  qr_url: string;
}

export interface TSAUpstream {
  id: string;
  name: string;
  url: string;
  username?: string;
  has_password: boolean;
  timeout_ms: number;
  max_retries: number;
  is_active: boolean;
  is_default: boolean;
  // Presente solo en la respuesta de updateUpstream: el backend borró las
  // credenciales almacenadas porque cambió el host del upstream. Hay que
  // reingresarlas para el destino nuevo.
  credentials_cleared?: boolean;
}

// ── Auth storage ──────────────────────────────────────────────

const TOKEN_KEY = "tsa_access_token";

// sessionStorage (no localStorage): el token debe desaparecer al cerrar el
// navegador, no persistir entre reinicios.
export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return sessionStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY);
}

// ── Fetch wrapper ─────────────────────────────────────────────

async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string>),
  };

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers });

  // Refresh automático si el token expiró — no aplica a endpoints que no usan
  // Bearer (login, verificación de código MFA y el flujo de setup obligatorio
  // de 2FA identifican al usuario con un token propio en el body); un 401 ahí
  // es un error normal (credenciales/código inválido) a mostrar inline, no una
  // sesión expirada.
  const noBearerAuthEndpoints = [
    "/api/admin/v1/auth/login",
    "/api/admin/v1/auth/2fa/verify",
    "/api/admin/v1/auth/2fa/setup-required",
    "/api/admin/v1/auth/2fa/complete-setup",
  ];
  if (res.status === 401 && !noBearerAuthEndpoints.includes(path)) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      headers["Authorization"] = `Bearer ${getToken()}`;
      const retryRes = await fetch(`${BASE_URL}${path}`, { ...options, headers });
      if (!retryRes.ok) {
        const err: ApiError = await retryRes.json().catch(() => ({ error: "unknown" }));
        throw err;
      }
      if (retryRes.status === 204) return undefined as T;
      return retryRes.json();
    } else {
      clearToken();
      window.location.href = "/login";
      throw { error: "unauthorized" };
    }
  }

  if (!res.ok) {
    const err: ApiError = await res.json().catch(() => ({ error: "network_error" }));
    throw err;
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

async function tryRefresh(): Promise<boolean> {
  try {
    const res = await fetch(`${BASE_URL}/api/admin/v1/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) return false;
    const data = await res.json();
    setToken(data.access_token);
    return true;
  } catch {
    return false;
  }
}

// ── API methods ───────────────────────────────────────────────

export const api = {
  // Auth
  async login(username: string, password: string): Promise<LoginResponse> {
    const data = await apiFetch<LoginResponse>("/api/admin/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (data.access_token) {
      setToken(data.access_token);
    }
    return data;
  },

  async verifyTOTP(mfaToken: string, code: string): Promise<LoginResponse> {
    const data = await apiFetch<LoginResponse>("/api/admin/v1/auth/2fa/verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, code }),
    });
    if (data.access_token) {
      setToken(data.access_token);
    }
    return data;
  },

  async setup2FA(): Promise<TOTP2FASetup> {
    return apiFetch<TOTP2FASetup>("/api/admin/v1/auth/2fa/setup");
  },

  async enable2FA(code: string): Promise<void> {
    await apiFetch("/api/admin/v1/auth/2fa/enable", {
      method: "POST",
      body: JSON.stringify({ code }),
    });
  },

  // 2FA obligatorio: flujo de setup forzado en el login (usuario sin sesión).
  async setupTOTPForLogin(setupToken: string): Promise<{ secret: string; qr_url: string }> {
    return apiFetch("/api/admin/v1/auth/2fa/setup-required", {
      method: "POST",
      body: JSON.stringify({ setup_token: setupToken }),
    });
  },

  async completeTOTPSetup(setupToken: string, code: string): Promise<LoginResponse> {
    const data = await apiFetch<LoginResponse>("/api/admin/v1/auth/2fa/complete-setup", {
      method: "POST",
      body: JSON.stringify({ setup_token: setupToken, code }),
    });
    if (data.access_token) {
      setToken(data.access_token);
    }
    return data;
  },

  async logout() {
    await apiFetch("/api/admin/v1/auth/logout", { method: "POST" }).catch(() => {});
    clearToken();
  },

  async me() {
    return apiFetch<{
      id: string; username: string; email: string; roles: string[];
      tenant_scope?: string[];
      tenant_scope_detail?: Array<{ id: string; name: string }>;
      totp_enabled: boolean;
    }>("/api/admin/v1/auth/me");
  },

  // Tenants
  async listTenants(params: { page?: number; limit?: number; status?: string; search?: string } = {}) {
    const q = new URLSearchParams();
    if (params.page)   q.set("page",   String(params.page));
    if (params.limit)  q.set("limit",  String(params.limit));
    if (params.status) q.set("status", params.status);
    if (params.search) q.set("search", params.search);
    return apiFetch<{ data: Tenant[]; pagination: Pagination }>(
      `/api/admin/v1/tenants?${q}`
    );
  },

  async getTenant(id: string) {
    return apiFetch<Tenant>(`/api/admin/v1/tenants/${id}`);
  },

  async createTenant(data: { name: string; slug?: string; description?: string; contact_email?: string }) {
    return apiFetch<Tenant>("/api/admin/v1/tenants", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async updateTenant(id: string, data: { name: string; description?: string; contact_email?: string }) {
    return apiFetch<Tenant>(`/api/admin/v1/tenants/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },

  async suspendTenant(id: string) {
    return apiFetch(`/api/admin/v1/tenants/${id}/suspend`, { method: "POST" });
  },

  async reactivateTenant(id: string) {
    return apiFetch(`/api/admin/v1/tenants/${id}/reactivate`, { method: "POST" });
  },

  async deleteTenant(id: string) {
    return apiFetch(`/api/admin/v1/tenants/${id}`, { method: "DELETE" });
  },

  async verifyAndDeleteTenant(id: string, totpCode: string) {
    return apiFetch(`/api/admin/v1/tenants/${id}/verify-delete`, {
      method: "POST",
      body: JSON.stringify({ totp_code: totpCode }),
    });
  },

  // Credentials
  async listCredentials(tenantId: string) {
    return apiFetch<{ data: APICredential[] }>(
      `/api/admin/v1/tenants/${tenantId}/credentials`
    );
  },

  async createCredential(tenantId: string, data: { name?: string }) {
    return apiFetch<APICredential>(`/api/admin/v1/tenants/${tenantId}/credentials`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async rotateCredential(credId: string) {
    return apiFetch<APICredential>(`/api/admin/v1/credentials/${credId}/rotate`, {
      method: "POST",
    });
  },

  async revokeCredential(credId: string) {
    return apiFetch(`/api/admin/v1/credentials/${credId}/revoke`, { method: "POST" });
  },

  // IP Allowlist
  async listIPAllowlist(tenantId: string) {
    return apiFetch<{ data: IPAllowlistEntry[] }>(
      `/api/admin/v1/tenants/${tenantId}/ip-allowlist`
    );
  },

  async createIPAllowlist(tenantId: string, data: { cidr: string; label?: string }) {
    return apiFetch<IPAllowlistEntry>(`/api/admin/v1/tenants/${tenantId}/ip-allowlist`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async deleteIPAllowlist(entryId: string) {
    return apiFetch(`/api/admin/v1/ip-allowlist/${entryId}`, { method: "DELETE" });
  },

  // Quotas
  async getQuota(tenantId: string) {
    return apiFetch<TenantQuota>(`/api/admin/v1/tenants/${tenantId}/quota`);
  },

  async updateQuota(tenantId: string, data: { burst_per_minute: number }) {
    return apiFetch<TenantQuota>(`/api/admin/v1/tenants/${tenantId}/quota`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },

  async addBundle(tenantId: string, data: { amount: number; note?: string; alert_threshold_percent?: number }) {
    return apiFetch<QuotaBundle>(`/api/admin/v1/tenants/${tenantId}/bundles`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async listBundles(tenantId: string) {
    return apiFetch<QuotaBundle[]>(`/api/admin/v1/tenants/${tenantId}/bundles`);
  },

  // Alert emails
  async listAlertEmails(tenantId: string) {
    return apiFetch<AlertEmail[]>(`/api/admin/v1/tenants/${tenantId}/alert-emails`);
  },

  async addAlertEmail(tenantId: string, data: { email: string; label?: string }) {
    return apiFetch<AlertEmail>(`/api/admin/v1/tenants/${tenantId}/alert-emails`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async deleteAlertEmail(tenantId: string, emailId: string) {
    return apiFetch<void>(`/api/admin/v1/tenants/${tenantId}/alert-emails/${emailId}`, {
      method: "DELETE",
    });
  },

  async testAlertEmails(tenantId: string) {
    return apiFetch<{ sent_to: string[]; message: string }>(
      `/api/admin/v1/tenants/${tenantId}/alert-emails/test`,
      { method: "POST" }
    );
  },

  // Reports
  async getUsageSummary() {
    return apiFetch<DashboardSummary>("/api/admin/v1/reports/usage/summary");
  },

  async getNextReportNumber() {
    return apiFetch<{ number: number }>("/api/admin/v1/reports/next-number", { method: "POST" });
  },

  async getUsage(params: { tenant_id?: string; from?: string; to?: string }) {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    if (params.tenant_id) q.set("tenant_id", params.tenant_id);
    return apiFetch<{ data: MonthlyAggregate[]; summary: unknown }>(
      `/api/admin/v1/reports/usage?${q}`
    );
  },

  async getTopIPs(params: { from?: string; to?: string }) {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    return apiFetch<{ data: IPUsage[] }>(
      `/api/admin/v1/reports/top-ips?${q}`
    );
  },

  async getTopUserAgents(params: { from?: string; to?: string }) {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    return apiFetch<{ data: UserAgentStat[] }>(
      `/api/admin/v1/reports/top-user-agents?${q}`
    );
  },

  async getTopCountries(params: { from?: string; to?: string }) {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    return apiFetch<{ data: CountryStat[] }>(
      `/api/admin/v1/reports/top-countries?${q}`
    );
  },

  async getFailureBreakdown(params: { tenant_id?: string; from?: string; to?: string }) {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    if (params.tenant_id) q.set("tenant_id", params.tenant_id);
    return apiFetch<FailureBreakdownResponse>(
      `/api/admin/v1/reports/failures?${q}`
    );
  },

  async getBundleReport(tenantId: string) {
    return apiFetch<Array<{ id: string; amount: number; consumed: number; note?: string; contracted_at: string; alert_threshold_percent?: number }>>(
      `/api/admin/v1/reports/bundles?tenant_id=${encodeURIComponent(tenantId)}`
    );
  },

  async getDailyUsage(tenantId: string, from: string, to: string) {
    const q = new URLSearchParams({ tenant_id: tenantId, from, to });
    return apiFetch<DailyUsage[]>(`/api/admin/v1/reports/daily-usage?${q}`);
  },

  async getHardwareMetrics() {
    return apiFetch<HardwareMetrics>("/api/admin/v1/system/hardware");
  },

  async updateBundleAlert(tenantId: string, bundleId: string, alertThresholdPercent: number | null) {
    return apiFetch<{ bundle_id: string; alert_threshold_percent: number | null }>(
      `/api/admin/v1/tenants/${tenantId}/bundles/${bundleId}`,
      { method: "PATCH", body: JSON.stringify({ alert_threshold_percent: alertThresholdPercent }) }
    );
  },

  // Audit
  async listAuditEvents(params: { page?: number; action?: string; from?: string; to?: string } = {}) {
    const q = new URLSearchParams();
    if (params.page)   q.set("page",   String(params.page));
    if (params.action) q.set("action", params.action);
    if (params.from)   q.set("from",   params.from);
    if (params.to)     q.set("to",     params.to);
    return apiFetch<{ data: AuditEvent[]; pagination: Pagination }>(
      `/api/admin/v1/audit-events?${q}`
    );
  },

  // Config
  async listUpstreams() {
    return apiFetch<{ data: TSAUpstream[] }>("/api/admin/v1/config/upstreams");
  },

  async createUpstream(data: { name: string; url: string; username?: string; password?: string; timeout_ms?: number; max_retries?: number }) {
    return apiFetch<TSAUpstream>("/api/admin/v1/config/upstreams", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async updateUpstream(id: string, data: { name?: string; url?: string; username?: string; password?: string; timeout_ms?: number; max_retries?: number; is_active?: boolean }) {
    // Limpiar undefined values
    const body = Object.fromEntries(
      Object.entries(data).filter(([, v]) => v !== undefined)
    );
    return apiFetch<TSAUpstream>(`/api/admin/v1/config/upstreams/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  async deleteUpstream(id: string) {
    // Soft delete by setting is_active to false
    return apiFetch(`/api/admin/v1/config/upstreams/${id}`, {
      method: "PUT",
      body: JSON.stringify({ is_active: false }),
    });
  },

  async setDefaultUpstream(id: string) {
    return apiFetch(`/api/admin/v1/config/upstreams/${id}/set-default`, { method: "POST" });
  },

  // Basic Auth Credentials
  async listBasicAuth(tenantId: string) {
    return apiFetch<{ data: BasicAuthCredential[]; tsa_endpoint: string }>(
      `/api/admin/v1/tenants/${tenantId}/basic-auth`
    );
  },

  async createBasicAuth(tenantId: string, payload: { username: string; name?: string }) {
    return apiFetch<BasicAuthCredential>(`/api/admin/v1/tenants/${tenantId}/basic-auth`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async revokeBasicAuth(id: string) {
    return apiFetch(`/api/admin/v1/basic-auth/${id}/revoke`, { method: "POST" });
  },

  // No-Auth Access (acceso TSP sin credenciales, por IP)
  async getNoAuth(tenantId: string) {
    return apiFetch<{ enabled: boolean; access: NoAuthAccess | null }>(
      `/api/admin/v1/tenants/${tenantId}/noauth`
    );
  },

  async enableNoAuth(tenantId: string, name?: string) {
    return apiFetch<{ enabled: boolean; access: NoAuthAccess; message: string }>(
      `/api/admin/v1/tenants/${tenantId}/noauth/enable`,
      { method: "POST", body: JSON.stringify({ name }) }
    );
  },

  async disableNoAuth(tenantId: string) {
    return apiFetch<{ message: string }>(
      `/api/admin/v1/tenants/${tenantId}/noauth/disable`,
      { method: "POST" }
    );
  },

  async deleteNoAuth(tenantId: string) {
    return apiFetch(`/api/admin/v1/tenants/${tenantId}/noauth`, { method: "DELETE" });
  },

  // Usuarios de la plataforma
  async listUsers(params: { page?: number; limit?: number; search?: string } = {}) {
    const q = new URLSearchParams();
    if (params.page)   q.set("page",   String(params.page));
    if (params.limit)  q.set("limit",  String(params.limit));
    if (params.search) q.set("search", params.search);
    return apiFetch<{ data: PlatformUser[]; pagination: Pagination }>(
      `/api/admin/v1/users?${q}`
    );
  },

  async createUser(data: { username: string; email: string; password: string; role: string; tenant_ids?: string[] }) {
    return apiFetch<PlatformUser>("/api/admin/v1/users", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },

  async updateUser(id: string, data: { email: string; is_active: boolean; role: string; tenant_ids?: string[] }) {
    return apiFetch<PlatformUser>(`/api/admin/v1/users/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },

  async deleteUser(id: string) {
    return apiFetch(`/api/admin/v1/users/${id}`, { method: "DELETE" });
  },

  async resetUserPassword(id: string, password: string) {
    return apiFetch(`/api/admin/v1/users/${id}/reset-password`, {
      method: "POST",
      body: JSON.stringify({ password }),
    });
  },

  async resetUserTOTP(id: string) {
    return apiFetch(`/api/admin/v1/users/${id}/reset-2fa`, { method: "POST" });
  },

  async listRoles() {
    return apiFetch<{ data: Role[] }>("/api/admin/v1/roles");
  },
};
