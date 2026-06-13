import { AuthPageTemplate } from "@/components/templates/auth-page-template";
import { AuthCard } from "@/components/organisms/auth-card";

export default function LoginPage() {
  return (
    <AuthPageTemplate>
      <AuthCard mode="login" />
    </AuthPageTemplate>
  );
}
