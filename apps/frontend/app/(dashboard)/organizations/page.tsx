"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { PlusIcon } from "lucide-react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ApiError } from "@/lib/api/client";
import { createOrganization, listOrganizations } from "@/lib/api/endpoints";
import { useActiveOrgId, useSelectOrg } from "@/lib/org/active-org";

const createOrgSchema = z.object({
  name: z.string().min(1, "Name is required"),
  slug: z
    .string()
    .min(2, "Slug must be at least 2 characters")
    .regex(/^[a-z0-9-]+$/, "Lowercase letters, numbers, and hyphens only"),
});

type CreateOrgValues = z.infer<typeof createOrgSchema>;

export default function OrganizationsPage() {
  const [open, setOpen] = useState(false);
  const activeOrgId = useActiveOrgId();
  const selectOrg = useSelectOrg();

  const { data: memberships, isLoading } = useQuery({
    queryKey: ["organizations"],
    queryFn: listOrganizations,
  });

  const {
    control,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateOrgValues>({
    resolver: zodResolver(createOrgSchema),
    defaultValues: { name: "", slug: "" },
  });

  const createMutation = useMutation({
    mutationFn: createOrganization,
    onSuccess: (org) => {
      // Invalidates every org-scoped query, including the org list itself.
      selectOrg(org.id);
      toast.success(`Organization "${org.name}" created.`);
      reset();
      setOpen(false);
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "Failed to create organization.");
    },
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Organizations</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button size="sm" />}>
            <PlusIcon className="size-4" /> Create organization
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create organization</DialogTitle>
              <DialogDescription>You&apos;ll be the owner of the new organization.</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSubmit((values) => createMutation.mutate(values))} noValidate>
              <FieldGroup>
                <Field data-invalid={!!errors.name}>
                  <FieldLabel htmlFor="org-name">Name</FieldLabel>
                  <Controller
                    control={control}
                    name="name"
                    render={({ field }) => <Input id="org-name" placeholder="Acme Inc." {...field} />}
                  />
                  <FieldError errors={[errors.name]} />
                </Field>
                <Field data-invalid={!!errors.slug}>
                  <FieldLabel htmlFor="org-slug">Slug</FieldLabel>
                  <Controller
                    control={control}
                    name="slug"
                    render={({ field }) => <Input id="org-slug" placeholder="acme-inc" {...field} />}
                  />
                  <FieldError errors={[errors.slug]} />
                </Field>
              </FieldGroup>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating…" : "Create"}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Slug</TableHead>
            <TableHead>Role</TableHead>
            <TableHead className="text-right">Active</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                Loading…
              </TableCell>
            </TableRow>
          ) : memberships?.length ? (
            memberships.map((m) => (
              <TableRow key={m.organizationId}>
                <TableCell className="font-medium">{m.organization.name}</TableCell>
                <TableCell className="text-muted-foreground">{m.organization.slug}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{m.role}</Badge>
                </TableCell>
                <TableCell className="text-right">
                  {m.organizationId === activeOrgId ? (
                    <Badge>Active</Badge>
                  ) : (
                    <Button variant="outline" size="xs" onClick={() => selectOrg(m.organizationId)}>
                      Switch
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                No organizations yet — create one to get started.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
