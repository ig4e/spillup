import type { App, Overview } from "./types";

export interface Hint {
  app: string;
  text: string;
  code?: string;
}

/** How to use the shared accounts from each app this machine has, so hints fit what's installed. */
export function hintsFor(apps: App[] | undefined, overview: Overview | undefined): Hint[] {
  const url = overview?.router_url ?? "http://localhost:8317";
  const has = (id: string) => apps?.some((a) => a.id === id && a.installed);
  const hints: Hint[] = [];
  if (has("claude")) {
    hints.push(
      has("ccp")
        ? { app: "Claude Code", text: "Run ccp instead of claude to use the shared accounts; plain claude keeps its own login.", code: "ccp" }
        : { app: "Claude Code", text: "Run spinup setup again to get ccp, Claude Code with the shared accounts." },
    );
  }
  if (has("codex")) hints.push({ app: "Codex", text: "Point Codex's base URL at the router, with the API key from Settings.", code: `${url}/v1` });
  if (has("t3")) hints.push({ app: "T3 Code", text: "Its providers use the router; spillup keeps every machine's T3 linked.", code: url });
  for (const [id, name] of [
    ["opencode", "OpenCode"],
    ["gemini", "Gemini CLI"],
    ["cursor", "Cursor Agent"],
    ["amp", "Amp"],
    ["droid", "Factory Droid"],
  ] as const) {
    if (has(id)) hints.push({ app: name, text: `Use ${name} with an OpenAI-compatible provider at the router, with the API key from Settings.`, code: `${url}/v1` });
  }
  if (!hints.length) hints.push({ app: "Your apps", text: "Point any OpenAI-, Anthropic- or Gemini-compatible app at the router, with the API key from Settings.", code: url });
  return hints;
}
