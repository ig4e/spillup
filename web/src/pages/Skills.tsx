import { Alert, Checkbox, Description, Input, Label, Modal, Switch, TextField } from "@heroui/react";
import { Button } from "../components/Button";
import { MCPServers } from "../components/MCPServers";
import { ArchiveRestore, FolderGit2, HardDrive, Plus, RefreshCw, Sparkles } from "lucide-react";
import { useState } from "react";
import { BrandTile } from "../components/brand";
import { Group, GroupSkeleton, IconTile, LargeTitle, Row, Segmented, Toggle } from "../components/ui";
import { useOptimisticSetting } from "../lib/useOptimisticSetting";
import { api, localDashboard } from "../lib/api";
import { capitalize, relative } from "../lib/format";
import { useAction, useQuery, useRemote } from "../lib/hooks";
import type { LocalSkill, RemovedSkill, SharedSkill, SkillSharingAction, Skills } from "../lib/types";

type SkillScope = { name: string; kind: "private" | "shared" | "unmanaged"; fleet?: boolean; path?: string };

const scopeLabels: Record<SkillSharingAction | "adopt", string> = {
  private: "Keep local copy", resume: "Use shared copy", publish: "Publish local copy", adopt: "Share with library",
};
const scopeDetails: Record<SkillSharingAction | "adopt", string> = {
  private: "This machine will keep its own editable copy when the library changes.",
  resume: "Replace this machine’s edits with the current shared skill and follow future changes.",
  publish: "Publish this machine’s skill files to the library for your other machines to use.",
  adopt: "Add this skill to the shared library for your other machines.",
};

const TOKEN_BUDGET = 8000;
const countLabel = (label: string, count?: number) => <>{label}<span className="max-[360px]:sr-only"> {count ?? "–"}</span></>;

