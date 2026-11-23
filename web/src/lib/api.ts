import type { Doctor, Done, Keys, Machines, Overview, Skills } from "./types";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

const mocked = import.meta.env.VITE_MOCK === "1";

async function call<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
  if (mocked) {
    const { mock } = await import("./mock");
    return mock(method, path, body) as Promise<T>;
  }
  const res = await fetch(`/api/${path}`, {
    method,
    headers: method === "POST" ? { "Content-Type": "application/json" } : undefined,
    body: method === "POST" ? JSON.stringify(body ?? {}) : undefined,
    credentials: "same-origin",
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(data?.error?.message ?? `${res.status} ${res.statusText}`, res.status);
  }
  return data as T;
}

const post = (path: string, body?: unknown) => call<Done>("POST", path, body);

export const api = {
  overview: () => call<Overview>("GET", "overview"),
  machines: () => call<Machines>("GET", "machines"),
  doctor: () => call<Doctor>("GET", "doctor"),
  skills: () => call<Skills>("GET", "skills"),
  keys: () => call<Keys>("GET", "keys"),

  handoff: (to: string) => post("accounts/handoff", { to }),
  takeover: () => post("accounts/takeover"),
  restartProxy: () => post("accounts/restart"),
  makeMain: (machine: string) => post("machines/main", { machine }),

  setMode: (names: string[], manual: boolean) => post("skills/mode", { names, manual }),
  removeSkills: (names: string[]) => post("skills/remove", { names }),
  restoreSkill: (name: string) => post("skills/restore", { name }),
  adoptSkills: (names: string[]) => post("skills/adopt", { names }),
  addSkills: (repo: string, names: string[], manual: boolean) => post("skills/add", { repo, names, manual }),
  syncSkills: () => post("skills/sync"),
};
