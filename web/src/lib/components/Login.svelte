<script lang="ts">
  import CircleAlertIcon from "@lucide/svelte/icons/circle-alert";
  import ContainerIcon from "@lucide/svelte/icons/container";
  import LoaderIcon from "@lucide/svelte/icons/loader-circle";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { api } from "$lib/api";
  import { app } from "$lib/state.svelte";

  let user = $state("");
  let password = $state("");
  let error = $state("");
  let pending = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    pending = true;
    try {
      await api.login(user, password);
      await app.init();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      password = "";
    } finally {
      pending = false;
    }
  }
</script>

<main class="flex min-h-svh items-center justify-center p-4">
  <Card.Root class="w-full max-w-sm">
    <Card.Header class="items-center text-center">
      <div class="bg-primary text-primary-foreground mx-auto mb-2 flex size-10 items-center justify-center rounded-lg">
        <ContainerIcon class="size-5" />
      </div>
      <Card.Title class="text-xl">Appdata Cleaner</Card.Title>
      <Card.Description>Log in to manage your appdata folders.</Card.Description>
    </Card.Header>
    <Card.Content>
      <form class="grid gap-4" onsubmit={submit}>
        <div class="grid gap-2">
          <Label for="user">User</Label>
          <Input id="user" autocomplete="username" required bind:value={user} autofocus />
        </div>
        <div class="grid gap-2">
          <Label for="password">Password</Label>
          <Input id="password" type="password" autocomplete="current-password" required bind:value={password} />
        </div>
        {#if error}
          <Alert.Root variant="destructive">
            <CircleAlertIcon />
            <Alert.Title>{error}</Alert.Title>
          </Alert.Root>
        {/if}
        <Button type="submit" class="w-full" disabled={pending}>
          {#if pending}<LoaderIcon class="animate-spin" />{/if}
          Log in
        </Button>
      </form>
    </Card.Content>
  </Card.Root>
</main>
