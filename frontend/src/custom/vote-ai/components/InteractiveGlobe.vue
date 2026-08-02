<template>
  <div class="globe-stage" :class="{ 'is-dark': isDark }">
    <div
      ref="shell"
      class="globe-shell"
      :class="{ dragging: isDragging }"
      @pointerdown="pointerDown"
      @pointermove="pointerMove"
      @pointerup="pointerUp"
      @pointercancel="pointerUp"
      @selectstart.prevent
      @dragstart.prevent
    >
      <canvas ref="canvas" aria-label="可拖动的全球节点网络"></canvas>
      <svg ref="connectionSvg" class="connections" preserveAspectRatio="none" aria-hidden="true">
        <line
          v-for="(connection, index) in connections"
          :key="connection.join('-')"
          :ref="(element) => setConnectionElement(element as SVGLineElement | null, index)"
          class="connection-line"
        />
      </svg>
      <div class="labels" aria-hidden="true">
        <span
          v-for="(label, index) in labels"
          :key="label.name"
          :ref="(element) => setLabelElement(element as HTMLElement | null, index)"
          class="globe-label"
          :class="{ main: label.main }"
        >{{ label.name }}</span>
      </div>
      <div class="drag-tip" :class="{ hidden: hasInteracted }">拖动探索全球节点</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import createGlobe from 'cobe'

const props = defineProps<{ isDark: boolean }>()

type GlobeLabel = {
  name: string
  location: [number, number]
  markerSize: number
  main?: boolean
}

const shell = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const connectionSvg = ref<SVGSVGElement | null>(null)
const isDragging = ref(false)
const hasInteracted = ref(false)

const labels: GlobeLabel[] = [
  { name: 'Vote', location: [0, -160], markerSize: 0.065, main: true },
  { name: 'CHINA', location: [39.9042, 116.4074], markerSize: 0.042 },
  { name: 'JAPAN', location: [35.6762, 139.6503], markerSize: 0.038 },
  { name: 'USA', location: [38.9072, -77.0369], markerSize: 0.042 },
  { name: 'AUSTRALIA', location: [-35.2809, 149.13], markerSize: 0.04 }
]
const connections = labels.slice(1).map((_, index) => [0, index + 1] as const)
const labelElements: HTMLElement[] = []
const connectionElements: SVGLineElement[] = []

let globe: ReturnType<typeof createGlobe> | undefined
let resizeObserver: ResizeObserver | undefined
// Initial view faces the central Pacific, keeping Vote near the visual center.
let phi = -2.8
let theta = 0
let phiVelocity = 0
let thetaVelocity = 0
let pointerId: number | null = null
let lastX = 0
let lastY = 0
let lastTime = 0
let width = 520
let darkValue = props.isDark ? 1 : 0

watch(
  () => props.isDark,
  (value) => {
    darkValue = value ? 1 : 0
  }
)

function setLabelElement(element: HTMLElement | null, index: number) {
  if (element) labelElements[index] = element
}

function setConnectionElement(element: SVGLineElement | null, index: number) {
  if (element) connectionElements[index] = element
}

function smoothstep(min: number, max: number, value: number) {
  const normalized = Math.max(0, Math.min(1, (value - min) / (max - min)))
  return normalized * normalized * (3 - 2 * normalized)
}

function projectLocation(location: [number, number], rotationPhi: number, rotationTheta: number) {
  const latitude = (location[0] * Math.PI) / 180
  const longitude = (location[1] * Math.PI) / 180
  const cosLatitude = Math.cos(latitude)
  const markerX = cosLatitude * Math.cos(longitude)
  const markerY = Math.sin(latitude)
  const markerZ = -cosLatitude * Math.sin(longitude)
  const cosPhi = Math.cos(rotationPhi)
  const sinPhi = Math.sin(rotationPhi)
  const cosTheta = Math.cos(rotationTheta)
  const sinTheta = Math.sin(rotationTheta)
  const screenX = cosPhi * markerX + sinPhi * markerZ
  const screenY = sinPhi * sinTheta * markerX + cosTheta * markerY - cosPhi * sinTheta * markerZ
  const depth = -sinPhi * cosTheta * markerX + sinTheta * markerY + cosPhi * cosTheta * markerZ
  const radius = 40 * 0.92

  // Keep a node pinned to the globe rim for a while after it rotates behind the horizon.
  // Once it passes the buffer depth it fades out; on return it travels from the rim
  // back to its real projected position instead of popping in abruptly.
  const projectedRadius = Math.hypot(screenX, screenY)
  const isBehindHorizon = depth < 0
  const rimScale = isBehindHorizon && projectedRadius > 0 ? 1 / projectedRadius : 1
  const displayX = screenX * rimScale
  const displayY = screenY * rimScale
  const visibility = isBehindHorizon ? smoothstep(-0.48, 0, depth) : 1

  return {
    x: 50 + displayX * radius,
    y: 50 - displayY * radius,
    visible: visibility,
    scale: 0.9 + visibility * 0.1
  }
}

