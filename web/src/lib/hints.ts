import type { App, Overview } from "./types";

export interface Hint {
  app: string;
  /** The app's brand, for its icon. */
  brand?: string;
  text: string;
  code?: string;
}

/** How to use the shared accounts from each app this machine has, so hints fit what's installed. */
export function hintsFor(apps: App[] | undefined, overview: Overview | undefined): Hint[] {
  const url = overview?.router_url;
  const has = (id: string) => apps?.some((a) => a.id === id && a.installed);
  const hints: Hint[] = [];
  if (has("claude")) {
    hints.push(
      has("ccp")
        ? { app: "Claude Code", brand: "claude", text: "Run ccp instead of claude to use the shared accounts; plain claude keeps its own login.", code: "ccp" }
        : { app: "Claude Code", brand: "claude", text: "ccp, Claude Code with the shared accounts, isn't here yet; setting this machine up adds it." },
    );
  }
  if (has("codex")) hints.push({ app: "Codex", brand: "openai", text: "Point Codex's base URL at the router, with the API key from Settings.", code: url ? `${url}/v1` : undefined });
  if (has("t3")) hints.push({ app: "T3 Code", brand: "t3", text: "Use the API key for Claude and Codex provider requests. Add the hub under Usage providers with the dashboard password to see quotas.", code: url });
  for (const [id, name] of [
    ["opencode", "OpenCode"],
    ["gemini", "Gemini CLI"],
    ["cursor", "Cursor Agent"],
    ["amp", "Amp"],
    ["droid", "Factory Droid"],
  ] as const) {
    if (has(id)) hints.push({ app: name, brand: id, text: `Use ${name} with an OpenAI-compatible provider at the router, with the API key from Settings.`, code: url ? `${url}/v1` : undefined });
  }
  if (!hints.length) hints.push({ app: "Your apps", text: "Point any OpenAI-, Anthropic- or Gemini-compatible app at the router, with the API key from Settings.", code: url });
  return hints;
}
