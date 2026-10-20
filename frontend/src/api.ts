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
  // Older servers send null instead of [] for a user without usage.
  apps: AppTime[] | null;
  by_day: { day: string; seconds: number }[] | null;
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
  /** "code" = Go constructs found in commits pushed to GitHub. */
  detector: "auto" | "code";
  rule: string;
  secret?: boolean;
}

export interface AchievementState {
  id: string;
  unlocked: boolean;
  day?: string;
  progress: number;
  target: number;
  /** My own unlock I have not been notified about yet. */
  new?: boolean;
  /** My own unlock I have not unpacked yet (plays the opening effect). */
  packed?: boolean;
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
  /** Pushes are analyzed (GitHub login set): "code" achievements can unlock. */
  code_tracking: boolean;
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

export type LeaderScope = "friends" | "all";
export type LeaderFilter = "all" | "online" | "risk";
/** How a user relates to me; "" = strangers. */
export type Relation = "me" | "friend" | "incoming" | "outgoing" | "";
export type LeaderSort = "streak" | "best" | "xp" | "name" | "seen";

export interface LeaderRow {
  /** 1-based place in the current sort. */
  pos: number;
  name: string;
  streak: number;
  best: number;
  state: State;
  done_today: boolean;
  deadline: string; // RFC3339
  seconds_left: number;
  last_active_day: string;
  xp: number;
  rank: number;
  rank_name: string;
  presence?: Presence;
  /** Tracked seconds for the user's last 7 days, oldest first. */
  week: number[];
  relation?: Relation;
}

export interface UsersQuery {
  scope?: LeaderScope;
  q?: string;
  filter?: LeaderFilter;
  sort?: LeaderSort;
  page?: number;
  limit?: number;
}

export interface UsersResponse {
  me: string;
  scope: LeaderScope;
  /** Scope sizes and my incoming requests; they ignore q and filter. */
  scopes: { friends: number; all: number; incoming: number };
  q: string;
  filter: LeaderFilter;
  sort: LeaderSort;
  page: number;
  limit: number;
  total: number;
  counts: { all: number; online: number; at_risk: number };
  users: LeaderRow[];
  /** My own row in the current sort, ignoring q and filter. */
  me_row?: LeaderRow;
  /** My friends (and me) not counted yet with the deadline close, nearest first. */
  at_risk: LeaderRow[];
}

export interface FriendRequest {
  name: string;
  streak: number;
  best: number;
  at: string; // RFC3339
}

export interface FriendRequestsResponse {
  incoming: FriendRequest[];
  outgoing: FriendRequest[];
}

export interface FriendResult {
  name: string;
  relation: Relation;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, {
    method,
    signal,
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
  achievementsSeen: () => call<void>("POST", "/achievements/seen"),
  openAchievement: (id: string) => call<void>("POST", `/achievements/${encodeURIComponent(id)}/open`),
  activity: (days = 7) => call<ActivityResponse>("GET", `/activity?days=${days}`),
  users: (q: UsersQuery, signal?: AbortSignal) => {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== "") p.set(k, String(v));
    return call<UsersResponse>("GET", `/users?${p}`, undefined, signal);
  },
  friendRequests: (signal?: AbortSignal) =>
    call<FriendRequestsResponse>("GET", "/friends/requests", undefined, signal),
  /** Ask someone; if they already asked me, this makes us friends. */
  addFriend: (name: string) => call<FriendResult>("POST", "/friends/requests", { name }),
  acceptFriend: (name: string) => call<FriendResult>("POST", `/friends/requests/${encodeURIComponent(name)}/accept`),
  declineFriend: (name: string) => call<FriendResult>("POST", `/friends/requests/${encodeURIComponent(name)}/decline`),
  cancelFriend: (name: string) => call<FriendResult>("DELETE", `/friends/requests/${encodeURIComponent(name)}`),
  removeFriend: (name: string) => call<FriendResult>("DELETE", `/friends/${encodeURIComponent(name)}`),
  ping: (note: string) =>
    call<{ counted: boolean; user: UserStatus }>("POST", "/ping", { source: "manual", note }),
};
