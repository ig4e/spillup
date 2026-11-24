import { Alert, Dropdown, Label, ListBox, Select } from "@heroui/react";
import { Button } from "../components/Button";
import { Activity, AppWindow, ArrowRightLeft, Crown, Ellipsis, KeyRound } from "lucide-react";
import { useOverview } from "../App";
import { useConfirm } from "../components/Confirm";
import { CopyButton, Dot, Group, GroupSkeleton, IconTile, LargeTitle, Row, holdName, osName } from "../components/ui";
import { OsTile } from "../components/brand";
import { Resources } from "../components/Resources";
import { Scan } from "../components/Scan";
import { api, currentRemote, setRemote } from "../lib/api";
import { relative } from "../lib/format";
import { navigate, useAction, useQuery } from "../lib/hooks";
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
          !canManage
            ? "Only the main machine and standbys can choose the main machine. Open spillup on one of them to change it."
            : holders.length < 2
              ? "To choose between machines, set up another one as a standby: install spillup there and it joins by itself."
              : "Every machine works with your accounts at the same time. The main machine holds the logins and serves everyone; a Worker + Standby takes over while it’s off, then hands them back."
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

      {machines.data && !currentRemote() ? <Resources machines={all} /> : null}
      {machines.data && !currentRemote() ? (
        <Group>
          <Row icon={<IconTile icon={Activity} color="teal" />} title="Activity" subtitle="Every machine's CPU, memory, disk, network and programs, live" to="/activity" />
        </Group>
      ) : null}

      {machines.loading && !machines.data ? (
        <GroupSkeleton rows={4} />
      ) : (
        <Group title="Devices" footer={`${all.filter((m) => m.online).length} of ${all.length} online.`}>
          {all.map((m) => (
            <DeviceRow key={m.name} m={m} canManage={canManage} onMakeMain={() => makeMain(m)} onMoveHere={() => moveHere(m)} />
          ))}
        </Group>
      )}
      <Scan />
      {dialog}
    </>
  );
}

function DeviceRow({ m, canManage, onMakeMain, onMoveHere }: { m: Machine; canManage: boolean; onMakeMain: () => void; onMoveHere: () => void }) {
  const canHold = m.hold === "hub" || m.hold === "standby";
  const actions = [
    canHold && m.hold !== "hub" ? { id: "main", label: "Make Main Machine", icon: Crown } : null,
    canHold && !m.leading && m.online ? { id: "move", label: "Move Accounts Here Now", icon: ArrowRightLeft } : null,
    !m.is_self && m.online && !currentRemote() && m.hold ? { id: "open", label: "Open Dashboard", icon: AppWindow } : null,
  ].filter((a): a is NonNullable<typeof a> => !!a);

  const status = m.expired ? "Tailscale login expired" : m.online ? "Online" : `Offline · seen ${relative(m.last_seen)}`;

  return (
    <Row
      icon={<OsTile os={m.os} dim={!m.online} />}
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
                <Dropdown.Menu
                  onAction={(key) => {
                    if (key === "main") onMakeMain();
                    else if (key === "move") onMoveHere();
                    else {
                      setRemote(m.name);
                      navigate("/");
                    }
                  }}
                >
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
