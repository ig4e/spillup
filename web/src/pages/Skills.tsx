import { Alert, Checkbox, Description, Input, Label, Modal, Switch, Tabs, TextField } from "@heroui/react";
import { Button } from "../components/Button";
import { ArchiveRestore, FolderGit2, HardDrive, Plus, RefreshCw, Sparkles } from "lucide-react";
import { useState } from "react";
import { Group, GroupSkeleton, IconTile, LargeTitle, Row, Toggle } from "../components/ui";
import { api } from "../lib/api";
import { capitalize, relative } from "../lib/format";
import { useAction, useQuery } from "../lib/hooks";
import type { LocalSkill, RemovedSkill, SharedSkill, Skills } from "../lib/types";

const TOKEN_BUDGET = 8000;

export function SkillsPage() {
  const skills = useQuery(api.skills);
  const { pending, run } = useAction();
  const [addOpen, setAddOpen] = useState(false);
  const data = skills.data;

  return (
    <>
      <LargeTitle
        title="Skills"
        subtitle="One library, identical on every machine and linked into Claude Code and Codex."
        actions={
          <>
            <Button variant="secondary" size="sm" isPending={pending === "sync"} onPress={() => run("sync", api.syncSkills, skills.reload)}>
              <><RefreshCw className="size-3.5" />Sync</>
            </Button>
            <Button size="sm" onPress={() => setAddOpen(true)}>
              <Plus className="size-3.5" /> Add Skills
            </Button>
          </>
        }
      />

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

      {!data ? (
        <GroupSkeleton rows={5} />
      ) : (
        <>
          <Budget data={data} />
          <Tabs defaultSelectedKey="shared" variant="primary">
            <Tabs.ListContainer className="mx-auto mb-5 w-fit">
              <Tabs.List aria-label="Skill lists" className="w-auto">
                <Tabs.Tab id="shared" className="px-4 whitespace-nowrap">
                  Shared ({data.shared.length})
                  <Tabs.Indicator />
                </Tabs.Tab>
                <Tabs.Tab id="here" className="px-4 whitespace-nowrap">
                  Only Here ({data.only_here.length})
                  <Tabs.Indicator />
                </Tabs.Tab>
                <Tabs.Tab id="removed" className="px-4 whitespace-nowrap">
                  Removed ({data.removed.length})
                  <Tabs.Indicator />
                </Tabs.Tab>
              </Tabs.List>
            </Tabs.ListContainer>
            <Tabs.Panel id="shared" className="p-0">
              <SharedList skills={data.shared} library={data.library} reload={skills.reload} />
            </Tabs.Panel>
            <Tabs.Panel id="here" className="p-0">
              <OnlyHereList skills={data.only_here} reload={skills.reload} />
            </Tabs.Panel>
            <Tabs.Panel id="removed" className="p-0">
              <RemovedList skills={data.removed} reload={skills.reload} />
            </Tabs.Panel>
          </Tabs>
        </>
      )}

      <AddSkills isOpen={addOpen} onOpenChange={setAddOpen} onAdded={skills.reload} />
    </>
  );
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

function SharedList({ skills, library, reload }: { skills: SharedSkill[]; library: string; reload: () => Promise<void> }) {
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
            Stored in <span className="font-mono">{library}</span>. Removed skills are kept, never deleted.
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
                <IconTile icon={s.source === "yours" ? HardDrive : FolderGit2} color={s.source === "yours" ? "indigo" : "purple"} />
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
                <Toggle
                  label={`${s.name} runs on its own`}
                  isSelected={s.mode === "auto"}
                  isDisabled={pending === s.name}
                  onChange={(auto) => run(s.name, () => api.setMode([s.name], !auto), reload)}
                />
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

function OnlyHereList({ skills, reload }: { skills: LocalSkill[]; reload: () => Promise<void> }) {
  const { pending, run } = useAction();
  return (
    <Group title="Only on This Machine" footer="Your other machines never see these. Keep one to add it to your library, on every machine.">
      {skills.length ? (
        skills.map((s) => (
          <Row
            key={s.name}
            icon={<IconTile icon={HardDrive} color="orange" />}
            title={s.name}
            subtitle={s.agent_own ? `${s.agents.map(capitalize).join(", ")}’s own${s.path ? ` · ${s.path}` : ""}` : "Installed here"}
            accessory={
              <Button size="sm" variant="secondary" isPending={pending === s.name} onPress={() => run(s.name, () => api.adoptSkills([s.name]), reload)}>
                Keep Everywhere
              </Button>
            }
          />
        ))
      ) : (
        <Row title={<span className="text-muted">Everything here is shared.</span>} />
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

function AddSkills({ isOpen, onOpenChange, onAdded }: { isOpen: boolean; onOpenChange: (o: boolean) => void; onAdded: () => Promise<void> }) {
  const [repo, setRepo] = useState("");
  const [names, setNames] = useState("");
  const [auto, setAuto] = useState(true);
  const { pending, run } = useAction();
  const list = names.split(/[\s,]+/).filter(Boolean);
  const valid = /^[\w.-]+\/[\w.-]+$/.test(repo.trim()) && list.length > 0;

  const submit = async () => {
    if (await run("add", () => api.addSkills(repo.trim(), list, !auto), onAdded)) {
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
