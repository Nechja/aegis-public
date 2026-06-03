<script lang="ts">
  import { store, dismiss } from './toasts.svelte';
</script>

<div class="toasts" aria-live="polite" aria-atomic="false">
  {#each store.toasts as t (t.id)}
    <div class="toast" data-tone={t.tone}>
      <button
        type="button"
        class="toast-body"
        onclick={() => dismiss(t.id)}
        aria-label="dismiss"
      >
        <span class="face" aria-hidden="true">{t.face}</span>
        <div class="toast-col">
          {#if t.source === 'shoppy-fren'}
            <span class="src">shoppy-fren</span>
          {/if}
          <span class="msg">{t.text}</span>
        </div>
      </button>
    </div>
  {/each}
</div>
