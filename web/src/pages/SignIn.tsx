import { OwnLogins } from "../components/OwnLogins";
import { LargeTitle } from "../components/ui";

export function SignInPage() {
  return (
    <>
      <LargeTitle title="Sign-In" subtitle="Choose which providers this machine signs in to by itself instead of using the shared accounts." />
      <OwnLogins />
    </>
  );
}
