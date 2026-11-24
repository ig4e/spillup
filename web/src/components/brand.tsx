import type { LucideIcon } from "lucide-react";
import { useId } from "react";
import ampColor from "@lobehub/icons-static-svg/icons/amp-color.svg?raw";
import antigravityColor from "@lobehub/icons-static-svg/icons/antigravity-color.svg?raw";
import cursorMono from "@lobehub/icons-static-svg/icons/cursor.svg?raw";
import deepseekMono from "@lobehub/icons-static-svg/icons/deepseek.svg?raw";
import devinColor from "@lobehub/icons-static-svg/icons/devin-color.svg?raw";
import geminiColor from "@lobehub/icons-static-svg/icons/gemini-color.svg?raw";
import grokMono from "@lobehub/icons-static-svg/icons/grok.svg?raw";
import kimiColor from "@lobehub/icons-static-svg/icons/kimi-color.svg?raw";
import metaaiColor from "@lobehub/icons-static-svg/icons/metaai-color.svg?raw";
import mistralColor from "@lobehub/icons-static-svg/icons/mistral-color.svg?raw";
import openaiMono from "@lobehub/icons-static-svg/icons/openai.svg?raw";
import opencodeMono from "@lobehub/icons-static-svg/icons/opencode.svg?raw";
import openrouterMono from "@lobehub/icons-static-svg/icons/openrouter.svg?raw";
import vertexaiColor from "@lobehub/icons-static-svg/icons/vertexai-color.svg?raw";
import android from "simple-icons/icons/android.svg?raw";
import apple from "simple-icons/icons/apple.svg?raw";
import archlinux from "simple-icons/icons/archlinux.svg?raw";
import claudeMark from "simple-icons/icons/claude.svg?raw";
import github from "simple-icons/icons/github.svg?raw";
import githubcopilot from "simple-icons/icons/githubcopilot.svg?raw";
import linux from "simple-icons/icons/linux.svg?raw";
import qwen from "simple-icons/icons/qwen.svg?raw";
import tailscale from "simple-icons/icons/tailscale.svg?raw";
import { IconTile, osTile, type Tint } from "./ui";

/** A mark is an SVG (monochrome ones draw in currentColor) or a short wordmark. */
type Mark = { svg: string } | { text: string };

/** A brand's tile: its mark in `fg` on `bg`, or on a white tile when `bg` is "light". */
interface Brand {
  mark: Mark;
  bg: string;
  fg?: string;
  scale?: number;
}

const windows = '<svg viewBox="0 0 24 24"><path d="M3 3h8.4v8.4H3zM12.6 3H21v8.4h-8.4zM3 12.6h8.4V21H3zM12.6 12.6H21V21h-8.4z"/></svg>';

const claude: Brand = { mark: { svg: claudeMark }, bg: "#d97757" };
const openai: Brand = { mark: { svg: openaiMono }, bg: "#0d0d0d" };
const gemini: Brand = { mark: { svg: geminiColor }, bg: "light", scale: 1.1 };
const kimi: Brand = { mark: { svg: kimiColor }, bg: "#0d0d0d" };
const grok: Brand = { mark: { svg: grokMono }, bg: "#0d0d0d", scale: 1.1 };
const copilot: Brand = { mark: { svg: githubcopilot }, bg: "#0d0d0d" };

