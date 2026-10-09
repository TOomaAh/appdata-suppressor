<script lang="ts">
  import ArrowDownIcon from "@lucide/svelte/icons/arrow-down";
  import ArrowUpIcon from "@lucide/svelte/icons/arrow-up";
  import CircleAlertIcon from "@lucide/svelte/icons/circle-alert";
  import SearchIcon from "@lucide/svelte/icons/search";
  import TrashIcon from "@lucide/svelte/icons/trash-2";
  import XIcon from "@lucide/svelte/icons/x";
  import * as Alert from "$lib/components/ui/alert";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Input } from "$lib/components/ui/input";
  import { Skeleton } from "$lib/components/ui/skeleton";
  import * as Table from "$lib/components/ui/table";
  import * as Tabs from "$lib/components/ui/tabs";
  import type { Folder } from "$lib/api";
  import { count, formatBytes, formatDate } from "$lib/format";
  import { app } from "$lib/state.svelte";
  import ConfirmDialog, { type ConfirmRequest } from "./ConfirmDialog.svelte";

  type Filter = "all" | "orphan" | "linked";
  type SortKey = "name" | "size" | "status";

  let filter = $state<Filter>("all");
  let search = $state("");
  let sort = $state<{ key: SortKey; dir: 1 | -1 }>({ key: "status", dir: 1 });
  let confirm = $state<ConfirmRequest | null>(null);

  const report = $derived(app.scan.report);
  const approx = $derived(app.scan.running ? "≈ " : "");

  const visible = $derived.by(() => {
    if (!report) return [];
    const q = search.trim().toLowerCase();
    const byName = (a: Folder, b: Folder) => a.name.localeCompare(b.name);
    const comparators: Record<SortKey, (a: Folder, b: Folder) => number> = {
      name: byName,
      size: (a, b) => a.size - b.size || byName(a, b),
      status: (a, b) => Number(b.orphan) - Number(a.orphan) || byName(a, b),
    };
    return report.folders
      .filter((f) => {
        if (filter === "orphan" && !f.orphan) return false;
        if (filter === "linked" && f.orphan) return false;
        return !q || f.name.toLowerCase().includes(q) || (f.containers ?? []).some((c) => c.toLowerCase().includes(q));
      })
      .sort((a, b) => sort.dir * comparators[sort.key](a, b));
  });

  const visibleOrphans = $derived(visible.filter((f) => f.orphan));
  const selectedVisible = $derived(visibleOrphans.filter((f) => app.selected.has(f.name)).length);

  function sortBy(key: SortKey) {
    sort = sort.key === key ? { key, dir: sort.dir === 1 ? -1 : 1 } : { key, dir: key === "size" ? -1 : 1 };
  }

  function toggleAll(checked: boolean) {
    for (const f of visibleOrphans) {
      if (checked) app.selected.add(f.name);
      else app.selected.delete(f.name);
    }
  }

  function retentionNote() {
    const days = app.trash?.retention_days ?? 0;
    return days > 0 ? `Permanently deleted after ${count(days, "day")}.` : "Kept until the trash is emptied.";
  }

  function askTrash(names: string[]) {
    if (!report || names.length === 0) return;
    const size = report.folders.filter((f) => names.includes(f.name)).reduce((sum, f) => sum + f.size, 0);
    confirm = {
      title: "Move to trash",
      description: `Move ${count(names.length, "folder")} (${formatBytes(size)}) to the trash?`,
      names,
      note: `You can restore them from the trash. ${retentionNote()}`,
      confirmLabel: "Move to trash",
      onConfirm: () => app.trashFolders(names),
    };
  }
</script>

