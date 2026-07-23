"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { CheckIcon, PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { listOrganizations } from "@/lib/api/endpoints";
import { useActiveOrgId, useSelectOrg } from "@/lib/org/active-org";

export function OrgSwitcher() {
  const { data: memberships } = useQuery({ queryKey: ["organizations"], queryFn: listOrganizations });
  const activeOrgId = useActiveOrgId();
  const selectOrg = useSelectOrg();

  const activeName = memberships?.find((m) => m.organizationId === activeOrgId)?.organization.name;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="outline" size="sm" />}>
        {activeName ?? "Select organization"}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Organizations</DropdownMenuLabel>
          {memberships?.length ? (
            memberships.map((m) => (
              <DropdownMenuItem key={m.organizationId} onClick={() => selectOrg(m.organizationId)}>
                <span className="flex-1">{m.organization.name}</span>
                {m.organizationId === activeOrgId && <CheckIcon className="size-4" />}
              </DropdownMenuItem>
            ))
          ) : (
            <DropdownMenuItem disabled>No organizations yet</DropdownMenuItem>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem render={<Link href="/organizations" />}>
          <PlusIcon className="size-4" /> Create organization
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
