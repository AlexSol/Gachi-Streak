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
  presence?: Presence;
  today_apps?: AppTime[];
}

export interface AppTime {
  app: string;
  seconds: number;
}

export interface Presence {
  app?: string;
  since?: string;
  idle: boolean;
  online: boolean;
  updated_at: string;
}

export interface ActivityUser {
  name: string;
  total_seconds: number;
  apps: AppTime[];
  by_day: { day: string; seconds: number }[];
}

export interface ActivityResponse {
  days: number;
  users: ActivityUser[];
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

export interface RankDef {
  level: number;
  key: string;
  name: string;
  min_xp: number;
  code: string;
}

export type AchCategory = "streak" | "go" | "tests" | "duo" | "shame" | "secret";

export interface AchievementDef {
  id: string;
  category: AchCategory;
  xp: number;
  target: number;
  detector: "auto" | "diff";
  rule: string;
  secret?: boolean;
}

export interface AchievementState {
  id: string;
  unlocked: boolean;
  day?: string;
  progress: number;
  target: number;
}

export interface UserProgress {
  name: string;
  xp: number;
  xp_days: number;
  xp_achievements: number;
  xp_penalty: number;
  rank: number;
  next_rank_xp?: number;
  active_days: number;
  failures: number;
  achievements: AchievementState[];
}

export interface ProgressResponse {
  me: string;
  xp_per_day: number;
  xp_per_failure: number;
  ranks: RankDef[];
  achievements: AchievementDef[];
  users: UserProgress[];
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
  progress: () => call<ProgressResponse>("GET", "/progress"),
  activity: (days = 7) => call<ActivityResponse>("GET", `/activity?days=${days}`),
  ping: (note: string) =>
    call<{ counted: boolean; user: UserStatus }>("POST", "/ping", { source: "manual", note }),
};
