import { Alert, Dropdown, Label, Toast, Tooltip } from "@heroui/react";
import {
  Activity,
  ChevronRight,
  ChevronsUpDown,
  Ellipsis,
  Eye,
  Gauge,
  Search,
  KeyRound,
  LogIn,
  MonitorSmartphone,
  SquareTerminal,
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
import { BrandTile } from "./components/brand";
import { BackAction, Dot, Group, LargeTitle, Row, holdName, type Tint } from "./components/ui";
import { api, setRemote } from "./lib/api";
import { canGoBack, navigate, useBusy, usePath, useQuery, useRemote, type Query } from "./lib/hooks";
import type { Overview } from "./lib/types";
import { AccountsPage } from "./pages/Accounts";
import { ActivityPage } from "./pages/Activity";
import { DoctorPage } from "./pages/Doctor";
import { MachinesPage } from "./pages/Machines";
import { OverviewPage } from "./pages/Overview";
import { ProxyPage } from "./pages/Proxy";
import { RemotePage } from "./pages/Remote";
import { TerminalPage } from "./pages/Terminal";
import { SettingsPage } from "./pages/Settings";
import { SignInPage } from "./pages/SignIn";
import { SkillsPage } from "./pages/Skills";
import { T3Page } from "./pages/T3";
import { TailscalePage } from "./pages/Tailscale";

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
  /** A brand whose mark replaces the icon, for integrations. */
  brand?: string;
  page: () => React.ReactNode;
  /** Full-bleed pages use the whole window, for embedded dashboards. */
  bleed?: boolean;
  /** Tabs get their own button in the phone's tab bar; every other page is listed under More. */
  tab?: boolean;
  /** shows hides the page while it has nothing to offer, such as Sign-In while only the main machine keeps logins. */
  shows?: (o: Overview) => boolean;
}

type Section = { title: string; pages: Page[] };

const sections: Section[] = [
  {
    title: "Fleet",
    pages: [
      { path: "/", label: "Overview", icon: Gauge, color: "blue", page: OverviewPage, tab: true },
      { path: "/machines", label: "Machines", icon: Network, color: "green", page: MachinesPage, tab: true },
      { path: "/activity", label: "Activity", icon: Activity, color: "teal", page: ActivityPage, bleed: true, tab: true },
      { path: "/desktop", label: "Remote", icon: MonitorSmartphone, color: "indigo", page: RemotePage, bleed: true },
      { path: "/terminal", label: "Terminal", icon: SquareTerminal, color: "graphite", page: TerminalPage, bleed: true, shows: (o) => o.settings.terminals !== false },
    ],
  },
  {
    title: "Accounts",
    pages: [
      { path: "/accounts", label: "Accounts", icon: KeyRound, color: "orange", page: AccountsPage },
      { path: "/sign-in", label: "Sign-In", icon: LogIn, color: "pink", page: SignInPage, shows: (o) => !o.service?.hub_only },
      { path: "/proxy", label: "CLIProxyAPI", icon: Server, color: "teal", page: ProxyPage, bleed: true },
    ],
  },
  {
    title: "Integrations",
    pages: [
      { path: "/t3", label: "T3 Code", icon: Share2, color: "indigo", brand: "t3", page: T3Page },
      { path: "/tailscale", label: "Tailscale", icon: Waypoints, color: "graphite", brand: "tailscale", page: TailscalePage },
    ],
  },
  { title: "Agents", pages: [{ path: "/skills", label: "Skills", icon: Sparkles, color: "purple", page: SkillsPage }] },
  {
    title: "System",
    pages: [
      { path: "/doctor", label: "Doctor & Updates", icon: Stethoscope, color: "red", page: DoctorPage },
      { path: "/settings", label: "Settings", icon: Settings, color: "gray", page: SettingsPage },
    ],
  },
];

const pages = sections.flatMap((s) => s.pages);

const MORE = "/more";
const morePage: Page = { path: MORE, label: "More", icon: Ellipsis, color: "gray", page: MorePage };
const tabs = [...pages.filter((p) => p.tab), morePage];
const moreSections = sections.map((s) => ({ ...s, pages: s.pages.filter((p) => !p.tab) })).filter((s) => s.pages.length);

/** The paths a machine that isn't set up yet shows: setting it up, and what helps with that. */
const beforeSetup = new Set(["/", "/tailscale", "/doctor", "/settings"]);

