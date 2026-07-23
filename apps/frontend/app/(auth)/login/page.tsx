"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/client";
import { login } from "@/lib/api/endpoints";
import { useSession } from "@/lib/auth/use-session";

const loginSchema = z.object({
  email: z.email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

type LoginFormValues = z.infer<typeof loginSchema>;

export default function LoginPage() {
  const router = useRouter();
  const { applyTokens } = useSession();
  const {
    control,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: "", password: "" },
  });

  const onSubmit = async (values: LoginFormValues) => {
    try {
      const tokens = await login(values);
      applyTokens(tokens);
      router.replace("/organizations");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Something went wrong. Try again.");
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Log in</CardTitle>
        <CardDescription>Welcome back to controlplane.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <FieldGroup>
            <Field data-invalid={!!errors.email}>
              <FieldLabel htmlFor="email">Email</FieldLabel>
              <Controller
                control={control}
                name="email"
                render={({ field }) => (
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    placeholder="you@example.com"
                    {...field}
                  />
                )}
              />
              <FieldError errors={[errors.email]} />
            </Field>

            <Field data-invalid={!!errors.password}>
              <FieldLabel htmlFor="password">Password</FieldLabel>
              <Controller
                control={control}
                name="password"
                render={({ field }) => (
                  <Input id="password" type="password" autoComplete="current-password" {...field} />
                )}
              />
              <FieldError errors={[errors.password]} />
            </Field>

            <Button type="submit" disabled={isSubmitting} className="w-full">
              {isSubmitting ? "Logging in…" : "Log in"}
            </Button>
          </FieldGroup>
        </form>
      </CardContent>
      <p className="px-6 pb-6 text-center text-sm text-muted-foreground">
        No account?{" "}
        <Link href="/register" className="text-primary underline-offset-4 hover:underline">
          Register
        </Link>
      </p>
    </Card>
  );
}
