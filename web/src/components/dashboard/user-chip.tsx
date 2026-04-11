"use client";

import { useEffect, useState } from "react";

import { LogoutButton } from "@/components/auth/logout-button";
import type { ClientUser } from "@/lib/types/auth";

type MeResponse = {
  data?: ClientUser;
  error?: {
    message: string;
  };
};

export function UserChip() {
  const [user, setUser] = useState<ClientUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;

    const loadUser = async () => {
      try {
        const response = await fetch("/api/auth/me", {
          method: "GET",
          credentials: "include",
          cache: "no-store",
        });

        const payload = (await response.json()) as MeResponse;

        if (!mounted) {
          return;
        }

        if (!response.ok || !payload.data) {
          setUser(null);
          return;
        }

        setUser(payload.data);
      } catch {
        if (mounted) {
          setUser(null);
        }
      } finally {
        if (mounted) {
          setLoading(false);
        }
      }
    };

    void loadUser();

    return () => {
      mounted = false;
    };
  }, []);

  if (loading) {
    return <div className="user-chip muted">Loading user...</div>;
  }

  if (!user) {
    return (
      <div className="user-chip muted">
        Session unavailable
        <LogoutButton />
      </div>
    );
  }

  return (
    <div className="user-chip">
      <div className="user-identity">
        <span className="user-dot" aria-hidden="true" />
        <div>
        <p className="user-name">{user.username || user.userId}</p>
        <p className="user-role">{user.role}</p>
        </div>
      </div>
      <LogoutButton />
    </div>
  );
}