function pageShows(page: Page, o: Overview | null | undefined) {
  if (!o) return true;
  if (!o.installed) return beforeSetup.has(page.path) || page === morePage;
  return page.shows?.(o) ?? true;
}

/** The sections with only the pages this machine's settings make useful. */
function useShownSections(list: Section[]) {
  const { data } = useOverview();
  return list.map((s) => ({ ...s, pages: s.pages.filter((p) => pageShows(p, data)) })).filter((s) => s.pages.length);
}

function filterSections(list: Section[], query: string) {
  const q = query.trim().toLowerCase();
  return list
    .map((section) => ({ ...section, pages: section.pages.filter((p) => !q || p.label.toLowerCase().includes(q) || section.title.toLowerCase().includes(q)) }))
    .filter((section) => section.pages.length);
}

const backFrom = (page: Page) => (page.tab || page === morePage ? null : () => (canGoBack() ? window.history.back() : navigate(MORE)));

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
  const pagePath = path === "/updates" ? "/doctor" : path;
  const found = pages.find((p) => p.path === pagePath) ?? (path === MORE ? morePage : pages[0]);
  const current = pageShows(found, overview.data) ? found : pages[0];
  const Page = current.page;

  return (
    <OverviewContext.Provider value={overview}>
      <BackAction.Provider value={backFrom(current)}>
        <div className="flex min-h-screen">
          <Sidebar path={current.path} collapsed={collapsed} onToggle={toggle} />
          <main className="@container/main min-w-0 flex-1">
            {current.bleed ? (
              <div className="flex h-dvh flex-col px-4 pb-[calc(env(safe-area-inset-bottom)+84px)] lg:h-screen lg:p-5">
                {remote ? <RemoteBanner name={remote} /> : null}
                <Page />
              </div>
            ) : (
              <div className="mx-auto w-full max-w-[760px] px-4 pb-[calc(env(safe-area-inset-bottom)+104px)] sm:px-10 lg:pb-12">
                {remote ? <RemoteBanner name={remote} /> : null}
                <ConnectionNotice remote={remote} />
                <Page />
              </div>
            )}
          </main>
        </div>
        <TabBar active={current.tab ? current.path : MORE} />
      </BackAction.Provider>
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
      <BrandTile brand={page.brand} icon={page.icon} color={page.color} size="sm" />
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
  const shown = filterSections(useShownSections(sections), q);
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

/** Which machine's dashboard is shown, and the others it can switch to. */
function useMachineSwitch() {
  const { data } = useOverview();
  const remote = useRemote();
  const local = useQuery(api.localOverview);
  const machines = useQuery(api.localMachines, 15000);
  const others = (machines.data?.machines ?? []).filter((m) => !m.is_self && m.online);
  return {
    name: data?.name || "This machine",
    status: data ? (data.installed ? `${holdName[data.hold]}${remote ? " · viewing" : ""}` : "Not set up yet") : "…",
    here: local.data?.name || "This machine",
    others,
    canSwitch: others.length > 0 || !!remote,
  };
}

function MachineMenu({ trigger, triggerClass, here, others }: { trigger: React.ReactNode; triggerClass: string; here: string; others: { name: string }[] }) {
  return (
    <Dropdown>
      <Dropdown.Trigger className={triggerClass}>{trigger}</Dropdown.Trigger>
      <Dropdown.Popover placement="bottom start" className="min-w-56">
        <Dropdown.Menu aria-label="Show a machine's dashboard" onAction={(key) => setRemote(key === "__self" ? null : String(key))}>
          <Dropdown.Item id="__self" textValue="This machine">
            <Label>{here} (this one)</Label>
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

function Avatar({ name, className }: { name: string; className: string }) {
  return (
    <span className={`flex shrink-0 items-center justify-center rounded-full bg-gradient-to-b from-[#a1a1a6] to-[#6e6e73] font-semibold text-white uppercase ${className}`}>
      {name.charAt(0)}
    </span>
  );
}

function MachineCard() {
  const { name, status, here, others, canSwitch } = useMachineSwitch();
  const card = (
    <span className="flex w-full items-center gap-3 rounded-lg px-2 py-1.5 text-left hover:bg-foreground/[0.06]">
      <Avatar name={name} className="size-10 text-[17px]" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[14px] font-semibold">{name}</span>
        <span className="block truncate text-xs text-muted">{status}</span>
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
  return <MachineMenu trigger={card} triggerClass="w-full rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-accent/60" here={here} others={others} />;
}

/** The machine at the top of More, like the Apple Account card at the top of iOS Settings. */
function MachineSwitcher() {
  const { name, status, here, others, canSwitch } = useMachineSwitch();
  const card = (
    <span className="flex min-h-[76px] w-full items-center gap-3.5 px-4 py-3 text-left active:bg-surface-hover">
      <Avatar name={name} className="size-[58px] text-[24px]" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[20px] leading-tight font-semibold">{name}</span>
        <span className="mt-0.5 block truncate text-[13px] text-muted">{canSwitch ? `${status} · Switch machine` : status}</span>
      </span>
      {canSwitch ? <ChevronsUpDown className="size-[18px] shrink-0 text-muted/70" /> : <ChevronRight className="size-[18px] shrink-0 text-muted/60" />}
    </span>
  );
  const cls = "block w-full overflow-hidden rounded-xl bg-surface outline-none focus-visible:ring-2 focus-visible:ring-accent/60";
  return (
    <div className="mb-7">
      {canSwitch ? (
        <MachineMenu trigger={card} triggerClass={cls} here={here} others={others} />
      ) : (
        <button type="button" onClick={() => navigate("/settings")} className={cls}>
          {card}
        </button>
      )}
    </div>
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

/** Every page not in the tab bar, as iOS Settings lists them: the machine first, then a group per section. */
function MorePage() {
  const [query, setQuery] = useState("");
  const shown = filterSections(useShownSections(moreSections), query);
  return (
    <>
      <LargeTitle title="More" />
      <label className="mb-6 -mt-2 flex h-10 items-center gap-2 ios-fill rounded-full px-3.5 text-muted focus-within:ring-2 focus-within:ring-accent/40">
        <Search className="size-[17px] shrink-0" strokeWidth={2.2} />
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search"
          aria-label="Search pages"
          className="min-w-0 flex-1 bg-transparent text-[17px] text-foreground outline-none placeholder:text-muted [&::-webkit-search-cancel-button]:hidden"
        />
      </label>
      {query.trim() ? null : <MachineSwitcher />}
      {shown.map((section) => (
        <Group key={section.title} title={section.title}>
          {section.pages.map((p) => (
            <Row key={p.path} icon={<BrandTile brand={p.brand} icon={p.icon} color={p.color} />} title={p.label} to={p.path} />
          ))}
        </Group>
      ))}
      {!shown.length ? <p className="py-10 text-center text-[15px] text-muted">No Results for “{query.trim()}”</p> : null}
    </>
  );
}

/** iOS 26's tab bar: a floating capsule of glass, the chosen tab tinted with a lens of glass behind it. */
function TabBar({ active }: { active: string }) {
  const index = Math.max(0, tabs.findIndex((t) => t.path === active));
  return (
    <nav aria-label="Pages" className="pointer-events-none fixed inset-x-0 bottom-0 z-30 flex justify-center px-5 pb-[max(env(safe-area-inset-bottom),14px)] lg:hidden">
      <div className="glass-tabbar pointer-events-auto relative flex w-full max-w-[440px] rounded-full p-1">
        <span
          aria-hidden
          className="tab-lens absolute inset-y-1 left-1 rounded-full transition-transform duration-300 ease-[cubic-bezier(0.3,1.3,0.5,1)]"
          style={{ width: `calc((100% - 8px) / ${tabs.length})`, transform: `translateX(${index * 100}%)` }}
        />
        {tabs.map((t) => {
          const on = t.path === active;
          const Icon = t.icon;
          return (
            <a
              key={t.path}
              href={t.path}
              onClick={(e) => {
                e.preventDefault();
                navigate(t.path);
              }}
              aria-current={on ? "page" : undefined}
              className={`relative flex h-[54px] flex-1 flex-col items-center justify-center gap-[3px] rounded-full outline-none focus-visible:ring-2 focus-visible:ring-accent/50 ${
                on ? "text-accent" : "text-foreground"
              }`}
            >
              <Icon className="size-[23px]" strokeWidth={on ? 2.3 : 1.9} />
              <span className="text-[10px] leading-none font-medium tracking-[0.01em]">{t.label}</span>
            </a>
          );
        })}
      </div>
    </nav>
  );
}
