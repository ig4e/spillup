import { Bot, Brain, Code2, Infinity as InfinityIcon, KeyRound, Moon, Orbit, Sparkle, Zap, type LucideIcon } from "lucide-react";
import type { Tint } from "../components/ui";

const looks: Record<string, { icon: LucideIcon; color: Tint; name: string }> = {
  claude: { icon: Bot, color: "orange", name: "Claude" },
  codex: { icon: Code2, color: "graphite", name: "ChatGPT & Codex" },
  antigravity: { icon: Orbit, color: "indigo", name: "Google Antigravity" },
  gemini: { icon: Sparkle, color: "blue", name: "Gemini" },
  kimi: { icon: Moon, color: "teal", name: "Kimi" },
  "kimi-ai": { icon: Moon, color: "teal", name: "Kimi (Moonshot AI)" },
  xai: { icon: Zap, color: "gray", name: "Grok (xAI)" },
  devin: { icon: Brain, color: "green", name: "Devin" },
  meta: { icon: InfinityIcon, color: "blue", name: "Meta AI" },
};

/** How the dashboard shows a provider; a provider it doesn't know yet still gets a sensible look. */
export function providerLook(id: string): { icon: LucideIcon; color: Tint; name: string } {
  return looks[id] ?? { icon: KeyRound, color: "gray", name: id.replace(/(^|-)(\w)/g, (_, s: string, c: string) => (s ? " " : "") + c.toUpperCase()) };
}
