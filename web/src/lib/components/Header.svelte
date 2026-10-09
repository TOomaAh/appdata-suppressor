<script lang="ts">
  import ContainerIcon from "@lucide/svelte/icons/container";
  import LogOutIcon from "@lucide/svelte/icons/log-out";
  import MoonIcon from "@lucide/svelte/icons/moon";
  import RefreshIcon from "@lucide/svelte/icons/refresh-cw";
  import SunIcon from "@lucide/svelte/icons/sun";
  import TrashIcon from "@lucide/svelte/icons/trash-2";
  import { toggleMode } from "mode-watcher";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Progress } from "$lib/components/ui/progress";
  import { app } from "$lib/state.svelte";

  const trashCount = $derived(app.trash?.items.length ?? 0);
  const { running, done, total } = $derived(app.scan);

  function rescan() {
    app.view = "folders";
    void app.startScan();
  }
</script>

<header class="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
  <div class="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3">
    <div class="bg-primary text-primary-foreground flex size-8 shrink-0 items-center justify-center rounded-lg">
      <ContainerIcon class="size-4" />
    </div>
    <div class="min-w-0">
      <h1 class="text-sm leading-tight font-semibold">Appdata Cleaner</h1>
      {#if app.scan.report}
        <p class="text-muted-foreground truncate font-mono text-xs">{app.scan.report.root}</p>
      {/if}
    </div>

    <div class="ml-auto flex items-center gap-1.5">
      <Button
        variant={app.view === "trash" ? "secondary" : "ghost"}
        size="sm"
        onclick={() => (app.view = app.view === "trash" ? "folders" : "trash")}
      >
        <TrashIcon />
        <span class="hidden sm:inline">Trash</span>
        {#if trashCount}<Badge variant="secondary" class="px-1.5">{trashCount}</Badge>{/if}
      </Button>
      <Button size="sm" onclick={rescan} disabled={running}>
        <RefreshIcon class={running ? "animate-spin" : ""} />
        {#if running}
          <span class="tabular-nums">{total ? `${done}/${total}` : "Scanning…"}</span>
        {:else}
          Scan
        {/if}
      </Button>
      <Button variant="ghost" size="icon-sm" onclick={toggleMode} aria-label="Toggle theme">
        <SunIcon class="scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
        <MoonIcon class="absolute scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
      </Button>
      {#if app.authEnabled}
        <Button variant="ghost" size="icon-sm" onclick={() => app.logout()} aria-label="Log out" title="Log out">
          <LogOutIcon />
        </Button>
      {/if}
    </div>
  </div>
  {#if running}
    <Progress value={total ? (100 * done) / total : 0} class="h-0.5 rounded-none" />
  {/if}
</header>
