import { Alert, Dropdown, Label, Toast, Tooltip } from "@heroui/react";
import {
  Activity,
  ChevronsUpDown,
  CloudDownload,
  Eye,
  Gauge,
  Search,
  KeyRound,
  LogIn,
  Network,
  PanelLeft,
  Server,
  Settings,
  Share2,
  Sparkles,
  Stethoscope,
  Waypoints,
  type LucideIcon,
} from "lucide-react";
import { createContext, useContext, useEffect, useState } from "react";
import { Dot, IconTile, holdName, type Tint } from "./components/ui";
import { api, setRemote } from "./lib/api";
import { navigate, useBusy, usePath, useQuery, useRemote, type Query } from "./lib/hooks";
import type { Overview } from "./lib/types";
import { AccountsPage } from "./pages/Accounts";
import { ActivityPage } from "./pages/Activity";
import { DoctorPage } from "./pages/Doctor";
import { MachinesPage } from "./pages/Machines";
import { OverviewPage } from "./pages/Overview";
import { ProxyPage } from "./pages/Proxy";
import { SettingsPage } from "./pages/Settings";
import { SignInPage } from "./pages/SignIn";
import { SkillsPage } from "./pages/Skills";
import { T3Page } from "./pages/T3";
import { TailscalePage } from "./pages/Tailscale";
import { UpdatesPage } from "./pages/Updates";

const OverviewContext = createContext<Query<Overview> | null>(null);

export function useOverview() {
  const q = useContext(OverviewContext);
  if (!q) throw new Error("useOverview outside the app");
  return q;
}

interface Page {
  path: string;
  label: string;
  icon: LucideIcon;
  color: Tint;
  page: () => React.ReactNode;
  /** Full-bleed pages use the whole window, for embedded dashboards. */
  bleed?: boolean;
}

const sections: { title: string; pages: Page[] }[] = [
  {
    title: "Fleet",
    pages: [
      { path: "/", label: "Overview", icon: Gauge, color: "blue", page: OverviewPage },
      { path: "/machines", label: "Machines", icon: Network, color: "green", page: MachinesPage },
      { path: "/activity", label: "Activity", icon: Activity, color: "teal", page: ActivityPage, bleed: true },
    ],
  },
  {
    title: "Accounts",
    pages: [
      { path: "/accounts", label: "Accounts", icon: KeyRound, color: "orange", page: AccountsPage },
      { path: "/sign-in", label: "Sign-In", icon: LogIn, color: "pink", page: SignInPage },
      { path: "/proxy", label: "CLIProxyAPI", icon: Server, color: "teal", page: ProxyPage, bleed: true },
    ],
  },
  {
    title: "Integrations",
    pages: [
      { path: "/t3", label: "T3 Code", icon: Share2, color: "indigo", page: T3Page },
      { path: "/tailscale", label: "Tailscale", icon: Waypoints, color: "graphite", page: TailscalePage },
    ],
  },
  { title: "Agents", pages: [{ path: "/skills", label: "Skills", icon: Sparkles, color: "purple", page: SkillsPage }] },
  {
    title: "System",
    pages: [
      { path: "/doctor", label: "Doctor", icon: Stethoscope, color: "red", page: DoctorPage },
      { path: "/updates", label: "Updates", icon: CloudDownload, color: "blue", page: UpdatesPage },
      { path: "/settings", label: "Settings", icon: Settings, color: "gray", page: SettingsPage },
    ],
  },
];

const pages = sections.flatMap((s) => s.pages);

export default function App() {
  const remote = useRemote();
  return <Fleet key={remote ?? ""} remote={remote} />;
}

function useCollapsed(): [boolean, () => void] {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("sidebar") === "collapsed");
  useEffect(() => localStorage.setItem("sidebar", collapsed ? "collapsed" : "open"), [collapsed]);
  return [collapsed, () => setCollapsed((c) => !c)];
}

