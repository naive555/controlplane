"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ApiError } from "@/lib/api/client";
import { assignSubscription, getSubscription, listPlans } from "@/lib/api/endpoints";
import { useActiveOrgId } from "@/lib/org/active-org";

export default function SubscriptionPage() {
  const activeOrgId = useActiveOrgId();
  const queryClient = useQueryClient();
  const [selectedPlanId, setSelectedPlanId] = useState("");

  const { data: subscription, isLoading } = useQuery({
    queryKey: ["subscription", activeOrgId],
    queryFn: getSubscription,
    enabled: activeOrgId !== null,
  });

  // Plans are global, not org-scoped — no activeOrgId in the query key.
  const { data: plans } = useQuery({ queryKey: ["plans"], queryFn: listPlans });

  // The contract has no admin/permission check on assign — any org member
  // can change the plan. The UI doesn't add a client-side gate either, for
  // parity with the source app's behavior (see lib/api/endpoints.ts).
  const assignMutation = useMutation({
    mutationFn: (planId: string) => assignSubscription(planId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["subscription", activeOrgId] });
      toast.success("Plan assigned.");
      setSelectedPlanId("");
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "Failed to assign plan.");
    },
  });

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-lg font-semibold">Subscription</h1>

      <Card className="max-w-md">
        <CardHeader>
          <CardTitle>{subscription?.plan.name ?? "No plan assigned"}</CardTitle>
          <CardDescription>
            {isLoading
              ? "Loading…"
              : subscription
                ? "Current plan and limits"
                : "This organization has no subscription yet."}
          </CardDescription>
        </CardHeader>
        {subscription && (
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {Object.entries(subscription.plan.limits ?? {}).map(([key, value]) => (
                <Badge key={key} variant="secondary">
                  {key}: {value === -1 ? "unlimited" : String(value)}
                </Badge>
              ))}
            </div>
          </CardContent>
        )}
        <CardFooter className="flex items-center gap-2">
          <Select value={selectedPlanId} onValueChange={(value) => setSelectedPlanId(value ?? "")}>
            <SelectTrigger className="flex-1">
              <SelectValue placeholder="Select a plan">
                {(value: string) => plans?.find((p) => p.id === value)?.name ?? "Select a plan"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {plans?.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            disabled={!selectedPlanId || assignMutation.isPending}
            onClick={() => assignMutation.mutate(selectedPlanId)}
          >
            {assignMutation.isPending ? "Assigning…" : "Assign"}
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}
