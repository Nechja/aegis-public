<script lang="ts">
  import { celebrations, dismissCelebration } from './toasts.svelte';
</script>

{#each celebrations.active as c (c.id)}
  <div class="celebrate" data-kind={c.kind} style="--dur: {c.duration}ms">
    {#each c.particles as p (p.id)}
      <span
        class="particle c-{p.color}"
        style="--x: {p.x}vw; --y: {p.y}vh; --r: {p.rot}deg; --delay: {p.delay}ms; --size: {p.size}px"
      ></span>
    {/each}

    {#if c.hero}
      <div class="hero-mascot" style="--hdur: {c.duration}ms">
        <span class="face">{c.hero}</span>
      </div>
    {/if}
  </div>
{/each}

<style>
  /* scoped wrapper only — individual visuals live in app.css to use vars */
</style>
