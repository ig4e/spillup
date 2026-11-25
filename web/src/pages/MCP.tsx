import { MCPServers } from "../components/MCPServers";
import { LargeTitle } from "../components/ui";

export function MCPPage() {
  return (
    <>
      <LargeTitle title="MCP Servers" />
      <MCPServers />
    </>
  );
}