/** Everything shown for one machine: remounted when the dashboard switches machines, so nothing stale stays. */
function Fleet({ remote }: { remote: string | null }) {
  const overview = useQuery(api.overview, 3000);
  const path = usePath();
  const [collapsed, toggle] = useCollapsed();
  const current = pages.find((p) => p.path === path) ?? pages[0];
  const Page = current.page;

  return (
    <OverviewContext.Provider value={overview}>
      <div className="flex min-h-screen">
        <Sidebar path={current.path} collapsed={collapsed} onToggle={toggle} />
        <main className="min-w-0 flex-1">
          <MobileBar path={current.path} />
          {current.bleed ? (
            <div className="flex h-screen flex-col p-4 lg:p-5">
              {remote ? <RemoteBanner name={remote} /> : null}
              <Page />
            </div>
          ) : (
            <div className="mx-auto w-full max-w-[760px] px-5 pb-12 sm:px-10">
              {remote ? <RemoteBanner name={remote} /> : null}
              <ConnectionNotice remote={remote} />
              <Page />
            </div>
          )}
        </main>
      </div>
      <BusyBar />
      <Toast.Provider placement="top end" />
    </OverviewContext.Provider>
  );
}

function BusyBar() {
  return useBusy() ? <div className="busy-bar" role="progressbar" aria-label="Working…" /> : null;
}

function RemoteBanner({ name }: { name: string }) {
  return (
    <div className="mb-5 flex items-center gap-3 rounded-xl bg-accent px-4 py-2.5 text-[13px] text-white shadow-sm">
      <Eye className="size-4" />
      <span className="flex-1">
        Viewing <b>{name}</b> from this machine. Keys and sign-ins stay on {name}.
      </span>
      <button type="button" className="rounded-md bg-white/20 px-2.5 py-1 font-medium hover:bg-white/30" onClick={() => setRemote(null)}>
        Back to This Machine
      </button>
    </div>
  );
}

function ConnectionNotice({ remote }: { remote: string | null }) {
  const { error } = useOverview();
  if (!error) return null;
  const signedOut = (error as { status?: number }).status === 401;
  return (
    <Alert status={signedOut ? "warning" : "danger"} className="mb-6">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Title>{signedOut ? "Signed out" : remote ? `${remote} isn't answering` : "spillup isn't answering"}</Alert.Title>
        <Alert.Description>
          {signedOut
            ? "Run spinup ui in a terminal to open a signed-in window."
            : remote
              ? `${error.message}. It needs a recent spillup and to be online on Tailscale.`
              : `Is spillup's service running? (${error.message})`}
        </Alert.Description>
      </Alert.Content>
    </Alert>
  );
}

function SidebarLink({ page, active, collapsed }: { page: Page; active: boolean; collapsed?: boolean }) {
  const link = (
    <a
      href={page.path}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        navigate(page.path);
      }}
      aria-current={active ? "page" : undefined}
      aria-label={collapsed ? page.label : undefined}
      className={`flex items-center gap-2.5 rounded-[8px] py-[5px] text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-accent/60 ${
        collapsed ? "justify-center px-1" : "px-2"
      } ${active ? "bg-accent text-white" : "text-foreground hover:bg-foreground/[0.06]"}`}
    >
      <IconTile icon={page.icon} color={page.color} size="sm" />
      {collapsed ? null : <span className="truncate">{page.label}</span>}
    </a>
  );
  if (!collapsed) return link;
  return (
    <Tooltip delay={200}>
      <Tooltip.Trigger className="block">{link}</Tooltip.Trigger>
      <Tooltip.Content placement="right">{page.label}</Tooltip.Content>
    </Tooltip>
  );
}

