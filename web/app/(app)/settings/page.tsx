import { Content, TopBar } from "@/components/shell/topbar";
import { SettingsScreen } from "./settings-screen";

export default function Settings() {
  return (
    <>
      <TopBar crumbs={[{ label: "Settings" }]} />
      <Content>
        <SettingsScreen />
      </Content>
    </>
  );
}
