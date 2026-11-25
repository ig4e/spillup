import assert from "node:assert/strict";
import test from "node:test";
import { agentHosts, hasAgentLink, mobilePeer } from "../src/lib/t3.ts";

test("phone clients and agent links have independent identities", () => {
  const phone = { name: "phone", os: "Android" };
  const hosts = agentHosts(JSON.stringify([
    { url: "http://phone.other.example:3333" },
    { urls: ["http://[fd7a:115c:a1e0::1]:3333", "http://DESK.example./"] },
    { status: "expired", url: "http://old-host:3333" },
  ]));
  assert.equal(mobilePeer(phone), true);
  assert.equal(hasAgentLink(phone, hosts), false);
  assert.equal(hasAgentLink({ name: "desk", dns_name: "desk.example." }, hosts), true);
  assert.equal(hasAgentLink({ name: "edge", ipv6: "fd7a:115c:a1e0::1" }, hosts), true);
  assert.equal(hasAgentLink({ name: "old-host" }, hosts), false);
});
