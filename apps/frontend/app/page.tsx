export default function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center bg-zinc-50 font-sans dark:bg-black">
      <main className="flex flex-col items-center gap-3 text-center">
        <h1 className="text-3xl font-semibold tracking-tight text-black dark:text-zinc-50">
          controlplane
        </h1>
        <p className="max-w-md text-zinc-600 dark:text-zinc-400">
          Dashboard shell. Real pages (auth, organizations, RBAC, audit logs, subscription) are
          built in a later phase — see <code>docs/04-migration-plan.md</code>.
        </p>
      </main>
    </div>
  );
}
