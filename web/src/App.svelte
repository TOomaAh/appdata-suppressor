<script lang="ts">
  import { ModeWatcher } from "mode-watcher";
  import { onMount } from "svelte";
  import FoldersView from "$lib/components/FoldersView.svelte";
  import Header from "$lib/components/Header.svelte";
  import Login from "$lib/components/Login.svelte";
  import TrashView from "$lib/components/TrashView.svelte";
  import { Toaster } from "$lib/components/ui/sonner";
  import { app } from "$lib/state.svelte";

  onMount(() => {
    void app.init();
  });
</script>

<ModeWatcher defaultMode="dark" />
<Toaster richColors position="bottom-right" />

{#if app.loggedIn === false}
  <Login />
{:else if app.loggedIn}
  <Header />
  <main class="mx-auto max-w-6xl px-4 py-6">
    {#if app.view === "trash"}
      <TrashView />
    {:else}
      <FoldersView />
    {/if}
  </main>
{/if}
