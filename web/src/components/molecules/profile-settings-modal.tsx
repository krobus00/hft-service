"use client";

import { Eye, EyeOff, Save } from "lucide-react";
import { FormEvent, useState } from "react";

import { ModalShell } from "@/components/molecules/modal-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { updateMyProfile } from "@/lib/api-client";
import type { AuthUser } from "@/types/auth";

type ProfileSettingsModalProps = {
  user: AuthUser;
  onSaved: (user: AuthUser) => void;
  onClose: () => void;
};

export function ProfileSettingsModal({ user, onSaved, onClose }: ProfileSettingsModalProps) {
  const [name, setName] = useState(user.name);
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }
    setError("");
    setIsSubmitting(true);
    try {
      const nextUser = await updateMyProfile({ name, password: password.trim() || undefined });
      onSaved(nextUser);
      onClose();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to update profile.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <ModalShell title="Profile settings" onClose={onClose}>
      <form className="grid gap-4" onSubmit={handleSubmit}>
        <div className="grid gap-2">
          <Label htmlFor="profile-name">Name</Label>
          <Input
            id="profile-name"
            value={name}
            disabled={isSubmitting}
            onChange={(event) => setName(event.target.value)}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="profile-password">New password</Label>
          <div className="relative">
            <Input
              id="profile-password"
              type={showPassword ? "text" : "password"}
              value={password}
              disabled={isSubmitting}
              className="pr-10"
              onChange={(event) => setPassword(event.target.value)}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-0 top-0 h-10 w-10"
              disabled={isSubmitting}
              onClick={() => setShowPassword((current) => !current)}
            >
              {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </Button>
          </div>
        </div>
        {error ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" disabled={isSubmitting} onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            <Save className="h-4 w-4" />
            Save
          </Button>
        </div>
      </form>
    </ModalShell>
  );
}