function Sidebar({ path, collapsed, onToggle }: { path: string; collapsed: boolean; onToggle: () => void }) {
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const shown = sections
    .map((section) => ({ ...section, pages: section.pages.filter((p) => !q || p.label.toLowerCase().includes(q) || section.title.toLowerCase().includes(q)) }))
    .filter((section) => section.pages.length);
  return (
    <aside
      className={`glass-sidebar sticky top-2 z-20 m-2 hidden h-[calc(100vh-16px)] shrink-0 flex-col rounded-[18px] pt-3 pb-3 transition-[width] duration-200 lg:flex ${
        collapsed ? "w-[60px] px-2" : "w-[232px] px-2.5"
      }`}
    >
      <div className={`mb-2 flex items-center gap-1.5 ${collapsed ? "justify-center" : ""}`}>
        {collapsed ? null : (
          <label className="flex h-7 min-w-0 flex-1 items-center gap-1.5 rounded-[8px] bg-foreground/[0.06] px-2 text-muted focus-within:ring-2 focus-within:ring-accent/50">
            <Search className="size-3.5 shrink-0" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search"
              aria-label="Search the sidebar"
              className="min-w-0 flex-1 bg-transparent text-[13px] text-foreground outline-none placeholder:text-muted"
            />
          </label>
        )}
        <button
          type="button"
          onClick={onToggle}
          aria-label={collapsed ? "Show the sidebar" : "Hide the sidebar"}
          className="rounded-md p-1.5 text-muted outline-none hover:bg-foreground/[0.06] hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/60"
        >
          <PanelLeft className="size-4" />
        </button>
      </div>
      {collapsed || q ? null : <MachineCard />}
      <nav className="mt-2 flex flex-1 flex-col overflow-y-auto">
        {shown.map((section, i) => (
          <div key={section.title} className="mb-2">
            {collapsed ? (
              i > 0 ? <div className="mx-2 my-2 h-px bg-separator" /> : null
            ) : (
              <div className="px-2 pt-1 pb-1 text-[11px] font-semibold text-muted">{section.title}</div>
            )}
            <div className="flex flex-col gap-0.5">
              {section.pages.map((p) => (
                <SidebarLink key={p.path} page={p} active={path === p.path} collapsed={collapsed} />
              ))}
            </div>
          </div>
        ))}
        {!shown.length ? <div className="px-2 py-4 text-[13px] text-muted">No results</div> : null}
      </nav>
      {collapsed ? null : <HolderStatus />}
    </aside>
  );
}

function MachineCard() {
  const { data } = useOverview();
  const remote = useRemote();
  const local = useQuery(api.localOverview);
  const machines = useQuery(api.localMachines, 15000);
  const name = data?.name || "This machine";
  const others = (machines.data?.machines ?? []).filter((m) => !m.is_self && m.online);
  const canSwitch = others.length > 0 || !!remote;

  const card = (
    <span className="flex w-full items-center gap-3 rounded-lg px-2 py-1.5 text-left hover:bg-foreground/[0.06]">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-gradient-to-b from-[#a1a1a6] to-[#6e6e73] text-[17px] font-semibold text-white uppercase">
        {name.charAt(0)}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[14px] font-semibold">{name}</span>
        <span className="block truncate text-xs text-muted">
          {data ? (data.installed ? `${holdName[data.hold]}${remote ? " · viewing" : ""}` : "Not set up yet") : "…"}
        </span>
      </span>
      {canSwitch ? <ChevronsUpDown className="size-4 shrink-0 text-muted" /> : null}
    </span>
  );
  if (!canSwitch) {
    return (
      <button type="button" onClick={() => navigate("/settings")} className="outline-none focus-visible:ring-2 focus-visible:ring-accent/60">
        {card}
      </button>
    );
  }
  return (
    <Dropdown>
      <Dropdown.Trigger className="w-full rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-accent/60">{card}</Dropdown.Trigger>
      <Dropdown.Popover placement="bottom start" className="min-w-56">
        <Dropdown.Menu aria-label="Show a machine's dashboard" onAction={(key) => setRemote(key === "__self" ? null : String(key))}>
          <Dropdown.Item id="__self" textValue="This machine">
            <Label>{local.data?.name || "This machine"} (this one)</Label>
          </Dropdown.Item>
          {others.map((m) => (
            <Dropdown.Item key={m.name} id={m.name} textValue={m.name}>
              <Label>{m.name}</Label>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}

function HolderStatus() {
  const { data } = useOverview();
  const s = data?.service;
  const leader = s?.leading ? data?.name : s?.leader || s?.leader_addr?.split(":")[0];
  return (
    <div className="mt-2 flex items-center gap-2 px-2 text-xs text-muted">
      <Dot tone={leader ? "success" : "warning"} />
      <span className="truncate">{leader ? `Accounts on ${leader}` : data?.installed === false ? "Not set up" : "No holder yet"}</span>
    </div>
  );
}

function MobileBar({ path }: { path: string }) {
  return (
    <div className="sticky top-0 z-20 flex gap-1 overflow-x-auto border-b border-separator bg-[var(--sidebar)] px-3 py-2 backdrop-blur-2xl lg:hidden">
      {pages.map((p) => (
        <SidebarLink key={p.path} page={p} active={path === p.path} />
      ))}
    </div>
  );
}
