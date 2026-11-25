# Fleet operations research

Spillup's main jobs are onboarding a machine, keeping its skills and tools in step, moving accounts safely, and setting up T3 Code links. The v0.4 changes address three gaps across these jobs.

| Tool or pattern | What Spillup uses or could add |
| --- | --- |
| Ansible check mode previews playbook work before applying it. [Check mode](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_checkmode.html) | `spinup setup --plan` previews the role and ordered steps. A future drift preview could identify settings, files, and versions that would change. |
| Rundeck shows running steps, captures execution history, and supports time limits. [Jobs](https://docs.rundeck.com/docs/manual/jobs/index.html) [Job timeouts](https://docs.rundeck.com/docs/manual/jobs/creating-jobs.html#timeout) | Setup and system updates now report stages and recent output. Activity shows the last 100 dashboard actions since startup. A persistent history with retry from a safe checkpoint would help with interrupted rollouts. |
| Syncthing reports connection and disconnection as distinct events. [Connected](https://docs.syncthing.net/events/deviceconnected.html) [Disconnected](https://docs.syncthing.net/events/devicedisconnected.html) | Add a machine timeline showing when Tailscale, Spillup, account sync, and T3 linking each last worked. This would help distinguish an offline device from an unhealthy service. |
| Tailscale separates network reachability, application grants, and configuration audit events. [Grants](https://tailscale.com/docs/features/access-control/grants) [Audit logging](https://tailscale.com/docs/features/logging/audit-logging) | Dashboard administration and library sync now require device ownership or a management credential where applicable. Separate capabilities for fleet management, library sync, and inference would let teams delegate access more precisely. |
| CLIProxyAPI Management Center handles OAuth, provider configuration, and quota views. [Management Center releases](https://github.com/router-for-me/Cli-Proxy-API-Management-Center/releases) | Add read-only quota summaries and upstream health to Spillup for workers. Keep the existing management UI for account changes. |

The next features with the clearest use cases are the machine timeline for outage diagnosis, a drift preview before applying changes, and staged updates that preserve a healthy account holder. Recent Actions records route names, origin class, duration, and status. It has no request bodies and resets when Spillup restarts.
