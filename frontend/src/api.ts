// Thin client for gachi-server. The session lives in an HttpOnly cookie,
// so the browser sends it automatically; JS never sees it.

export type State = "safe" | "pending" | "warning" | "failed";

export interface Failure {
  id: number;
  day: string;
  at: string;
}

export interface UserStatus {
  name: string;
  timezone: string;
  streak: number;
  best: number;
  last_active_day: string;
  done_today: boolean;
  state: State;
  deadline: string; // RFC3339
  seconds_left: number;
  last_failure?: Failure;
}

export interface StatusResponse {
  me: string;
  server_time: string;
  warn_hours: number;
  users: UserStatus[];
}

export interface Me {
  name: string;
  timezone: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error ?? res.statusText);
  return data as T;
}

export const api = {
  me: () => call<Me>("GET", "/auth/me"),
  login: (token: string) => call<Me>("POST", "/auth/login", { token }),
  logout: () => call<void>("POST", "/auth/logout"),
  status: () => call<StatusResponse>("GET", "/status"),
  ping: (note: string) =>
    call<{ counted: boolean; user: UserStatus }>("POST", "/ping", { source: "manual", note }),
};
