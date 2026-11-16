export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T = Record<string, unknown>>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch(
    path.startsWith("/api/") ? path : "/api/manage/" + path,
    {
      method,
      credentials: "same-origin",
      signal: AbortSignal.timeout(20000),
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    },
  );
  const data = await response.json().catch(() => {
    throw new ApiError("服务端响应无效，请稍后重试", response.status);
  });
  if (!response.ok) {
    if (response.status === 401 && !path.includes("/auth/login"))
      window.dispatchEvent(new Event("ane:logout"));
    throw new ApiError(data.msg || "请求失败", response.status);
  }
  return data as T;
}
export function scoped(path: string, merchant: string) {
  return (
    path +
    (path.includes("?") ? "&" : "?") +
    "merchant_id=" +
    encodeURIComponent(merchant)
  );
}
export const money = (n: number) =>
  new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY" }).format(
    n / 100,
  );
export const time = (s?: string | null) =>
  s ? new Date(s).toLocaleString("zh-CN", { hour12: false }) : "—";
export function maskAppKey(key: string, name: string) {
  const suffix = "_" + name;
  const hasSuffix = key.endsWith(suffix);
  const random = hasSuffix ? key.slice(0, -suffix.length) : key;
  return (
    random.slice(0, 4) + "******" + random.slice(-4) + (hasSuffix ? suffix : "")
  );
}
export interface Merchant {
  app_count?: number;
  payment_note: string;
  id: number;
  username: string;
  name: string;
  is_admin: boolean;
  enabled: boolean;
  pool_limit: number;
  order_ttl_minutes: number;
  merchant_key?: string;
  qr_image?: string;
  wechat_qr_image?: string;
  has_qr?: boolean;
  device_online?: boolean;
  created_at?: string;
}
export interface Order {
  trade_no: string;
  merchant_id: number;
  app_id: string;
  payment_type: "alipay" | "wxpay";
  out_trade_no: string;
  name: string;
  amount_cents: number;
  pay_amount_cents: number;
  discount_cents: number;
  status: string;
  created_at: string;
  expires_at: string;
  paid_at: string | null;
  callback_status: string;
  callback_attempts: number;
  callback_result: string;
  notify_url: string;
  return_url: string;
  param: string;
}
export interface Device {
  wifi_ssid: string;
  merchant_id: number;
  name: string;
  installation_id: string;
  revoked: boolean;
  last_seen: string | null;
  bound_at: string | null;
  online: boolean;
  version: string;
  model: string;
  android_version: string;
  battery: number;
  charging: boolean;
  network: string;
  network_validated: boolean;
  listener_connected: boolean;
  service_running: boolean;
  pending_events: number;
  uptime_seconds: number;
  heartbeat_age_seconds: number | null;
}
export interface EventRecord {
  payment_type: "alipay" | "wxpay";
  id: string;
  merchant_id: number;
  merchant_name: string;
  raw_text: string;
  amount: number;
  status: string;
  reason: string;
  posted_at: string;
  received_at: string;
  trade_no: string | null;
}
export interface Pool {
  payment_type: "alipay" | "wxpay";
  merchant_id: number;
  merchant_name: string;
  amount_cents: number;
  capacity: number;
  pool_limit: number;
  occupied: number;
  slots: {
    discount: number;
    trade_no: string;
    out_trade_no: string;
    expires_at: string;
  }[];
}
export interface SMTP {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  password?: string;
  from_address: string;
  from_name: string;
  security: string;
}
