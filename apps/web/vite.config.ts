import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react' // หรือ plugin ที่คุณใช้อยู่

export default defineConfig({
  plugins: [react()],
  server: {
    allowedHosts: ['cellular-debian-highlighted-rolled.trycloudflare.com']
  }
})