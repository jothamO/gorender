import { useEffect } from 'react';

// ── gorender contract ──────────────────────────────────────────────────────
// Two hooks: useFrame() gives you the current time,
// useReady() signals gorender when the frame is painted.

function useFrame() {
  const params = new URLSearchParams(window.location.search);
  const frame  = parseInt(params.get('frame') ?? '0', 10);
  const fps    = {{.FPS}};
  return {
    frame,
    fps,
    t: frame / fps,           // seconds
    progress: frame / {{.DurationFrames}}, // 0 → 1
  };
}

function useReady(isReady = true) {
  useEffect(() => {
    if (!isReady) return;
    document.fonts.ready.then(() => {
      window.__READY__ = true;
    });
  }, [isReady]);
}

// ── Utilities ──────────────────────────────────────────────────────────────
const clamp   = (v, min = 0, max = 1) => Math.max(min, Math.min(max, v));
const lerp    = (a, b, t) => a + (b - a) * t;
const easeOut = t => 1 - (1 - t) ** 2;

// ── Composition ────────────────────────────────────────────────────────────
// Drive all styles deterministically from t.
// No useEffect-based animations, no timers, no Date.now().

export default function Composition() {
  const { t, fps } = useFrame();
  const totalSecs = {{.DurationFrames}} / fps;

  const fadeIn   = clamp(t / 0.5);
  const fadeOut  = clamp((totalSecs - t) / 0.5);
  const opacity  = Math.min(fadeIn, fadeOut);
  const slideY   = lerp(40, 0, easeOut(clamp(t / 0.5)));

  // Signal gorender once all hooks have run and DOM is painted.
  useReady(true);

  return (
    <div style={styles.composition}>

      <div style={{
        ...styles.headline,
        opacity,
        transform: `translate(-50%, calc(-50% + ${slideY}px))`,
      }}>
        {{.Name}}
      </div>

      {/* Add your elements here */}

    </div>
  );
}

// ── Styles ─────────────────────────────────────────────────────────────────
// Use px values based on 1080×1920. Never vw/vh.
const styles = {
  composition: {
    width:    1080,
    height:   1920,
    overflow: 'hidden',
    background: '#0a0a0a',
    position: 'relative',
  },
  headline: {
    position:  'absolute',
    top:       '50%',
    left:      '50%',
    color:     'white',
    fontSize:  72,
    fontWeight: 700,
    textAlign: 'center',
  },
};
