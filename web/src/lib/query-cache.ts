export interface QuerySnapshot<T> {
  data?: T;
  error?: Error;
  refreshError?: Error;
  loading: boolean;
}

const cold = { loading: true };

function equalJSON(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (!a || !b || typeof a !== "object" || typeof b !== "object" || Array.isArray(a) !== Array.isArray(b)) return false;
  const keys = Object.keys(a);
  if (keys.length !== Object.keys(b).length) return false;
  return keys.every(key => Object.hasOwn(b, key) && equalJSON(Reflect.get(a, key), Reflect.get(b, key)));
}

function accessDenied(error: unknown): boolean {
  return error instanceof Error && "status" in error && (error.status === 401 || error.status === 403);
}

export class QueryEntry<T> {
  private state: QuerySnapshot<T> = cold;
  private listeners = new Set<() => void>();
  private observers = new Set<() => void>();
  private pending?: Promise<void>;
  private revision = 0;
  private startedAt = -Infinity;
  lastUsed = Date.now();

  snapshot = () => this.state;

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
      this.lastUsed = Date.now();
    };
  };

  observe(revalidate: () => void) {
    this.observers.add(revalidate);
    return () => { this.observers.delete(revalidate); };
  }

  get idle() {
    return !this.listeners.size && !this.observers.size && !this.pending;
  }

  private publish(next: QuerySnapshot<T>) {
    this.state = next;
    this.listeners.forEach(listener => listener());
  }

  deny(error: Error) {
    this.revision++;
    this.pending = undefined;
    this.startedAt = Date.now();
    this.publish({ error, loading: false });
  }

  invalidate() {
    this.revision++;
    this.startedAt = -Infinity;
    this.pending = undefined;
    this.observers.forEach(revalidate => revalidate());
  }

  fetch(load: () => Promise<T>, staleTime: number): Promise<void> {
    if (this.pending) return this.pending;
    const now = Date.now();
    if (now - this.startedAt < staleTime) return Promise.resolve();
    const revision = this.revision;
    this.startedAt = now;
    this.lastUsed = now;
    // Invoke immediately so even an overlapping machine switch cannot retarget this request.
    this.pending = load().then(data => {
      if (revision !== this.revision) return;
      const prev = this.state;
      if (!prev.loading && !prev.error && !prev.refreshError && equalJSON(prev.data, data)) return;
      this.publish({ data, loading: false });
    }).catch((err: unknown) => {
      if (revision !== this.revision) return;
      const error = err instanceof Error ? err : new Error(String(err));
      if (this.state.data === undefined || accessDenied(error)) this.publish({ error, loading: false });
      else this.publish({ data: this.state.data, refreshError: error, loading: false });
    }).finally(() => {
      if (revision === this.revision) this.pending = undefined;
    });
    return this.pending;
  }
}

/** Entries live only in this tab. Unobserved machine and resource data age out on access. */
export class QueryCache {
  private entries = new Map<string, QueryEntry<unknown>>();

  entry<T>(key: string): QueryEntry<T> {
    const now = Date.now();
    let entry = this.entries.get(key);
    if (!entry || (entry.idle && now - entry.lastUsed >= 5 * 60_000)) {
      for (const [name, cached] of this.entries) {
        if (cached.idle && now - cached.lastUsed >= 5 * 60_000) this.entries.delete(name);
      }
      if (this.entries.size >= 128) {
        const idle = [...this.entries].filter(([, value]) => value.idle).sort((a, b) => a[1].lastUsed - b[1].lastUsed);
        for (const [name] of idle.slice(0, this.entries.size - 127)) this.entries.delete(name);
      }
      entry = new QueryEntry();
      this.entries.set(key, entry);
    }
    entry.lastUsed = now;
    return entry as QueryEntry<T>;
  }

  invalidate() {
    this.entries.forEach(entry => entry.invalidate());
  }

  accessRefused(path: string, error: Error & { status: number }) {
    if (!accessDenied(error)) return;
    const peer = path.match(/^remote\/[^/]+\//)?.[0];
    const route = peer ? path.slice(peer.length) : path;
    const sessionDenied = error.message === "open the dashboard with `spinup ui` to sign in"
      || error.message === "spillup's dashboard answers only devices signed in to your Tailscale account";
    const peerDenied = !!peer && error.message === "missing or wrong key for this machine's dashboard";
    // Overview checks dashboard admission. Individual capabilities, such as the terminal, can refuse access independently.
    if (!sessionDenied && route !== "overview" && !peerDenied) return;
    for (const [key, entry] of this.entries) {
      if (sessionDenied || !peer || key.startsWith(peer)) entry.deny(error);
    }
  }
}

export const dashboardQueries = new QueryCache();
