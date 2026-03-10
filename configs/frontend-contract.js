/**
 * gorender frontend contract
 *
 * Your web composition must implement this contract to be renderable by gorender.
 * Framework doesn't matter — React, Vue, Svelte, vanilla JS all work.
 *
 * THE CONTRACT:
 *   1. Read the current frame from the `frame` query parameter.
 *   2. Render your composition deterministically for that frame.
 *   3. Set window.__READY__ = true once the frame is fully painted.
 *
 * gorender will:
 *   - Navigate to your URL with ?frame=N for each frame
 *   - Poll window.__READY__ until true (or timeout)
 *   - Screenshot the viewport
 *   - Move to the next frame
 */

// ─── Vanilla JS example ────────────────────────────────────────────────────

const params = new URLSearchParams(window.location.search);
const currentFrame = parseInt(params.get('frame') ?? '0', 10);
const fps = 30;
const currentTime = currentFrame / fps; // seconds

// Use currentTime to drive your animations.
// Example: CSS custom property approach
document.documentElement.style.setProperty('--t', currentTime.toString());

// Wait for fonts + images, then signal ready.
document.fonts.ready.then(() => {
  // If you have images, wait for them too.
  const images = Array.from(document.images);
  const imageLoads = images.map(img =>
    img.complete ? Promise.resolve() : new Promise(r => { img.onload = r; img.onerror = r; })
  );
  Promise.all(imageLoads).then(() => {
    // All assets loaded. Signal gorender.
    window.__READY__ = true;
  });
});


// ─── React hook example ────────────────────────────────────────────────────

/*
import { useEffect, useState } from 'react';

function useGorender() {
  const params = new URLSearchParams(window.location.search);
  const frame = parseInt(params.get('frame') ?? '0', 10);
  const fps = 30;
  const t = frame / fps;

  return { frame, t, fps };
}

function useGorenderReady(isReady: boolean) {
  useEffect(() => {
    if (isReady) {
      (window as any).__READY__ = true;
    }
  }, [isReady]);
}

// Usage in a component:
function MyComposition() {
  const { frame, t } = useGorender();
  const [fontsLoaded, setFontsLoaded] = useState(false);

  useEffect(() => {
    document.fonts.ready.then(() => setFontsLoaded(true));
  }, []);

  useGorenderReady(fontsLoaded);

  return (
    <div style={{ opacity: Math.min(1, t / 0.5) }}> // fade in over 0.5s
      Frame {frame} — {t.toFixed(2)}s
    </div>
  );
}
*/


// ─── Easing utilities you might want ───────────────────────────────────────

const ease = {
  // t is 0..1
  inOut: t => t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t,
  in:    t => t * t,
  out:   t => t * (2 - t),
  // Clamp t to a window within the full composition.
  // E.g. clamp(currentTime, startSec, endSec) → 0..1
  clamp: (t, start, end) => Math.max(0, Math.min(1, (t - start) / (end - start))),
};