export function SkillsPage() {
  const skills = useQuery(api.skills);
  const { pending, run } = useAction();
  const [addScope, setAddScope] = useState<"choose" | "shared" | null>(null);
  const [tab, setTab] = useState<"shared" | "here" | "removed" | "mcp">("shared");
  const mode = useOptimisticSetting(api.skills, skills.data,
    (machine, edit: { name: string; auto: boolean }) => api.setMode(machine, edit.name, edit.auto),
    (previous, edit): Skills => ({
      ...previous,
      shared: previous.shared.map(skill => skill.name === edit.name ? { ...skill, mode: edit.auto ? "auto" : "manual" } : skill),
    }));
  const data = mode.data;
  const remote = useRemote();
  const local = localDashboard && !remote;
  const canManageScope = local && !!data?.sharing_available;
  const [scope, setScope] = useState<SkillScope | null>(null);
  const shared = data?.shared.filter(s => !s.private) ?? [];
  const privateSkills = data?.shared.filter(s => s.private) ?? [];
  const manage = canManageScope ? setScope : undefined;

  return (
    <>
      <LargeTitle
        title="Skills & MCP"
        center={
            <Segmented
              label="Skill lists"
              value={tab}
              onChange={setTab}
              options={[
                { id: "shared", label: countLabel("Shared", data ? shared.length : undefined) },
                { id: "here", label: countLabel("Only Here", data ? data.only_here.length + privateSkills.length : undefined) },
                { id: "removed", label: countLabel("Removed", data?.removed.length) },
                { id: "mcp", label: "MCP" },
              ]}
            />
        }
        actions={
          tab === "mcp" ? undefined : <>
            <Button aria-label="Sync skills" className="max-[400px]:px-2" variant="secondary" size="sm" isPending={pending === "sync"} onPress={() => run("sync", api.syncSkills, skills.reload)}>
              <><RefreshCw className="size-3.5" /><span className="max-[400px]:sr-only">Sync</span></>
            </Button>
            {local ? <Button size="sm" aria-label="Add Skills" className="max-[400px]:px-2" isDisabled={!data || !!skills.refreshError} onPress={() => setAddScope(canManageScope ? "choose" : "shared")}>
              <Plus className="size-3.5" /><span className="max-[400px]:sr-only">Add Skills</span>
            </Button> : null}
          </>
        }
      />

      {tab === "mcp" ? <MCPServers /> : <>
      {skills.error ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>Couldn’t read your skills</Alert.Title>
            <Alert.Description>{skills.error.message}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
      {data?.clashes?.map((c) => (
        <Alert key={c.name} status="warning" className="mb-4">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>Two skills are called “{c.name}”</Alert.Title>
            <Alert.Description>Agents reach only one of them ({c.folders.join(", ")}). Rename one, or remove it.</Alert.Description>
          </Alert.Content>
        </Alert>
      ))}

      {skills.loading ? (
        <GroupSkeleton rows={5} />
      ) : data ? (
        <>
          <Budget data={data} />
          {tab === "shared" ? <SharedList skills={shared} manage={manage} library={data.library} reload={skills.reload} savingMode={mode.pending} setMode={(name, auto) => mode.save({ name, auto })} /> : null}
          {tab === "here" ? <>
            {privateSkills.length ? <Group title="Private skills">{privateSkills.map(s =>
              <Row key={s.name} icon={<IconTile icon={HardDrive} color="indigo" />} title={s.name}
                subtitle={s.path ?? "Private working copy"}
                accessory={<div className="flex flex-wrap items-center justify-end gap-2">
                  <Toggle label={s.name + " runs on its own"} isSelected={s.mode === "auto"} isDisabled={mode.pending || !canManageScope} onChange={auto => mode.save({ name: s.name, auto })} />
                  {manage ? <ScopeButton skill={{ name: s.name, kind: "private", fleet: s.shared_available, path: s.path }} manage={manage} /> : null}
                </div>} />
            )}</Group> : null}
            <OnlyHereList skills={data.only_here} manage={manage} local={local} reload={skills.reload} />
          </> : null}
          {tab === "removed" ? <RemovedList skills={data.removed} reload={skills.reload} /> : null}
        </>
      ) : null}

      {addScope && local ? <AddSkills canManageScope={addScope === "choose"} isOpen={!!data && (addScope !== "choose" || canManageScope)} onOpenChange={open => { if (!open) setAddScope(null); }} onAdded={skills.reload} /> : null}
      {scope && canManageScope ? <ScopeDialog skill={scope} close={() => setScope(null)} reload={skills.reload} /> : null}
      </>}
    </>
  );
}

function ScopeButton({ skill, manage }: { skill: SkillScope; manage: (s: SkillScope) => void }) {
  return <Button size="sm" variant="secondary" aria-label={"Sharing for " + skill.name} onPress={() => manage(skill)}>Sharing</Button>;
}

function ScopeDialog({ skill, close, reload }: { skill: SkillScope; close: () => void; reload: () => Promise<void> }) {
  const [action, setAction] = useState<SkillSharingAction | "adopt" | null>(null);
  const { pending, run } = useAction();
  const actions: (SkillSharingAction | "adopt")[] = skill.kind === "shared" ? ["private"] : skill.kind === "unmanaged"
    ? ["private", "adopt"] : skill.fleet ? ["publish", "resume"] : ["publish"];
  const apply = async () => {
    if (!action) return;
    const mutate = action === "adopt" ? () => api.adoptSkills([skill.name]) : () => api.setSkillSharing(skill.name, action);
    if (await run(action, mutate, reload)) close();
  };
  return <Modal.Backdrop isOpen onOpenChange={value => { if (!value && !pending) close(); }} variant="blur">
    <Modal.Container size="sm"><Modal.Dialog>
      <Modal.Header><Modal.Heading className="break-words">{skill.name}</Modal.Heading></Modal.Header>
      <Modal.Body className="min-w-0 space-y-3">
        {action ? <p>{scopeDetails[action]}</p> : <>
          <p>{skill.kind === "shared" ? "This machine follows the shared skill." : skill.kind === "private" ? "Edits stay on this machine." : "Installed only on this machine."}</p>
          {skill.path ? <code className="block text-xs [overflow-wrap:anywhere]">{skill.path}</code> : null}
          <div className="flex flex-wrap gap-2">{actions.map(a => <Button key={a} size="sm" variant="secondary" onPress={() => setAction(a)}>{scopeLabels[a]}</Button>)}</div>
        </>}
      </Modal.Body>
      <Modal.Footer>
        <Button variant="tertiary" isDisabled={!!pending} onPress={close}>Cancel</Button>
        {action ? <Button isPending={!!pending} onPress={() => void apply()}>{scopeLabels[action]}</Button> : null}
      </Modal.Footer>
    </Modal.Dialog></Modal.Container>
  </Modal.Backdrop>;
}

function Budget({ data }: { data: Skills }) {
  const auto = data.shared.filter((s) => s.mode === "auto").length;
  const pct = Math.min(100, (data.token_estimate / TOKEN_BUDGET) * 100);
  const color = pct > 90 ? "bg-danger" : pct > 60 ? "bg-warning" : "bg-success";
  return (
    <Group
      footer={`${auto} skills run on their own, so their descriptions cost tokens in every session. Switch one off to run it only when you call it (/name, $name).`}
    >
      <div className="px-4 py-3.5">
        <div className="mb-2 flex items-baseline justify-between text-[14px]">
          <span>Cost per session</span>
          <span className="text-muted">≈{data.token_estimate.toLocaleString()} tokens</span>
        </div>
        <div
          className="h-1.5 overflow-hidden rounded-full bg-surface-secondary"
          role="meter"
          aria-label="Token cost per session"
          aria-valuenow={data.token_estimate}
          aria-valuemax={TOKEN_BUDGET}
        >
          <div className={`h-full rounded-full ${color} transition-[width]`} style={{ width: `${pct}%` }} />
        </div>
      </div>
    </Group>
  );
}

function SharedList({ skills, library, reload, savingMode, setMode, manage }: {
  skills: SharedSkill[]; library: string; reload: () => Promise<void>;
  manage?: (skill: SkillScope) => void; savingMode: boolean; setMode: (name: string, auto: boolean) => void;
}) {
  const { pending, run } = useAction();
  const [editing, setEditing] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const toggle = (name: string, on: boolean) => setSelected((s) => (on ? [...s, name] : s.filter((n) => n !== name)));
  const done = () => {
    setEditing(false);
    setSelected([]);
  };

  return (
    <Group
      title={
        <span className="flex items-center justify-between">
          <span>In Your Library</span>
          {skills.length ? (
            <button type="button" className="text-[13px] font-normal text-accent" onClick={() => (editing ? done() : setEditing(true))}>
              {editing ? "Done" : "Edit"}
            </button>
          ) : null}
        </span>
      }
      footer={
        editing ? (
          <span className="flex items-center justify-between gap-3">
            <span>{selected.length} selected</span>
            <Button
              size="sm"
              variant="danger-soft"
              isDisabled={!selected.length}
              isPending={pending === "remove"}
              onPress={() =>
                run("remove", () => api.removeSkills(selected), async () => {
                  done();
                  await reload();
                })
              }
            >
              Remove from Every Machine
            </Button>
          </span>
        ) : (
          <>
            Shared from <span className="font-mono">{library}</span>.
          </>
        )
      }
    >
      {skills.length ? (
        skills.map((s) => (
          <Row
            key={s.name}
            icon={
              editing ? (
                <Checkbox variant="secondary" aria-label={`Select ${s.name}`} isSelected={selected.includes(s.name)} onChange={(on) => toggle(s.name, on)}>
                  <Checkbox.Content>
                    <Checkbox.Control className="rounded-full">
                      <Checkbox.Indicator />
                    </Checkbox.Control>
                  </Checkbox.Content>
                </Checkbox>
              ) : (
                <BrandTile brand={s.source === "yours" ? undefined : "github"} icon={s.source === "yours" ? HardDrive : FolderGit2} color={s.source === "yours" ? "indigo" : "purple"} />
              )
            }
            title={
              <span className="flex items-center gap-2">
                {s.name}
                {!s.installed ? <span className="text-xs text-warning">Not installed here</span> : null}
              </span>
            }
            subtitle={`${s.source === "yours" ? "Your own" : s.source} · ${s.agents.length ? s.agents.map(capitalize).join(" & ") : "no agent yet"}`}
            accessory={
              editing ? undefined : (
                <div className="flex flex-wrap items-center justify-end gap-2"><Toggle
                  label={`${s.name} runs on its own`}
                  isSelected={s.mode === "auto"}
                  isDisabled={savingMode || !!pending}
                  onChange={auto => setMode(s.name, auto)}
                />
                  {manage ? <ScopeButton skill={{ name: s.name, kind: "shared" }} manage={manage} /> : null}
                </div>
              )
            }
          />
        ))
      ) : (
        <Row icon={<IconTile icon={Sparkles} color="gray" />} title={<span className="text-muted">No skills yet. Add some from GitHub.</span>} />
      )}
    </Group>
  );
}

function OnlyHereList({ skills, reload, manage, local }: { skills: LocalSkill[]; reload: () => Promise<void>; manage?: (s: SkillScope) => void; local: boolean }) {
  const { pending, run } = useAction();
  return (
    <Group title="Other installed skills">
      {skills.length ? (
        skills.map((s) => (
          <Row
            key={s.name}
            icon={<IconTile icon={HardDrive} color="orange" />}
            title={s.name}
            subtitle={s.agent_own ? `${s.agents.map(capitalize).join(", ")}’s own${s.path ? ` · ${s.path}` : ""}` : "Installed here"}
            accessory={
              manage ? <ScopeButton skill={{ name: s.name, kind: "unmanaged", path: s.path }} manage={manage} />
                : local ? <Button size="sm" variant="secondary" isPending={pending === s.name} onPress={() => run(s.name, () => api.adoptSkills([s.name]), reload)}>Keep Everywhere</Button> : undefined
            }
          />
        ))
      ) : (
        <Row title={<span className="text-muted">No other installed skills.</span>} />
      )}
    </Group>
  );
}

function RemovedList({ skills, reload }: { skills: RemovedSkill[]; reload: () => Promise<void> }) {
  const { pending, run } = useAction();
  return (
    <Group title="Recently Removed" footer="Nothing is ever deleted. Restoring brings a skill back on every machine.">
      {skills.length ? (
        skills.map((s) => (
          <Row
            key={s.name}
            icon={<IconTile icon={ArchiveRestore} color="gray" />}
            title={s.was}
            subtitle={`Removed ${relative(s.removed)}`}
            accessory={
              <Button size="sm" variant="secondary" isPending={pending === s.name} onPress={() => run(s.name, () => api.restoreSkill(s.name), reload)}>
                Restore
              </Button>
            }
          />
        ))
      ) : (
        <Row title={<span className="text-muted">Nothing removed.</span>} />
      )}
    </Group>
  );
}

function AddSkills({ isOpen, onOpenChange, onAdded, canManageScope }: { canManageScope: boolean; isOpen: boolean; onOpenChange: (o: boolean) => void; onAdded: () => Promise<void> }) {
  const [privateCopy, setPrivateCopy] = useState(true);
  const [repo, setRepo] = useState("");
  const [names, setNames] = useState("");
  const [auto, setAuto] = useState(true);
  const { pending, run } = useAction();
  const list = names.split(/[\s,]+/).filter(Boolean);
  const valid = /^[\w.-]+\/[\w.-]+$/.test(repo.trim()) && list.length > 0;

  const submit = async () => {
    if (await run("add", () => api.addSkills(repo.trim(), list, !auto, canManageScope && privateCopy), onAdded)) {
      setRepo("");
      setNames("");
      onOpenChange(false);
    }
  };

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange} variant="blur">
      <Modal.Container size="sm">
        <Modal.Dialog>
          <Modal.Header>
            <Modal.Heading>Add Skills from GitHub</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4">
            <TextField value={repo} onChange={setRepo} isRequired>
              <Label>Repository</Label>
              <Input placeholder="anthropics/skills" variant="secondary" />
            </TextField>
            <TextField value={names} onChange={setNames} isRequired>
              <Label>Skills</Label>
              <Input placeholder="frontend-design, pdf" variant="secondary" />
              <Description>Folder names in that repository</Description>
            </TextField>
            {canManageScope ? <Segmented label="Install skills to" value={privateCopy ? "local" : "shared"}
              onChange={value => setPrivateCopy(value === "local")}
              options={[{ id: "local", label: "This machine" }, { id: "shared", label: "Shared library" }]} />
              : <p className="text-sm text-muted">These skills will be added to the shared library.</p>}
            <Switch isSelected={auto} onChange={setAuto} size="sm">
              <Switch.Content>
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
                <span className="text-sm">Run on their own</span>
              </Switch.Content>
            </Switch>
          </Modal.Body>
          <Modal.Footer>
            <Button slot="close" variant="secondary">
              Cancel
            </Button>
            <Button isDisabled={!valid} isPending={pending === "add"} onPress={submit}>
              <>Add</>
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
