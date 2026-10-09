<script lang="ts">
  import ArrowLeftIcon from "@lucide/svelte/icons/arrow-left";
  import TrashIcon from "@lucide/svelte/icons/trash-2";
  import UndoIcon from "@lucide/svelte/icons/undo-2";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import { Skeleton } from "$lib/components/ui/skeleton";
  import * as Table from "$lib/components/ui/table";
  import type { TrashItem } from "$lib/api";
  import { count, formatBytes, formatDate } from "$lib/format";
  import { app } from "$lib/state.svelte";
  import ConfirmDialog, { type ConfirmRequest } from "./ConfirmDialog.svelte";

  let confirm = $state<ConfirmRequest | null>(null);

  const items = $derived(app.trash?.items ?? []);
  const total = $derived(items.reduce((sum, i) => sum + i.size, 0));
  const retention = $derived.by(() => {
    const days = app.trash?.retention_days ?? 0;
    return days > 0 ? `Folders are permanently deleted after ${count(days, "day")}.` : "Folders are kept until the trash is emptied.";
  });

  $effect(() => {
    void app.loadTrash();
  });

  function askPurge(selection: TrashItem[]) {
    if (selection.length === 0) return;
    const size = selection.reduce((sum, i) => sum + i.size, 0);
    confirm = {
      title: "Delete permanently",
      description: `Permanently delete ${count(selection.length, "folder")} (${formatBytes(size)})?`,
      names: selection.map((i) => i.name),
      note: "This cannot be undone.",
      confirmLabel: "Delete permanently",
      onConfirm: () => app.purge(selection.map((i) => i.id)),
    };
  }
</script>

<div class="flex flex-wrap items-center gap-3">
  <Button variant="ghost" size="sm" onclick={() => (app.view = "folders")}>
    <ArrowLeftIcon /> Folders
  </Button>
  <div>
    <h2 class="text-lg font-semibold">Trash</h2>
    <p class="text-muted-foreground text-sm">
      {#if items.length}{count(items.length, "folder")}, {formatBytes(total)}.{/if}
      {retention}
    </p>
  </div>
  <Button variant="destructive" size="sm" class="ml-auto" disabled={items.length === 0 || app.busy}
    onclick={() => askPurge(items)}>
    <TrashIcon /> Empty trash
  </Button>
</div>

<Card.Root class="mt-4 py-0">
  {#if !app.trash}
    <div class="space-y-3 p-4">
      {#each { length: 3 }, i (i)}<Skeleton class="h-8" />{/each}
    </div>
  {:else if items.length === 0}
    <p class="text-muted-foreground py-16 text-center text-sm">The trash is empty.</p>
  {:else}
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="pl-4">Folder</Table.Head>
          <Table.Head>Deleted</Table.Head>
          <Table.Head>Size</Table.Head>
          <Table.Head class="w-0"></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each items as item (item.id)}
          <Table.Row>
            <Table.Cell class="pl-4 font-mono text-sm font-medium">{item.name}</Table.Cell>
            <Table.Cell class="text-muted-foreground text-sm">{formatDate(item.deleted_at)}</Table.Cell>
            <Table.Cell class="text-muted-foreground font-mono text-sm tabular-nums">{formatBytes(item.size)}</Table.Cell>
            <Table.Cell class="pr-4">
              <div class="flex justify-end gap-1">
                <Button variant="outline" size="sm" disabled={app.busy} onclick={() => app.restore([item.id])}>
                  <UndoIcon /> Restore
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Delete {item.name} permanently"
                  title="Delete permanently" disabled={app.busy} onclick={() => askPurge([item])}>
                  <TrashIcon class="text-destructive" />
                </Button>
              </div>
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  {/if}
</Card.Root>

<ConfirmDialog bind:request={confirm} />
