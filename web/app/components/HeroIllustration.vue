<script setup lang="ts">
// Ilustração do banner: caixas de frutas em perspectiva, desenhadas em SVG
// (sem depender de foto). As caixas usam as cores do tema; as frutas, as suas.
type Fruit = 'apple' | 'orange' | 'pear' | 'lime'

// Gerador pseudoaleatório com semente fixa: o desenho sai sempre igual.
let seed = 7
const rand = () => {
  seed = (seed * 9301 + 49297) % 233280
  return seed / 233280
}

const rows = [
  { y: 62, scale: 0.62, x0: 150, count: 6 },
  { y: 138, scale: 0.8, x0: 95, count: 5 },
  { y: 228, scale: 1, x0: 40, count: 5 },
]
const pattern: Fruit[] = ['apple', 'orange', 'pear', 'apple', 'lime', 'orange', 'pear']

const crates = rows.flatMap((row, ri) =>
  Array.from({ length: row.count }, (_, i) => {
    const w = 128 * row.scale
    const h = 62 * row.scale
    const x = row.x0 + i * (w + 10 * row.scale)
    const type = pattern[(i + ri * 2) % pattern.length]!
    const r = 13 * row.scale
    const fruits: { cx: number, cy: number, r: number }[] = []
    // Duas camadas de frutas, a de cima um pouco deslocada (monte).
    for (let layer = 0; layer < 2; layer++) {
      const n = Math.floor((w - 6) / (r * 2)) - layer
      for (let k = 0; k < n; k++) {
        fruits.push({
          cx: x + 4 + r + layer * r + k * r * 2 + (rand() - 0.5) * r * 0.3,
          cy: row.y - layer * r * 1.15 + (rand() - 0.5) * r * 0.25,
          r: r * (0.9 + rand() * 0.15),
        })
      }
    }
    return { x, y: row.y, w, h, type, fruits }
  }),
)
</script>

<template>
  <svg viewBox="0 0 680 320" preserveAspectRatio="xMidYMid slice" class="size-full" aria-hidden="true">
    <defs>
      <radialGradient id="hero-apple" cx="35%" cy="30%" r="75%">
        <stop offset="0%" stop-color="#ff6b6b" />
        <stop offset="55%" stop-color="#d62839" />
        <stop offset="100%" stop-color="#7d0f1c" />
      </radialGradient>
      <radialGradient id="hero-orange" cx="35%" cy="30%" r="75%">
        <stop offset="0%" stop-color="#ffc061" />
        <stop offset="55%" stop-color="#f58a07" />
        <stop offset="100%" stop-color="#a84a00" />
      </radialGradient>
      <radialGradient id="hero-pear" cx="35%" cy="30%" r="75%">
        <stop offset="0%" stop-color="#f4f7a1" />
        <stop offset="55%" stop-color="#c6d65a" />
        <stop offset="100%" stop-color="#6f8a1e" />
      </radialGradient>
      <radialGradient id="hero-lime" cx="35%" cy="30%" r="75%">
        <stop offset="0%" stop-color="#b9f08c" />
        <stop offset="55%" stop-color="#5fb33a" />
        <stop offset="100%" stop-color="#2d6a17" />
      </radialGradient>
      <linearGradient id="hero-fade" x1="0" x2="1" y1="0" y2="0">
        <stop offset="0%" stop-color="white" stop-opacity="0" />
        <stop offset="35%" stop-color="white" stop-opacity="1" />
      </linearGradient>
      <mask id="hero-mask">
        <rect width="680" height="320" fill="url(#hero-fade)" />
      </mask>
    </defs>

    <g mask="url(#hero-mask)">
      <g v-for="(crate, i) in crates" :key="i">
        <!-- frutas -->
        <g v-for="(f, j) in crate.fruits" :key="j">
          <circle :cx="f.cx" :cy="f.cy" :r="f.r" :fill="`url(#hero-${crate.type})`" />
          <ellipse :cx="f.cx - f.r * 0.35" :cy="f.cy - f.r * 0.4" :rx="f.r * 0.28" :ry="f.r * 0.16" fill="white" opacity="0.45" />
        </g>
        <!-- caixa (na cor do tema) com ripas -->
        <rect :x="crate.x" :y="crate.y" :width="crate.w" :height="crate.h" rx="4" class="fill-brand-800" />
        <rect :x="crate.x" :y="crate.y" :width="crate.w" :height="crate.h * 0.16" rx="3" class="fill-brand-600" opacity="0.9" />
        <line
          v-for="s in 3"
          :key="s"
          :x1="crate.x + 4"
          :x2="crate.x + crate.w - 4"
          :y1="crate.y + crate.h * (0.16 + s * 0.22)"
          :y2="crate.y + crate.h * (0.16 + s * 0.22)"
          class="stroke-brand-950"
          stroke-width="2"
          opacity="0.35"
        />
      </g>
    </g>
  </svg>
</template>