const brands: Record<string, Brand> = {
  claude,
  anthropic: claude,
  "claude-code": claude,
  codex: openai,
  openai,
  gemini,
  "gemini-cli": gemini,
  antigravity: { mark: { svg: antigravityColor }, bg: "light", scale: 1.05 },
  vertex: { mark: { svg: vertexaiColor }, bg: "light" },
  kimi,
  "kimi-ai": kimi,
  moonshot: kimi,
  xai: grok,
  grok,
  devin: { mark: { svg: devinColor }, bg: "light", scale: 1.2 },
  meta: { mark: { svg: metaaiColor }, bg: "light", scale: 1.2 },
  qwen: { mark: { svg: qwen }, bg: "#6950ef" },
  mistral: { mark: { svg: mistralColor }, bg: "light" },
  deepseek: { mark: { svg: deepseekMono }, bg: "#4d6bfe", scale: 1.1 },
  "github-copilot": copilot,
  copilot,
  openrouter: { mark: { svg: openrouterMono }, bg: "#6467f2" },
  opencode: { mark: { svg: opencodeMono }, bg: "#0d0d0d" },
  cursor: { mark: { svg: cursorMono }, bg: "#0d0d0d" },
  amp: { mark: { svg: ampColor }, bg: "light" },
  t3: { mark: { text: "T3" }, bg: "#1c1c1e" },
  tailscale: { mark: { svg: tailscale }, bg: "#1c1c1e", scale: 1.2 },
  github: { mark: { svg: github }, bg: "#1c1c1e" },
  windows: { mark: { svg: windows }, bg: "#0078d4", scale: 0.95 },
  linux: { mark: { svg: linux }, bg: "#fcc624", fg: "#1d1d1f" },
  arch: { mark: { svg: archlinux }, bg: "#1793d1" },
  macos: { mark: { svg: apple }, bg: "#1c1c1e", scale: 0.95 },
  ios: { mark: { svg: apple }, bg: "light", fg: "#1d1d1f", scale: 0.95 },
  android: { mark: { svg: android }, bg: "#3ddc84", fg: "#073042", scale: 1.1 },
};

const box = { sm: "size-6 rounded-[6px]", md: "size-7 rounded-[7px]", lg: "size-12 rounded-[12px]" };
const glyph = { sm: 14, md: 16, lg: 26 };
const ring = "shadow-[inset_0_0_0_0.5px_rgba(0,0,0,0.1)] [.dark_&]:shadow-[inset_0_0_0_0.5px_rgba(255,255,255,0.16)]";

/**
 * A brand's mark in the dashboard's rounded icon tile, like macOS System Settings.
 * Unknown brands (providers appear dynamically) get the lucide `icon` in a `color` tile instead.
 */
export function BrandTile({
  brand,
  icon,
  color = "gray",
  size = "md",
  dim,
}: {
  brand?: string;
  icon: LucideIcon;
  color?: Tint;
  size?: "sm" | "md" | "lg";
  dim?: boolean;
}) {
  const b = brand ? brands[brand.toLowerCase()] : undefined;
  if (!b) return <IconTile icon={icon} color={dim ? "gray" : color} size={size} />;
  const light = b.bg === "light";
  const px = Math.round(glyph[size] * (b.scale ?? 1));
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center ${box[size]} ${ring} ${dim ? "opacity-60 grayscale" : ""}`}
      style={{
        color: b.fg ?? "#fff",
        background: light ? "linear-gradient(180deg, #fff, #ececf0)" : `linear-gradient(180deg, color-mix(in oklab, ${b.bg} 84%, white), ${b.bg})`,
      }}
      aria-hidden
    >
      <BrandMark mark={b.mark} px={px} size={size} />
    </span>
  );
}

function BrandMark({ mark, px, size }: { mark: Mark; px: number; size: "sm" | "md" | "lg" }) {
  const id = useId().replace(/[^\w-]/g, "");
  if ("text" in mark) {
    const font = { sm: "text-[10px]", md: "text-[12px]", lg: "text-[20px]" }[size];
    return <span className={`font-bold leading-none tracking-[-0.04em] ${font}`}>{mark.text}</span>;
  }
  const svg = mark.svg.replace(/<title>.*?<\/title>/, "").replaceAll("lobe-icons-", `${id}-`);
  return <span className="inline-flex [&>svg]:size-full [&>svg]:fill-current" style={{ width: px, height: px }} dangerouslySetInnerHTML={{ __html: svg }} />;
}

function osBrand(os?: string) {
  const o = (os ?? "").toLowerCase();
  if (o.includes("mac") || o.includes("darwin")) return "macos";
  if (o.includes("ios")) return "ios";
  if (o.includes("android")) return "android";
  if (o.includes("windows")) return "windows";
  if (o.includes("arch")) return "arch";
  if (o.includes("linux")) return "linux";
  return undefined;
}

/** A machine's operating system as a brand tile, grayed out when the machine is offline. */
export function OsTile({ os, size, dim }: { os?: string; size?: "sm" | "md" | "lg"; dim?: boolean }) {
  const t = osTile(os);
  return <BrandTile brand={osBrand(os)} icon={t.icon} color={t.color} size={size} dim={dim} />;
}
