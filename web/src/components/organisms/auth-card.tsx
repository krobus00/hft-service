"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Loader2, LogIn, UserPlus } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { AuthFormField } from "@/components/molecules/auth-form-field";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ApiError, getAuthConfig, login, setup } from "@/lib/auth-api";

type AuthCardProps = {
  mode: "login" | "setup";
};

type FormState = {
  email: string;
  name: string;
  password: string;
};

export function AuthCard({ mode }: AuthCardProps) {
  const router = useRouter();
  const [form, setForm] = useState<FormState>({
    email: "",
    name: "",
    password: "",
  });
  const [error, setError] = useState("");
  const [showSetupLink, setShowSetupLink] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    let isMounted = true;
    async function loadAuthConfig() {
      if (mode !== "login") {
        return;
      }
      try {
        const config = await getAuthConfig();
        if (isMounted) {
          setShowSetupLink(config.show_setup_link);
        }
      } catch {
        if (isMounted) {
          setShowSetupLink(true);
        }
      }
    }
    void loadAuthConfig();
    return () => {
      isMounted = false;
    };
  }, [mode]);

  const copy = useMemo(
    () =>
      mode === "setup"
        ? {
            title: "Create admin",
            description: "Create the first Krobot administrator account.",
            action: "Create admin",
            switchText: "Already have an account?",
            switchHref: "/login",
            switchLabel: "Sign in",
          }
        : {
            title: "Sign in",
            description: "Use your Krobot dashboard credentials.",
            action: "Sign in",
            switchText: "First time running Krobot?",
            switchHref: "/setup",
            switchLabel: "Create admin",
          },
    [mode],
  );

  function updateField(field: keyof FormState, value: string) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      if (mode === "setup") {
        await setup(form);
      } else {
        await login({ email: form.email, password: form.password });
      }
      router.replace("/");
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message);
      } else {
        setError("Unable to reach Krobot API.");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card className="w-full max-w-md">
      <CardHeader>
        <CardTitle>{copy.title}</CardTitle>
        <CardDescription>{copy.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="grid gap-4" onSubmit={onSubmit}>
          {mode === "setup" ? (
            <AuthFormField
              id="name"
              label="Name"
              value={form.name}
              autoComplete="name"
              onChange={(value) => updateField("name", value)}
            />
          ) : null}
          <AuthFormField
            id="email"
            label="Email"
            type="email"
            value={form.email}
            autoComplete="email"
            onChange={(value) => updateField("email", value)}
          />
          <AuthFormField
            id="password"
            label="Password"
            type="password"
            value={form.password}
            autoComplete={mode === "setup" ? "new-password" : "current-password"}
            onChange={(value) => updateField("password", value)}
          />
          {error ? (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          ) : null}
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : mode === "setup" ? (
              <UserPlus className="h-4 w-4" />
            ) : (
              <LogIn className="h-4 w-4" />
            )}
            {copy.action}
          </Button>
          {mode === "setup" || showSetupLink ? (
            <p className="text-center text-sm text-muted-foreground">
              {copy.switchText}{" "}
              <Link className="font-medium text-primary hover:underline" href={copy.switchHref}>
                {copy.switchLabel}
              </Link>
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  );
}