function updateOverlays(rotationPhi: number, rotationTheta: number) {
  const points = labels.map((label, index) => {
    const projected = projectLocation(label.location, rotationPhi, rotationTheta)
    const point = {
      x: (projected.x / 100) * width,
      y: (projected.y / 100) * width,
      visible: projected.visible,
      scale: projected.scale
    }
    const element = labelElements[index]
    if (element) {
      element.style.transform = `translate3d(${point.x}px, ${point.y}px, 0) translate(-50%, calc(-100% - 10px)) scale(${point.scale})`
      element.style.opacity = String(point.visible)
      element.style.visibility = point.visible > 0.02 ? 'visible' : 'hidden'
    }
    return point
  })

  connections.forEach(([fromIndex, toIndex], index) => {
    const line = connectionElements[index]
    if (!line) return
    const from = points[fromIndex]
    const to = points[toIndex]
    const endpointVisibility = Math.min(from.visible, to.visible)
    const opacity = smoothstep(0.02, 0.35, endpointVisibility) * 0.52
    line.setAttribute('x1', String(from.x))
    line.setAttribute('y1', String(from.y))
    line.setAttribute('x2', String(to.x))
    line.setAttribute('y2', String(to.y))
    line.style.opacity = String(opacity)
    line.style.visibility = opacity > 0.02 ? 'visible' : 'hidden'
  })
}

function normalizeAngle(angle: number) {
  return Math.atan2(Math.sin(angle), Math.cos(angle))
}

function resize() {
  if (!shell.value || !canvas.value) return
  width = Math.min(shell.value.clientWidth, 560)
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  canvas.value.style.width = `${width}px`
  canvas.value.style.height = `${width}px`
  canvas.value.width = width * dpr
  canvas.value.height = width * dpr
  connectionSvg.value?.setAttribute('viewBox', `0 0 ${width} ${width}`)
}

function pointerDown(event: PointerEvent) {
  if (!shell.value) return
  event.preventDefault()
  window.getSelection()?.removeAllRanges()
  pointerId = event.pointerId
  isDragging.value = true
  hasInteracted.value = true
  lastX = event.clientX
  lastY = event.clientY
  lastTime = performance.now()
  phiVelocity = 0
  thetaVelocity = 0
  shell.value.setPointerCapture(pointerId)
}

function pointerMove(event: PointerEvent) {
  if (!isDragging.value || event.pointerId !== pointerId) return
  event.preventDefault()
  const now = performance.now()
  const dx = event.clientX - lastX
  const dy = event.clientY - lastY
  const dt = Math.max(now - lastTime, 8)
  phi = normalizeAngle(phi + dx / 180)
  theta = normalizeAngle(theta - dy / 220)
  phiVelocity = (dx / dt) * 0.016
  thetaVelocity = (-dy / dt) * 0.012
  lastX = event.clientX
  lastY = event.clientY
  lastTime = now
}

function pointerUp(event: PointerEvent) {
  if (pointerId !== event.pointerId) return
  isDragging.value = false
  pointerId = null
}

onMounted(async () => {
  await nextTick()
  if (!shell.value || !canvas.value) return
  resize()
  resizeObserver = new ResizeObserver(resize)
  resizeObserver.observe(shell.value)
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const dpr = Math.min(window.devicePixelRatio || 1, 2)

  globe = createGlobe(canvas.value, {
    devicePixelRatio: dpr,
    width: width * dpr,
    height: width * dpr,
    phi,
    theta,
    dark: darkValue,
    diffuse: 1.05,
    mapSamples: 14000,
    mapBrightness: props.isDark ? 4.2 : 5.2,
    mapBaseBrightness: props.isDark ? 0.04 : 0.22,
    baseColor: props.isDark ? [0.34, 0.26, 0.22] : [0.94, 0.86, 0.81],
    markerColor: [0.77, 0.32, 0],
    glowColor: props.isDark ? [0.12, 0.08, 0.06] : [1, 0.995, 0.98],
    opacity: 0.7,
    scale: 0.94,
    offset: [0, 0],
    markers: labels.map(({ location, markerSize }) => ({ location, size: markerSize })),
    onRender: (state) => {
      const currentDpr = Math.min(window.devicePixelRatio || 1, 2)
      state.width = width * currentDpr
      state.height = width * currentDpr
      state.dark = darkValue
      state.mapBrightness = darkValue ? 4.2 : 5.2
      state.mapBaseBrightness = darkValue ? 0.04 : 0.22
      state.baseColor = darkValue ? [0.34, 0.26, 0.22] : [0.94, 0.86, 0.81]
      state.glowColor = darkValue ? [0.12, 0.08, 0.06] : [1, 0.995, 0.98]
      if (!isDragging.value) {
        phiVelocity *= 0.94
        thetaVelocity *= 0.92
        phi = normalizeAngle(phi + phiVelocity + (reducedMotion ? 0 : 0.0018))
        theta = normalizeAngle(theta + thetaVelocity)
      }
      state.phi = phi
      state.theta = theta
      updateOverlays(phi, theta)
    }
  })
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  globe?.destroy()
})
</script>

