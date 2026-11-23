import { Button, Skeleton } from "@heroui/react";
import { Apple, Check, ChevronRight, Copy, Laptop, Monitor, Server, Smartphone, type LucideIcon } from "lucide-react";
import { useState, type ReactNode } from "react";
import { navigate } from "../lib/hooks";
import type { Hold } from "../lib/types";

/** Apple's system colors, for the rounded icon tiles in front of rows. */
export const tint = {
  blue: "#0a84ff",
  green: "#30c759",
  orange: "#ff9500",
  red: "#ff3b30",
  purple: "#af52de",
  indigo: "#5e5ce6",
  teal: "#30b0c7",
  pink: "#ff2d55",
  yellow: "#ffcc00",
  gray: "#8e8e93",
  graphite: "#636366",
} as const;

export type Tint = keyof typeof tint;

export function IconTile({ icon: Icon, color, size = "md" }: { icon: LucideIcon; color: Tint; size?: "sm" | "md" | "lg" }) {
  const box = { sm: "size-6 rounded-[6px]", md: "size-7 rounded-[7px]", lg: "size-12 rounded-[12px]" }[size];
  const glyph = { sm: "size-3.5", md: "size-4", lg: "size-6" }[size];
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center text-white ${box}`}
      style={{ background: `linear-gradient(180deg, color-mix(in oklab, ${tint[color]} 82%, white), ${tint[color]})` }}
    >
      <Icon className={glyph} strokeWidth={2.2} />
    </span>
  );
}

export function LargeTitle({ title, subtitle, actions }: { title: string; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-7 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-[30px] leading-tight font-bold tracking-[-0.022em]">{title}</h1>
        {subtitle ? <p className="mt-1 max-w-2xl text-[15px] text-muted">{subtitle}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}

/** An inset grouped list, as in System Settings: a heading, rounded rows split by hairlines, a footnote. */
export function Group({ title, footer, children, className = "" }: { title?: ReactNode; footer?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`mb-7 ${className}`}>
      {title ? <h2 className="mb-1.5 px-1 text-[13px] font-semibold text-foreground/80">{title}</h2> : null}
      <div className="overflow-hidden rounded-xl bg-surface shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)] [&>*+*]:border-t [&>*+*]:border-separator">
        {children}
      </div>
      {footer ? <p className="mt-1.5 px-1 text-xs leading-relaxed text-muted">{footer}</p> : null}
    </section>
  );
}

export function Row({
  icon,
  title,
  subtitle,
  value,
  accessory,
  to,
  onPress,
  tone,
}: {
  icon?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  value?: ReactNode;
  accessory?: ReactNode;
  to?: string;
  onPress?: () => void;
  tone?: "danger" | "accent";
}) {
  const pressable = !!(to || onPress);
  const body = (
    <>
      {icon}
      <div className="min-w-0 flex-1">
        <div className={`truncate text-[14px] ${tone === "danger" ? "text-danger" : tone === "accent" ? "text-accent" : ""}`}>{title}</div>
        {subtitle ? <div className="mt-0.5 truncate text-xs text-muted">{subtitle}</div> : null}
      </div>
      {value !== undefined ? <div className="shrink-0 text-[14px] text-muted">{value}</div> : null}
      {accessory}
      {to ? <ChevronRight className="size-4 shrink-0 text-muted/60" /> : null}
    </>
  );
  const cls = "flex min-h-11 w-full items-center gap-3 px-4 py-2 text-left";
  if (!pressable) return <div className={cls}>{body}</div>;
  return (
    <button
      type="button"
      className={`${cls} outline-none transition-colors hover:bg-surface-hover focus-visible:bg-surface-hover active:bg-surface-tertiary`}
      onClick={() => (to ? navigate(to) : onPress?.())}
    >
      {body}
    </button>
  );
}

export function Dot({ tone, className = "" }: { tone: "success" | "warning" | "danger" | "muted" | "accent"; className?: string }) {
  const color = { success: "bg-success", warning: "bg-warning", danger: "bg-danger", muted: "bg-muted/50", accent: "bg-accent" }[tone];
  return <span className={`inline-block size-2 shrink-0 rounded-full ${color} ${className}`} />;
}

export const holdName: Record<Hold, string> = { hub: "Main", standby: "Standby", never: "Uses accounts" };

export function osTile(os?: string): { icon: LucideIcon; color: Tint } {
  const o = (os ?? "").toLowerCase();
  if (o.includes("ios") || o.includes("android")) return { icon: Smartphone, color: "green" };
  if (o.includes("mac") || o.includes("darwin")) return { icon: Apple, color: "graphite" };
  if (o.includes("windows")) return { icon: Monitor, color: "blue" };
  if (o.includes("linux")) return { icon: Laptop, color: "orange" };
  return { icon: Server, color: "gray" };
}

export function osName(os?: string) {
  const o = (os ?? "").toLowerCase();
  if (o === "darwin" || o === "macos") return "macOS";
  if (o === "ios") return "iOS";
  if (!o) return "Unknown";
  return o.charAt(0).toUpperCase() + o.slice(1);
}

export function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      isIconOnly
      size="sm"
      variant="ghost"
      aria-label={label}
      className="size-7 min-w-7"
      onPress={async () => {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 1400);
      }}
    >
      {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5 text-muted" />}
    </Button>
  );
}

export function Mono({ children, copy }: { children: string; copy?: boolean }) {
  return (
    <span className="inline-flex max-w-full items-center gap-0.5 align-middle">
      <span className="truncate rounded-md bg-surface-secondary px-1.5 py-0.5 font-mono text-[12px]">{children}</span>
      {copy ? <CopyButton value={children} /> : null}
    </span>
  );
}

export function Widget({ label, value, detail, icon }: { label: string; value: ReactNode; detail?: ReactNode; icon: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 rounded-2xl bg-surface p-4 shadow-[0_0_0_0.5px_var(--border),0_1px_2px_rgb(0_0_0/0.04)]">
      <div className="flex items-center gap-2 text-xs font-medium text-muted">
        {icon}
        {label}
      </div>
      <div className="truncate text-[26px] leading-none font-semibold tracking-tight">{value}</div>
      {detail ? <div className="truncate text-xs text-muted">{detail}</div> : null}
    </div>
  );
}

export function GroupSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="mb-7 overflow-hidden rounded-xl bg-surface">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-3 border-t border-separator px-4 py-3 first:border-t-0">
          <Skeleton className="size-7 rounded-[7px]" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3 w-1/3 rounded" />
            <Skeleton className="h-2.5 w-1/4 rounded" />
          </div>
        </div>
      ))}
    </div>
  );
}
