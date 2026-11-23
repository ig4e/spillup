import { Alert, Toast } from "@heroui/react";
import { Gauge, KeyRound, Network, Settings, Sparkles, Stethoscope, type LucideIcon } from "lucide-react";
import { createContext, useContext } from "react";
import { Dot, IconTile, holdName, type Tint } from "./components/ui";
import { api } from "./lib/api";
import { navigate, usePath, useQuery, type Query } from "./lib/hooks";
import type { Overview } from "./lib/types";
import { AccountsPage } from "./pages/Accounts";
import { DoctorPage } from "./pages/Doctor";
import { MachinesPage } from "./pages/Machines";
import { OverviewPage } from "./pages/Overview";
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
}

const pages: Page[] = [
  { path: "/", label: "Overview", icon: Gauge, color: "blue", page: OverviewPage },
  { path: "/machines", label: "Machines", icon: Network, color: "green", page: MachinesPage },
  { path: "/accounts", label: "Accounts", icon: KeyRound, color: "orange", page: AccountsPage },
  { path: "/skills", label: "Skills", icon: Sparkles, color: "purple", page: SkillsPage },
  { path: "/doctor", label: "Doctor", icon: Stethoscope, color: "red", page: DoctorPage },
  { path: "/settings", label: "Settings", icon: Settings, color: "gray", page: SettingsPage },
];

export default function App() {
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
          <div className="mx-auto w-full max-w-[860px] px-5 py-8 sm:px-10 lg:py-12">
            <ConnectionNotice />
            <Page />
          </div>
        </main>
      </div>
      <Toast.Provider placement="top end" />
    </OverviewContext.Provider>
  );
}

function ConnectionNotice() {
  const { error } = useOverview();
  if (!error) return null;
  const signedOut = (error as { status?: number }).status === 401;
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
        {pages.slice(0, 5).map((p) => (
          <SidebarLink key={p.path} page={p} active={path === p.path} />
        ))}
        <div className="my-2 h-px bg-separator" />
        <SidebarLink page={pages[5]} active={path === pages[5].path} />
      </nav>
      <HolderStatus />
    </aside>
  );
}

function MachineCard() {
  const { data } = useOverview();
  const name = data?.name || "This machine";
  return (
    <button
      type="button"
      onClick={() => navigate("/settings")}
      className="flex items-center gap-3 rounded-lg px-2 py-1.5 text-left outline-none hover:bg-foreground/[0.06] focus-visible:ring-2 focus-visible:ring-accent/60"
    >
      <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-gradient-to-b from-[#a1a1a6] to-[#6e6e73] text-[17px] font-semibold text-white uppercase">
        {name.charAt(0)}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-[14px] font-semibold">{name}</span>
        <span className="block truncate text-xs text-muted">
          {data ? (data.installed ? `${holdName[data.hold]} · spillup` : "Not set up yet") : "…"}
        </span>
      </span>
    </button>
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
