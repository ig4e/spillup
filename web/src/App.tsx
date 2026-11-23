import { Alert, Dropdown, Label, Toast } from "@heroui/react";
import { ChevronsUpDown, Eye, Gauge, KeyRound, Network, Server, Settings, Sparkles, Stethoscope, type LucideIcon } from "lucide-react";
import { createContext, useContext } from "react";
import { Dot, IconTile, holdName, type Tint } from "./components/ui";
import { api, setRemote } from "./lib/api";
import { navigate, usePath, useQuery, useRemote, type Query } from "./lib/hooks";
import type { Overview } from "./lib/types";
import { AccountsPage } from "./pages/Accounts";
import { DoctorPage } from "./pages/Doctor";
import { MachinesPage } from "./pages/Machines";
import { OverviewPage } from "./pages/Overview";
import { ProxyPage } from "./pages/Proxy";
import { SettingsPage } from "./pages/Settings";
import { SkillsPage } from "./pages/Skills";

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
  wide?: boolean;
}

const pages: Page[] = [
  { path: "/", label: "Overview", icon: Gauge, color: "blue", page: OverviewPage },
  { path: "/machines", label: "Machines", icon: Network, color: "green", page: MachinesPage },
  { path: "/accounts", label: "Accounts", icon: KeyRound, color: "orange", page: AccountsPage },
  { path: "/proxy", label: "CLIProxyAPI", icon: Server, color: "teal", page: ProxyPage, wide: true },
  { path: "/skills", label: "Skills", icon: Sparkles, color: "purple", page: SkillsPage },
  { path: "/doctor", label: "Doctor", icon: Stethoscope, color: "red", page: DoctorPage },
  { path: "/settings", label: "Settings", icon: Settings, color: "gray", page: SettingsPage },
];

export default function App() {
  const remote = useRemote();
  return <Fleet key={remote ?? ""} remote={remote} />;
}

/** Everything shown for one machine: remounted when the dashboard switches machines, so nothing stale stays. */
function Fleet({ remote }: { remote: string | null }) {
  const overview = useQuery(api.overview, 3000);
  const path = usePath();
  const current = pages.find((p) => p.path === path) ?? pages[0];
  const Page = current.page;

  return (
    <OverviewContext.Provider value={overview}>
      <div className="flex min-h-screen">
        <Sidebar path={current.path} />
        <main className="min-w-0 flex-1">
          <MobileBar path={current.path} />
          <div className={`mx-auto w-full px-5 py-8 sm:px-10 lg:py-12 ${current.wide ? "max-w-none" : "max-w-[860px]"}`}>
            {remote ? <RemoteBanner name={remote} /> : null}
            <ConnectionNotice remote={remote} />
            <Page />
          </div>
        </main>
      </div>
      <Toast.Provider placement="top end" />
    </OverviewContext.Provider>
  );
}

function RemoteBanner({ name }: { name: string }) {
  return (
    <div className="mb-6 flex items-center gap-3 rounded-xl bg-accent px-4 py-2.5 text-[13px] text-white shadow-sm">
      <Eye className="size-4" />
      <span className="flex-1">
        Viewing <b>{name}</b>’s dashboard from this machine. Keys and sign-ins stay on {name}.
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
  if (remote && !signedOut) {
    return (
      <Alert status="danger" className="mb-6">
        <Alert.Indicator />
        <Alert.Content>
          <Alert.Title>{remote} isn't answering</Alert.Title>
          <Alert.Description>{error.message}. It needs spillup 0.3.3 or newer and to be online on Tailscale.</Alert.Description>
        </Alert.Content>
      </Alert>
    );
  }
  return (
    <Alert status={signedOut ? "warning" : "danger"} className="mb-6">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Title>{signedOut ? "Signed out" : "spinup isn't answering"}</Alert.Title>
        <Alert.Description>
          {signedOut ? "Run spinup ui in a terminal to open a signed-in window." : `Is spinup ui still running? (${error.message})`}
        </Alert.Description>
      </Alert.Content>
    </Alert>
  );
}

function SidebarLink({ page, active }: { page: Page; active: boolean }) {
  return (
    <a
      href={page.path}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        navigate(page.path);
      }}
      aria-current={active ? "page" : undefined}
      className={`flex items-center gap-2.5 rounded-md px-2 py-[5px] text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-accent/60 ${
        active ? "bg-accent text-white" : "text-foreground hover:bg-foreground/[0.06]"
      }`}
    >
      <IconTile icon={page.icon} color={page.color} size="sm" />
      {page.label}
    </a>
  );
}

function Sidebar({ path }: { path: string }) {
  return (
    <aside className="sticky top-0 hidden h-screen w-[248px] shrink-0 flex-col border-r border-separator bg-[var(--sidebar)] px-3 pt-5 pb-4 backdrop-blur-2xl backdrop-saturate-150 lg:flex">
      <MachineCard />
      <nav className="mt-4 flex flex-col gap-0.5">
        {pages.slice(0, 6).map((p) => (
          <SidebarLink key={p.path} page={p} active={path === p.path} />
        ))}
        <div className="my-2 h-px bg-separator" />
        <SidebarLink page={pages[6]} active={path === pages[6].path} />
      </nav>
      <HolderStatus />
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
  const canSwitch = !!local.data?.can_manage && (others.length > 0 || !!remote);

  const card = (
    <span className="flex w-full items-center gap-3 rounded-lg px-2 py-1.5 text-left hover:bg-foreground/[0.06]">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-gradient-to-b from-[#a1a1a6] to-[#6e6e73] text-[17px] font-semibold text-white uppercase">
        {name.charAt(0)}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[14px] font-semibold">{name}</span>
        <span className="block truncate text-xs text-muted">
          {data ? (data.installed ? `${holdName[data.hold]}${remote ? " · remote" : " · spillup"}` : "Not set up yet") : "…"}
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
        <Dropdown.Menu
          aria-label="Show a machine's dashboard"
          onAction={(key) => {
            setRemote(key === "__self" ? null : String(key));
            navigate("/");
          }}
        >
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
    <div className="mt-auto flex items-center gap-2 px-2 text-xs text-muted">
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
