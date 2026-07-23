"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { FullPageSkeleton } from "@/components/full-page-skeleton";
import { Button } from "@/components/ui/button";
import { getActiveOrgId } from "@/lib/auth/token-store";
import { useSession } from "@/lib/auth/use-session";
import { cn } from "@/lib/utils";

const NAV_ITEMS: { href: string; label: string; requiresOrg: boolean }[] = [
  { href: "/organizations", label: "Organizations", requiresOrg: false },
  { href: "/members", label: "Members", requiresOrg: true },
  { href: "/roles", label: "Roles", requiresOrg: true },
  { href: "/audit", label: "Audit Logs", requiresOrg: true },
  { href: "/subscription", label: "Subscription", requiresOrg: true },
];

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const { status, user, logoutSession } = useSession();
  const router = useRouter();
  // Non-reactive on purpose: Step 6 introduces the real org-switcher context
  // that other pages subscribe to. This layout only needs the value at
  // render time (it re-reads on every navigation) to gate nav items.
  const hasActiveOrg = Boolean(getActiveOrgId());

  useEffect(() => {
    if (status === "anon") {
      router.replace("/login");
    }
  }, [status, router]);

  if (status === "loading") return <FullPageSkeleton />;
  if (status === "anon") return null; // redirect in flight

  const handleLogout = async () => {
    try {
      await logoutSession();
    } catch {
      toast.error("Logout request failed, but you've been signed out locally.");
    }
  };

  return (
    <div className="flex min-h-screen flex-1">
      <aside className="w-56 shrink-0 border-r p-4">
        <div className="mb-6 px-3 font-semibold">controlplane</div>
        <nav className="flex flex-col gap-1">
          {NAV_ITEMS.map((item) => {
            const disabled = item.requiresOrg && !hasActiveOrg;
            return disabled ? (
              <span
                key={item.href}
                title="Select an organization first"
                className="cursor-not-allowed rounded-md px-3 py-2 text-sm text-muted-foreground/50"
              >
                {item.label}
              </span>
            ) : (
              <Link
                key={item.href}
                href={item.href}
                className={cn("rounded-md px-3 py-2 text-sm hover:bg-accent hover:text-accent-foreground")}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
      </aside>
      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b px-6 py-3">
          <div className="text-sm text-muted-foreground">{user?.email}</div>
          <Button variant="outline" size="sm" onClick={() => void handleLogout()}>
            Log out
          </Button>
        </header>
        <main className="flex-1 p-6">{children}</main>
      </div>
    </div>
  );
}
