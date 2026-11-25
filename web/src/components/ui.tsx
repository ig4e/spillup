import { Skeleton, Spinner, Switch } from "@heroui/react";
import { Button } from "./Button";
import { Apple, Check, ChevronLeft, ChevronRight, Copy, Laptop, Monitor, Server, Smartphone, type LucideIcon } from "lucide-react";
import { createContext, useContext, useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { navigate, useDesktop } from "../lib/hooks";
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

interface TitleProps {
  title: string;
  subtitle?: ReactNode;
  actions?: ReactNode;
  center?: ReactNode;
  bleed?: boolean;
}

/** Where the phone's back button goes from this page; null on the pages the tab bar opens. */
export const BackAction = createContext<(() => void) | null>(null);

/** The page's title: a toolbar on the desktop, an iOS navigation bar over a large title on a phone. */
export function LargeTitle(props: TitleProps) {
  return useDesktop() ? <Toolbar {...props} /> : <NavBar {...props} />;
}

/** The page's toolbar, as in Tahoe's apps: back and forward, the title, a view switcher in the middle,
 * and the page's actions, on glass that stays put while the page scrolls under it. Full-width pages use
 * the same bar, flush with their content. */
function Toolbar({
  title,
  subtitle,
  actions,
  center,
  bleed,
}: TitleProps) {
  return (
    <>
      <header
        className={`glass-toolbar sticky top-0 z-10 grid min-h-[52px] grid-cols-[1fr_auto_1fr] items-center gap-3 px-6 py-2 ${bleed ? "-mx-5 -mt-5 mb-3" : "toolbar-span mb-2"}`}
      >
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex shrink-0 overflow-hidden rounded-full bg-surface shadow-[0_0_0_0.5px_var(--border)]">
            <button type="button" aria-label="Back" onClick={() => window.history.back()} className="px-2 py-1.5 text-muted hover:bg-surface-hover hover:text-foreground">
              <ChevronLeft className="size-4" />
            </button>
            <button type="button" aria-label="Forward" onClick={() => window.history.forward()} className="px-2 py-1.5 text-muted hover:bg-surface-hover hover:text-foreground">
              <ChevronRight className="size-4" />
            </button>
          </div>
          <h1 className="min-w-0 truncate text-[15px] font-semibold tracking-[-0.01em]">{title}</h1>
        </div>
        <div className="flex justify-center">{center}</div>
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">{actions}</div>
      </header>
      {subtitle ? <p className={`max-w-2xl text-[13px] text-muted ${bleed ? "mb-3 px-1" : "mb-6"}`}>{subtitle}</p> : bleed ? null : <div className="mb-4" />}
    </>
  );
}

/** iOS's navigation bar: clear over the large title, glass with a small title once the large one scrolls under it. */
function NavBar({ title, subtitle, actions, center, bleed }: TitleProps) {
  const back = useContext(BackAction);
  const bar = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const scrolled = useScrolledUnder(heading, bar);
  return (
    <>
      <header
        ref={bar}
        data-scrolled={scrolled || undefined}
        className={`nav-bar sticky top-0 z-20 grid grid-cols-[1fr_auto_1fr] items-center gap-2 pt-[env(safe-area-inset-top)] ${bleed ? "" : "-mx-4 px-4 sm:-mx-10 sm:px-10"}`}
      >
        <div className={`flex items-center ${back ? "h-12" : "h-5"}`}>
          {back ? (
            <button type="button" aria-label="Back" onClick={back} className="glass-button flex size-10 items-center justify-center rounded-full text-foreground active:scale-95">
              <ChevronLeft className="size-[22px]" strokeWidth={2.4} />
            </button>
          ) : null}
        </div>
        <div className={`min-w-0 truncate text-[17px] font-semibold transition-opacity duration-200 ${scrolled ? "opacity-100" : "opacity-0"}`}>{title}</div>
        <div />
      </header>
      <div className={bleed ? "mb-3" : "mb-6"}>
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <h1 ref={heading} className="min-w-0 text-[34px] leading-[41px] font-bold tracking-[0.01em] break-words">
            {title}
          </h1>
          {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
        </div>
        {subtitle ? <p className="mt-1.5 text-[15px] leading-snug text-muted">{subtitle}</p> : null}
        {center ? <div className="nav-center mt-4">{center}</div> : null}
      </div>
    </>
  );
}

function useScrolledUnder(target: RefObject<HTMLElement | null>, cover: RefObject<HTMLElement | null>) {
  const [under, setUnder] = useState(false);
  useEffect(() => {
    const el = target.current;
    if (!el) return;
    const io = new IntersectionObserver(([e]) => setUnder(!e.isIntersecting && e.boundingClientRect.top < (cover.current?.offsetHeight ?? 0) + 1), {
      rootMargin: `-${cover.current?.offsetHeight ?? 0}px 0px 0px 0px`,
    });
    io.observe(el);
    return () => io.disconnect();
  }, [target, cover]);
  return under;
}

/** A macOS segmented control: a light track, the chosen segment raised on it. */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  label,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { id: T; label: ReactNode }[];
  label: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="segmented inline-flex items-center rounded-[7px] p-[2px]">
      {options.map((o) => (
        <button
          key={o.id}
          type="button"
          role="radio"
          aria-checked={o.id === value}
          onClick={() => onChange(o.id)}
          className={`segment h-[22px] rounded-[5px] px-3 text-[12px] font-medium whitespace-nowrap outline-none focus-visible:ring-2 focus-visible:ring-accent/50 ${
            o.id === value ? "segment--on" : ""
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** An inset grouped list, as in System Settings: a heading, rounded rows split by hairlines, a footnote. */
export function Group({ title, footer, children, className = "" }: { title?: ReactNode; footer?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`mb-7 ${className}`}>
      {title ? <h2 className="mb-1.5 px-4 text-[13px] font-semibold text-foreground/80 lg:px-1">{title}</h2> : null}
      <div className="group-list overflow-hidden rounded-xl bg-surface">
        {children}
      </div>
      {footer ? <p className="mt-1.5 px-4 text-[13px] leading-relaxed text-muted lg:px-1 lg:text-xs">{footer}</p> : null}
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
  busy,
}: {
  icon?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  value?: ReactNode;
  accessory?: ReactNode;
  to?: string;
  onPress?: () => void;
  tone?: "danger" | "accent";
  /** busy shows a spinner and ignores presses while the row's action runs. */
  busy?: boolean;
}) {
  const pressable = !!(to || onPress) && !busy;
  const body = (
    <>
      {icon}
      <div className="min-w-0 flex-1 sm:flex sm:items-center sm:gap-3">
        <div className="min-w-0 flex-1">
          <div className={`break-words text-[16px] leading-snug lg:text-[14px] ${tone === "danger" ? "text-danger" : tone === "accent" ? "text-accent" : ""}`}>{title}</div>
          {subtitle ? <div className="mt-0.5 break-words text-[13px] leading-snug text-muted lg:text-xs">{subtitle}</div> : null}
        </div>
        {value !== undefined ? <div className="mt-0.5 min-w-0 break-words text-[13px] text-muted sm:mt-0 sm:max-w-[55%] sm:text-right sm:text-[14px]">{value}</div> : null}
      </div>
      {accessory ? <div className="flex min-w-0 max-w-[58%] items-center justify-end">{accessory}</div> : null}
      {busy ? <Spinner size="sm" color="current" className="shrink-0 text-muted" /> : null}
      {to && !busy ? <ChevronRight className="size-4 shrink-0 text-muted/60" /> : null}
    </>
  );
  const cls = `group-row relative flex min-h-11 w-full items-center gap-3 px-4 py-2 text-left ${icon ? "group-row--icon" : ""}`;
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

export const holdName: Record<Hold, string> = { hub: "Main", standby: "Worker + Standby", never: "Worker" };

export function osTile(os?: string): { icon: LucideIcon; color: Tint } {
  const o = (os ?? "").toLowerCase();
  if (o.includes("mac") || o.includes("darwin")) return { icon: Apple, color: "graphite" };
  if (o.includes("ios") || o.includes("android")) return { icon: Smartphone, color: "green" };
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

export function Mono({ children, copy, compact = false }: { children: string; copy?: boolean; compact?: boolean }) {
  return (
    <span className="inline-flex max-w-full items-center gap-0.5 align-middle">
      <span className={`min-w-0 rounded-md bg-surface-secondary px-1.5 py-0.5 text-left font-mono text-[12px] ${compact ? "truncate" : "break-all whitespace-normal sm:truncate"}`}>{children}</span>
      {copy ? <CopyButton value={children} /> : null}
    </span>
  );
}

export function Widget({ label, value, detail, icon }: { label: string; value: ReactNode; detail?: ReactNode; icon: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 rounded-xl bg-surface p-4">
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

/** An iOS-style switch. HeroUI only makes it clickable inside Switch.Content, so every switch goes through here. */
export function Toggle({
  isSelected,
  onChange,
  label,
  isDisabled,
  children,
}: {
  isSelected: boolean;
  onChange: (on: boolean) => void;
  label: string;
  isDisabled?: boolean;
  children?: ReactNode;
}) {
  return (
    <Switch size="sm" aria-label={label} isSelected={isSelected} isDisabled={isDisabled} onChange={onChange}>
      <Switch.Content>
        {children}
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  );
}

/** One choice in a list of options, as in System Settings: a checkmark marks the chosen one. */
export function Choice({
  icon,
  title,
  subtitle,
  chosen,
  onChoose,
  disabled,
}: {
  icon?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  chosen: boolean;
  onChoose?: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={chosen}
      disabled={disabled}
      onClick={() => !chosen && onChoose?.()}
      className={`group-row relative flex min-h-11 w-full items-center gap-3 px-4 py-2.5 text-left outline-none transition-colors ${
        icon ? "group-row--icon" : ""
      } ${disabled ? "cursor-not-allowed opacity-50" : "hover:bg-surface-hover focus-visible:bg-surface-hover"}`}
    >
      {icon}
      <div className="min-w-0 flex-1">
        <div className="text-[16px] lg:text-[14px]">{title}</div>
        {subtitle ? <div className="mt-0.5 text-xs whitespace-normal text-muted">{subtitle}</div> : null}
      </div>
      {chosen ? <Check className="size-4 shrink-0 text-accent" strokeWidth={2.6} /> : <span className="size-4 shrink-0" />}
    </button>
  );
}
