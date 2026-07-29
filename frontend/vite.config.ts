import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {defineConfig, type ResolvedConfig} from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// main.go embeds frontend/dist, so the folder has to exist in a fresh clone
// before anything Go compiles. The keep file is the only committed thing in
// there, and a build wipes the folder, so put it back afterwards.
function keepDist() {
  let outDir = 'dist'
  return {
    name: 'chit-keep-dist',
    configResolved(config: ResolvedConfig) {
      outDir = resolve(config.root, config.build.outDir)
    },
    closeBundle() {
      writeFileSync(resolve(outDir, '.gitkeep'), '')
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), keepDist()]
})
