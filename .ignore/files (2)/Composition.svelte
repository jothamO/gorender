<script>
  // ── gorender contract ──────────────────────────────────────────────────
  // Svelte runs at build time — the output is vanilla JS with no runtime.
  // gorender navigates to: ?frame=0, ?frame=1, ?frame=2 ...

  const params = new URLSearchParams(location.search);
  const frame  = parseInt(params.get('frame') ?? '0', 10);
  const fps    = {{.FPS}};
  const t      = frame / fps; // current time in seconds
  const total  = {{.DurationFrames}};
  const totalSecs = total / fps;

  // ── Utilities ────────────────────────────────────────────────────────
  const clamp   = (v, min = 0, max = 1) => Math.max(min, Math.min(max, v));
  const lerp    = (a, b, t) => a + (b - a) * t;
  const easeOut = t => 1 - (1 - t) ** 2;

  // ── Derived animation values ─────────────────────────────────────────
  // Compute everything from t. Svelte's reactivity handles the DOM updates.

  $: fadeIn  = clamp(t / 0.5);
  $: fadeOut = clamp((totalSecs - t) / 0.5);
  $: opacity = Math.min(fadeIn, fadeOut);
  $: translateY = lerp(40, 0, easeOut(clamp(t / 0.5)));

  // ── Signal ready after mount ─────────────────────────────────────────
  import { onMount } from 'svelte';

  onMount(async () => {
    await document.fonts.ready;
    // If you have images, wait for them:
    // await Promise.all(Array.from(document.images).map(waitForImage));
    window.__READY__ = true;
  });
</script>

<!--
  The composition always renders at 1080×1920.
  Never use vw/vh — use px or % of this container.
-->
<div class="composition">

  <div
    class="headline"
    style="opacity: {opacity}; transform: translate(-50%, calc(-50% + {translateY}px))"
  >
    {{.Name}}
  </div>

  <!-- Add your elements here -->

</div>

<style>
  :global(*) { margin: 0; padding: 0; box-sizing: border-box; }

  .composition {
    width: 1080px;
    height: 1920px;
    overflow: hidden;
    background: #0a0a0a;
    position: relative;
  }

  .headline {
    position: absolute;
    top: 50%;
    left: 50%;
    color: white;
    font-size: 72px;
    font-weight: 700;
    text-align: center;
  }
</style>