<style scoped>
.globe-stage {
  position: relative;
  display: grid;
  width: min(100%, 560px);
  place-items: center;
}

.globe-shell {
  position: relative;
  width: 100%;
  aspect-ratio: 1;
  cursor: grab;
  isolation: isolate;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
}

.globe-shell.dragging {
  cursor: grabbing;
}

.globe-shell::before {
  position: absolute;
  z-index: -1;
  inset: 10%;
  border: 1px solid rgba(196, 81, 0, 0.07);
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.72);
  content: '';
}

.globe-shell::after {
  position: absolute;
  z-index: -2;
  inset: 17%;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 0 72px 36px rgba(255, 250, 244, 0.92);
  content: '';
}

.is-dark .globe-shell::before {
  border-color: rgba(223, 123, 66, 0.14);
  background: rgba(69, 50, 41, 0.28);
}

.is-dark .globe-shell::after {
  background: rgba(48, 35, 29, 0.38);
  box-shadow: 0 0 72px 36px rgba(62, 22, 0, 0.18);
}

canvas {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  user-select: none;
}

.connections,
.labels {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.connections {
  z-index: 1;
  overflow: visible;
}

.labels {
  z-index: 3;
}

.connection-line {
  fill: none;
  stroke: rgba(156, 63, 0, 0.68);
  stroke-dasharray: 4 6;
  stroke-linecap: round;
  stroke-width: 1.2;
  vector-effect: non-scaling-stroke;
}

.globe-label {
  position: absolute;
  top: 0;
  left: 0;
  padding: 5px 9px;
  transform-origin: 50% 100%;
  border: 1px solid rgba(88, 66, 56, 0.1);
  border-radius: 7px;
  background: rgba(252, 249, 244, 0.9);
  box-shadow: 0 4px 12px rgba(62, 22, 0, 0.06);
  color: #584238;
  font-size: 11px;
  will-change: transform, opacity;
}

.globe-label::after {
  position: absolute;
  top: 100%;
  left: 50%;
  width: 1px;
  height: 8px;
  transform: translateX(-50%);
  background: rgba(156, 63, 0, 0.35);
  content: '';
}

.globe-label.main {
  padding-inline: 11px;
  border-color: transparent;
  background: rgba(156, 63, 0, 0.92);
  color: white;
  font-weight: 600;
}

.is-dark .globe-label {
  border-color: rgba(223, 123, 66, 0.16);
  background: rgba(48, 35, 29, 0.9);
  color: #eadfd8;
}

.drag-tip {
  position: absolute;
  bottom: 7%;
  left: 50%;
  padding: 8px 12px;
  transform: translateX(-50%);
  transition: opacity 0.18s linear;
  border: 1px solid rgba(88, 66, 56, 0.1);
  border-radius: 999px;
  background: rgba(255, 253, 249, 0.84);
  color: #755f54;
  font-size: 11px;
  pointer-events: none;
  white-space: nowrap;
}

.is-dark .drag-tip {
  border-color: rgba(223, 123, 66, 0.14);
  background: rgba(48, 35, 29, 0.88);
  color: #d8c7bd;
}

.drag-tip.hidden {
  transform: translate(-50%, 8px);
  opacity: 0;
}

@media (max-width: 680px) {
  .globe-stage {
    width: min(88vw, 390px);
  }

  .globe-label {
    padding: 4px 7px;
    font-size: 9px;
  }

  .drag-tip {
    bottom: 4%;
    padding: 6px 9px;
    font-size: 9px;
  }
}
</style>
