// Typed client for the Go backend (internal/server).

export interface Folder {
  name: string;
  size: number;
  containers: string[] | null;
  orphan: boolean;
  size_pending?: boolean;
}

export interface Report {
  root: string;
  folders: Folder[];
  total_size: number;
  orphan_size: number;
  orphan_count: number;
  used_count: number;
  scanned_at: string;
}

export interface ScanStatus {
  running: boolean;
  done: number;
  total: number;
  error?: string;
  report: Report | null;
}

export interface TrashItem {
  id: string;
  name: string;
  deleted_at: string;
  size: number;
}

export interface TrashList {
  items: TrashItem[];
  retention_days: number;
}

export interface Result {
  name: string;
  error?: string;
}

/** Thrown when the session is missing or expired. */
export class UnauthorizedError extends Error {
  constructor() {
    super("Session expired");
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (res.status === 401) throw new UnauthorizedError();
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `${res.status} ${res.statusText}`);
  return body as T;
}

function post<T>(url: string, body: unknown): Promise<T> {
  return request<T>(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export const api = {
  session: () => request<{ auth: boolean }>("/api/session"),
  login: async (user: string, password: string) => {
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user, password }),
    });
    if (res.ok) return;
    const body = await res.json().catch(() => ({}));
    const messages: Record<number, string> = {
      401: "Invalid user name or password.",
      429: "Too many failed attempts, try again in a minute.",
    };
    throw new Error(messages[res.status] || body.error || `Error ${res.status}`);
  },
  logout: () => fetch("/api/logout", { method: "POST" }),

  scanStatus: () => request<ScanStatus>("/api/scan"),
  startScan: () => post<ScanStatus>("/api/scan", {}),

  trashFolders: (names: string[]) => post<{ results: Result[] }>("/api/delete", { names }),
  trash: () => request<TrashList>("/api/trash"),
  restore: (ids: string[]) => post<{ results: Result[] }>("/api/trash/restore", { ids }),
  purge: (ids: string[]) => post<{ results: Result[] }>("/api/trash/purge", { ids }),
};
