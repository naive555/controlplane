"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getAuditLogs, listMembers } from "@/lib/api/endpoints";
import { useActiveOrgId } from "@/lib/org/active-org";

// Recorded actions per docs/02-api-contract.md — role.created/role.assigned
// are defined in the contract but not currently written by the backend;
// kept in the filter list anyway since the contract documents them as valid
// `action` query values.
const KNOWN_ACTIONS = [
  "user.login",
  "user.register",
  "org.created",
  "org.member.invited",
  "org.member.removed",
  "role.created",
  "role.assigned",
];

const ALL_ACTIONS = "__all_actions__";
const ALL_USERS = "__all_users__";

export default function AuditPage() {
  const activeOrgId = useActiveOrgId();
  const [action, setAction] = useState(ALL_ACTIONS);
  const [userId, setUserId] = useState(ALL_USERS);
  const [limit, setLimit] = useState(50);

  const { data: members } = useQuery({
    queryKey: ["members", activeOrgId],
    queryFn: listMembers,
    enabled: activeOrgId !== null,
  });

  const filters = {
    action: action === ALL_ACTIONS ? undefined : action,
    userId: userId === ALL_USERS ? undefined : userId,
    limit,
  };

  const { data: logs, isLoading } = useQuery({
    queryKey: ["audit", activeOrgId, filters],
    queryFn: () => getAuditLogs(filters),
    enabled: activeOrgId !== null,
  });

  function memberLabel(id: string | null): string {
    if (!id) return "—";
    return members?.find((m) => m.userId === id)?.email ?? id;
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-lg font-semibold">Audit Logs</h1>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium">Action</label>
          <Select value={action} onValueChange={(value) => setAction(value ?? ALL_ACTIONS)}>
            <SelectTrigger className="w-56">
              <SelectValue>{(value: string) => (value === ALL_ACTIONS ? "All actions" : value)}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_ACTIONS}>All actions</SelectItem>
              {KNOWN_ACTIONS.map((a) => (
                <SelectItem key={a} value={a}>
                  {a}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium">User</label>
          <Select value={userId} onValueChange={(value) => setUserId(value ?? ALL_USERS)}>
            <SelectTrigger className="w-56">
              <SelectValue>
                {(value: string) => (value === ALL_USERS ? "All users" : memberLabel(value))}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_USERS}>All users</SelectItem>
              {members?.map((m) => (
                <SelectItem key={m.userId} value={m.userId}>
                  {m.email}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <label htmlFor="audit-limit" className="text-sm font-medium">
            Limit
          </label>
          <Input
            id="audit-limit"
            type="number"
            min={1}
            max={100}
            className="w-24"
            value={limit}
            onChange={(e) => {
              const next = Number(e.target.value);
              if (Number.isFinite(next)) {
                setLimit(Math.min(100, Math.max(1, next)));
              }
            }}
          />
        </div>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Action</TableHead>
            <TableHead>Actor</TableHead>
            <TableHead>Metadata</TableHead>
            <TableHead>When</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                Loading…
              </TableCell>
            </TableRow>
          ) : logs?.length ? (
            logs.map((log) => (
              <TableRow key={log.id}>
                <TableCell>
                  <Badge variant="secondary">{log.action}</Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">{memberLabel(log.userId)}</TableCell>
                <TableCell className="max-w-xs truncate font-mono text-xs text-muted-foreground">
                  {log.metadata && Object.keys(log.metadata).length ? JSON.stringify(log.metadata) : "—"}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(log.createdAt).toLocaleString("en-US")}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                No audit log entries yet.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
