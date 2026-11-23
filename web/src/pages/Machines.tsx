import { Alert, Button, Dropdown, Label, ListBox, Select } from "@heroui/react";
import { ArrowRightLeft, Crown, Ellipsis, KeyRound } from "lucide-react";
import { useOverview } from "../App";
import { useConfirm } from "../components/Confirm";
import { CopyButton, Dot, Group, GroupSkeleton, IconTile, LargeTitle, Mono, Row, holdName, osName, osTile } from "../components/ui";
import { api } from "../lib/api";
import { relative } from "../lib/format";
import { useAction, useQuery } from "../lib/hooks";
import type { Machine } from "../lib/types";

export function MachinesPage() {
  const machines = useQuery(api.machines, 5000);
  const overview = useOverview();
  const { run } = useAction();
  const { ask, dialog } = useConfirm();

  const canManage = !!overview.data?.can_manage;
  const all = machines.data?.machines ?? [];
  const holders = all.filter((m) => m.hold === "hub" || m.hold === "standby");
  const main = all.find((m) => m.hold === "hub");
  const refresh = () => Promise.all([machines.reload(), overview.reload()]);

  const makeMain = (m: Machine) =>
    ask({
      title: `Make “${m.name}” the main machine?`,
      status: "accent",
      confirm: "Make Main",
      body: (
        <>
          {m.name} will hold your accounts whenever it’s on.
          {main && main.name !== m.name ? ` ${main.name} becomes a standby and takes over only while ${m.name} is off.` : ""} The accounts
          move between requests, so nothing running is cut off and nobody is logged out.
        </>
      ),
      run: () => run("main", () => api.makeMain(m.name), refresh),
    });

  const moveHere = (m: Machine) =>
    ask({
      title: `Move the accounts to “${m.name}” now?`,
      status: "warning",
      confirm: "Move Accounts",
      body: (
        <>
          Running requests finish first, up to two minutes, then {m.name} takes over.
          {m.hold === "standby" && main?.online
            ? ` Because ${main.name} is still the main machine, the accounts return to it once ${m.name} is idle. To keep them on ${m.name}, make it the main machine.`
            : ""}
        </>
      ),
      run: () => run("handoff", () => api.handoff(m.name), refresh),
    });

  return (
    <>
      <LargeTitle title="Machines" subtitle="Every device on your Tailscale network." />

      {machines.data?.problem ? (
        <Alert status="danger" className="mb-6">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>Tailscale didn’t answer</Alert.Title>
            <Alert.Description>{machines.data.problem}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}

      <Group
        footer={
          !canManage ? (
            <>
              Only the main machine and standbys can move the accounts. Open spillup on one of them, or run{" "}
              <Mono copy>spinup setup &lt;name&gt; --standby</Mono> here.
            </>
          ) : holders.length < 2 ? (
            <>
              Add a standby to choose between machines: run <Mono copy>spinup setup &lt;name&gt; --standby</Mono> on another one.
            </>
          ) : (
            "The main machine holds your Claude and Codex accounts whenever it’s on. Standbys take over while it’s off, then hand them back."
          )
        }
      >
        <Row
          icon={<IconTile icon={Crown} color="yellow" />}
          title="Main machine"
          subtitle="Usually the one that’s on the most"
          accessory={
            <Select
              aria-label="Main machine"
              className="w-48"
              variant="secondary"
              value={main?.name ?? null}
              onChange={(key) => {
                const m = holders.find((h) => h.name === key);
                if (m && m.hold !== "hub") makeMain(m);
              }}
              isDisabled={!canManage || holders.length < 2}
              placeholder="Choose…"
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {holders.map((m) => (
                    <ListBox.Item key={m.name} id={m.name} textValue={m.name}>
                      {m.name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          }
        />
      </Group>

      {machines.loading && !machines.data ? (
        <GroupSkeleton rows={4} />
      ) : (
        <Group title="Devices" footer={`${all.filter((m) => m.online).length} of ${all.length} online.`}>
          {all.map((m) => (
            <DeviceRow key={m.name} m={m} canManage={canManage} onMakeMain={() => makeMain(m)} onMoveHere={() => moveHere(m)} />
          ))}
        </Group>
      )}
      {dialog}
    </>
  );
}

function DeviceRow({ m, canManage, onMakeMain, onMoveHere }: { m: Machine; canManage: boolean; onMakeMain: () => void; onMoveHere: () => void }) {
  const t = osTile(m.os);
  const canHold = m.hold === "hub" || m.hold === "standby";
  const actions = [
    canHold && m.hold !== "hub" ? { id: "main", label: "Make Main Machine", icon: Crown } : null,
    canHold && !m.leading && m.online ? { id: "move", label: "Move Accounts Here Now", icon: ArrowRightLeft } : null,
  ].filter((a): a is NonNullable<typeof a> => !!a);

  const status = m.expired ? "Tailscale login expired" : m.online ? "Online" : `Offline · seen ${relative(m.last_seen)}`;

  return (
    <Row
      icon={<IconTile icon={t.icon} color={m.online ? t.color : "gray"} />}
      title={
        <span className="flex items-center gap-2">
          <span className="font-medium">{m.name}</span>
          {m.is_self ? <span className="text-xs text-muted">This machine</span> : null}
        </span>
      }
      subtitle={
        <span className="flex items-center gap-1.5">
          <Dot tone={m.expired ? "danger" : m.online ? "success" : "muted"} className="size-1.5" />
          {status} · {osName(m.os)} · <span className="tabular-nums">{m.ip ?? "—"}</span>
        </span>
      }
      value={
        <span className="flex items-center gap-1.5">
          {m.leading ? (
            <span className="flex items-center gap-1 text-success">
              <KeyRound className="size-3.5" /> Accounts
            </span>
          ) : null}
          {m.hold ? <span className="ml-2">{holdName[m.hold]}</span> : null}
        </span>
      }
      accessory={
        <span className="flex items-center">
          {m.ip ? <CopyButton value={m.ip} label={`Copy ${m.name}'s address`} /> : null}
          {canManage && actions.length ? (
            <Dropdown>
              <Button isIconOnly size="sm" variant="ghost" aria-label={`Actions for ${m.name}`} className="size-7 min-w-7">
                <Ellipsis className="size-4 text-muted" />
              </Button>
              <Dropdown.Popover placement="bottom end">
                <Dropdown.Menu onAction={(key) => (key === "main" ? onMakeMain() : onMoveHere())}>
                  {actions.map((a) => (
                    <Dropdown.Item key={a.id} id={a.id} textValue={a.label}>
                      <a.icon className="size-4" />
                      <Label>{a.label}</Label>
                    </Dropdown.Item>
                  ))}
                </Dropdown.Menu>
              </Dropdown.Popover>
            </Dropdown>
          ) : null}
        </span>
      }
    />
  );
}
