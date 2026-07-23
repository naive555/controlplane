"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { FullPageSkeleton } from "@/components/full-page-skeleton";
import { useSession } from "@/lib/auth/use-session";

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  const { status } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (status === "authed") {
      router.replace("/organizations");
    }
  }, [status, router]);

  if (status === "loading") return <FullPageSkeleton />;
  if (status === "authed") return null; // redirect in flight

  return (
    <div className="flex flex-1 items-center justify-center bg-muted/30 p-6">
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}
