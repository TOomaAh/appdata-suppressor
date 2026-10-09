<script lang="ts" module>
  export interface ConfirmRequest {
    title: string;
    description: string;
    /** Folder names listed under the description. */
    names: string[];
    note: string;
    confirmLabel: string;
    onConfirm: () => void;
  }
</script>

<script lang="ts">
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { buttonVariants } from "$lib/components/ui/button";

  let { request = $bindable() }: { request: ConfirmRequest | null } = $props();

  function confirm() {
    const action = request?.onConfirm;
    request = null;
    action?.();
  }
</script>

<AlertDialog.Root open={request !== null} onOpenChange={(open) => !open && (request = null)}>
  <AlertDialog.Content>
    {#if request}
      <AlertDialog.Header>
        <AlertDialog.Title>{request.title}</AlertDialog.Title>
        <AlertDialog.Description>{request.description}</AlertDialog.Description>
      </AlertDialog.Header>
      <div class="flex max-h-40 flex-wrap gap-1.5 overflow-y-auto">
        {#each request.names as name (name)}
          <code class="bg-muted rounded px-1.5 py-0.5 font-mono text-xs">{name}</code>
        {/each}
      </div>
      <p class="text-muted-foreground text-sm">{request.note}</p>
      <AlertDialog.Footer>
        <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
        <AlertDialog.Action class={buttonVariants({ variant: "destructive" })} onclick={confirm}>
          {request.confirmLabel}
        </AlertDialog.Action>
      </AlertDialog.Footer>
    {/if}
  </AlertDialog.Content>
</AlertDialog.Root>
