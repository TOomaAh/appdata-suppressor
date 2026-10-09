import { toast } from "svelte-sonner";
import { SvelteSet } from "svelte/reactivity";
import { api, UnauthorizedError, type Result, type ScanStatus, type TrashList } from "./api";
import { count } from "./format";

const POLL_MS = 700;

/** Global UI state: session, scan status (polled while running) and trash. */
class AppState {
  /** null while the session check is pending. */
  loggedIn = $state<boolean | null>(null);
  authEnabled = $state(false);
  view = $state<"folders" | "trash">("folders");
  scan = $state<ScanStatus>({ running: false, done: 0, total: 0, report: null });
  trash = $state<TrashList | null>(null);
  /** Selected orphan folder names. */
  selected = new SvelteSet<string>();
  busy = $state(false);

  #pollTimer: ReturnType<typeof setTimeout> | undefined;

  async init() {
    try {
      const session = await api.session();
      this.authEnabled = session.auth;
      this.loggedIn = true;
    } catch (e) {
      this.loggedIn = false;
      if (!(e instanceof UnauthorizedError)) toast.error(String(e));
      return;
    }
    // The server scans at startup; only start one if it has nothing yet.
    const st = await this.#guard(api.scanStatus());
    if (st && !st.report && !st.running && !st.error) await this.startScan();
    else await this.poll();
    void this.loadTrash();
  }

  async logout() {
    clearTimeout(this.#pollTimer);
    await api.logout().catch(() => {});
    this.loggedIn = false;
    this.scan = { running: false, done: 0, total: 0, report: null };
    this.trash = null;
    this.selected.clear();
  }

  async startScan() {
    const st = await this.#guard(api.startScan());
    if (st) this.#apply(st);
    await this.poll();
  }

  async poll() {
    clearTimeout(this.#pollTimer);
    const wasRunning = this.scan.running;
    const st = await this.#guard(api.scanStatus());
    if (!st) return;
    this.#apply(st);
    if (st.running) this.#pollTimer = setTimeout(() => this.poll(), POLL_MS);
    else if (wasRunning) void this.loadTrash();
  }

  async loadTrash() {
    const trash = await this.#guard(api.trash());
    if (trash) this.trash = trash;
  }

  async trashFolders(names: string[]) {
    const done = await this.#batch(api.trashFolders(names), "moved to trash");
    done.forEach((n) => this.selected.delete(n));
    // The server drops trashed folders from its cached report: no rescan needed.
    await this.poll();
    await this.loadTrash();
  }

  async restore(ids: string[]) {
    await this.#batch(api.restore(ids), "restored", (id) => this.#trashName(id));
    // The server rescans to learn the usage and size of restored folders.
    await this.poll();
    await this.loadTrash();
  }

  async purge(ids: string[]) {
    await this.#batch(api.purge(ids), "permanently deleted", (id) => this.#trashName(id));
    await this.loadTrash();
  }

  #apply(st: ScanStatus) {
    this.scan = st;
    if (!st.report) return;
    // Drop selections that no longer exist or are no longer orphans.
    const orphans = new Set(st.report.folders.filter((f) => f.orphan).map((f) => f.name));
    for (const name of this.selected) if (!orphans.has(name)) this.selected.delete(name);
  }

  #trashName(id: string) {
    return this.trash?.items.find((i) => i.id === id)?.name ?? id;
  }

  /** Runs a batch call, reports per-item results and returns the names that succeeded. */
  async #batch(call: Promise<{ results: Result[] }>, verb: string, label = (n: string) => n) {
    this.busy = true;
    try {
      const res = await this.#guard(call);
      if (!res) return [];
      const failed = res.results.filter((r) => r.error);
      const ok = res.results.length - failed.length;
      if (failed.length) {
        toast.error(`${ok} ${verb}, ${failed.length} failed`, {
          description: failed.map((r) => `${label(r.name)}: ${r.error}`).join("\n"),
          duration: 10000,
        });
      } else {
        toast.success(`${count(ok, "folder")} ${verb}`);
      }
      return res.results.filter((r) => !r.error).map((r) => r.name);
    } finally {
      this.busy = false;
    }
  }

  /** Awaits an API call; on failure shows the error (or the login form) and returns null. */
  async #guard<T>(call: Promise<T>): Promise<T | null> {
    try {
      return await call;
    } catch (e) {
      if (e instanceof UnauthorizedError) {
        clearTimeout(this.#pollTimer);
        this.loggedIn = false;
      } else {
        toast.error(e instanceof Error ? e.message : String(e));
      }
      return null;
    }
  }
}

export const app = new AppState();
