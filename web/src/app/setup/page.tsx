import { AuthPageTemplate } from "@/components/templates/auth-page-template";
import { AuthCard } from "@/components/organisms/auth-card";

export default function SetupPage() {
  return (
    <AuthPageTemplate>
      <AuthCard mode="setup" />
    </AuthPageTemplate>
  );
}