{#snippet stat(label: string, value: string | number, tone: string, sub = "")}
  <Card.Root class="gap-1 py-4">
    <Card.Header class="px-4">
      <Card.Description class="text-xs font-medium tracking-wide uppercase">{label}</Card.Description>
    </Card.Header>
    <Card.Content class="px-4">
      <p class="font-mono text-2xl font-semibold tabular-nums {tone}">{value}</p>
      {#if sub}<p class="text-muted-foreground text-xs">{sub}</p>{/if}
    </Card.Content>
  </Card.Root>
{/snippet}

{#snippet sortHead(key: SortKey, label: string)}
  <Table.Head>
    <button class="hover:text-foreground inline-flex items-center gap-1" onclick={() => sortBy(key)}>
      {label}
      {#if sort.key === key}
        {#if sort.dir === 1}<ArrowUpIcon class="size-3" />{:else}<ArrowDownIcon class="size-3" />{/if}
      {/if}
    </button>
  </Table.Head>
{/snippet}

{#if !report}
  {#if app.scan.error && !app.scan.running}
    <Alert.Root variant="destructive">
      <CircleAlertIcon />
      <Alert.Title>Scan failed</Alert.Title>
      <Alert.Description>{app.scan.error}</Alert.Description>
    </Alert.Root>
  {:else}
    <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
      {#each { length: 4 }, i (i)}<Skeleton class="h-24 rounded-xl" />{/each}
    </div>
    <Skeleton class="mt-6 h-96 rounded-xl" />
  {/if}
{:else}
  <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
    {@render stat("Folders", report.folders.length, "", `${approx}${formatBytes(report.total_size)} total`)}
    {@render stat("In use", report.used_count, "text-success")}
    {@render stat("Orphans", report.orphan_count, "text-destructive")}
    {@render stat("Orphan space", approx + formatBytes(report.orphan_size), "text-warning")}
  </div>

  {#if app.scan.error}
    <Alert.Root variant="destructive" class="mt-4">
      <CircleAlertIcon />
      <Alert.Title>Last scan failed</Alert.Title>
      <Alert.Description>{app.scan.error}</Alert.Description>
    </Alert.Root>
  {/if}

  <div class="mt-6 flex flex-wrap items-center gap-3">
    <Tabs.Root bind:value={filter}>
      <Tabs.List>
        <Tabs.Trigger value="all">All</Tabs.Trigger>
        <Tabs.Trigger value="orphan">Orphans <Badge variant="secondary" class="ml-1 px-1.5">{report.orphan_count}</Badge></Tabs.Trigger>
        <Tabs.Trigger value="linked">In use</Tabs.Trigger>
      </Tabs.List>
    </Tabs.Root>
    <span class="text-muted-foreground ml-auto text-xs">
      {#if app.scan.running}
        Computing sizes… {app.scan.done}/{app.scan.total}
      {:else}
        Scanned {formatDate(report.scanned_at)}
      {/if}
    </span>
    <div class="relative w-full sm:w-64">
      <SearchIcon class="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
      <Input type="search" placeholder="Search folders or containers…" class="pl-8" bind:value={search} />
    </div>
  </div>

  {#if app.selected.size}
    <div class="bg-destructive/10 text-destructive mt-4 flex items-center gap-3 rounded-lg px-4 py-2 text-sm font-medium">
      {app.selected.size} selected
      <Button variant="destructive" size="sm" onclick={() => askTrash([...app.selected].sort())} disabled={app.busy}>
        <TrashIcon /> Move to trash
      </Button>
      <Button variant="ghost" size="sm" class="ml-auto" onclick={() => app.selected.clear()}>
        <XIcon /> Clear selection
      </Button>
    </div>
  {/if}

  <Card.Root class="mt-4 py-0">
    {#if visible.length === 0}
      <p class="text-muted-foreground py-16 text-center text-sm">No folders match these filters.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head class="w-10 pl-4">
              <Checkbox
                aria-label="Select the orphans shown"
                disabled={visibleOrphans.length === 0}
                checked={visibleOrphans.length > 0 && selectedVisible === visibleOrphans.length}
                indeterminate={selectedVisible > 0 && selectedVisible < visibleOrphans.length}
                onCheckedChange={toggleAll}
              />
            </Table.Head>
            {@render sortHead("name", "Folder")}
            {@render sortHead("size", "Size")}
            {@render sortHead("status", "Status")}
            <Table.Head>Containers</Table.Head>
            <Table.Head class="w-0"></Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each visible as f (f.name)}
            <Table.Row class={f.orphan ? "bg-destructive/[0.03]" : ""}>
              <Table.Cell class="pl-4">
                {#if f.orphan}
                  <Checkbox
                    aria-label="Select {f.name}"
                    checked={app.selected.has(f.name)}
                    onCheckedChange={(v) => (v ? app.selected.add(f.name) : app.selected.delete(f.name))}
                  />
                {/if}
              </Table.Cell>
              <Table.Cell class="font-mono text-sm font-medium">{f.name}</Table.Cell>
              <Table.Cell class="text-muted-foreground font-mono text-sm tabular-nums">
                {#if f.size_pending && !f.size}
                  <Skeleton class="h-4 w-14" />
                {:else}
                  <span class={f.size_pending ? "opacity-50" : ""} title={f.size_pending ? "Computing…" : undefined}>
                    {formatBytes(f.size)}
                  </span>
                {/if}
              </Table.Cell>
              <Table.Cell>
                {#if f.orphan}
                  <Badge variant="destructive">Orphan</Badge>
                {:else}
                  <Badge variant="outline" class="border-success/40 text-success">In use</Badge>
                {/if}
              </Table.Cell>
              <Table.Cell>
                <div class="flex flex-wrap gap-1">
                  {#each f.containers ?? [] as c (c)}
                    <Badge variant="secondary" class="font-mono">{c}</Badge>
                  {:else}
                    <span class="text-muted-foreground">—</span>
                  {/each}
                </div>
              </Table.Cell>
              <Table.Cell class="pr-4">
                {#if f.orphan}
                  <Button variant="ghost" size="icon-sm" aria-label="Move {f.name} to trash" title="Move to trash"
                    onclick={() => askTrash([f.name])} disabled={app.busy}>
                    <TrashIcon class="text-destructive" />
                  </Button>
                {/if}
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Root>
{/if}

<ConfirmDialog bind:request={confirm} />
