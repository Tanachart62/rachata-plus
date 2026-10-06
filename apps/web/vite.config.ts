import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react' // หรือ plugin ที่คุณใช้อยู่

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5174,
    strictPort: true,
    proxy: {
      '/auth': { target: 'http://127.0.0.1:8080' },
      '/api': { target: 'http://127.0.0.1:8080' },
      '/media': { target: 'http://127.0.0.1:8080' },
    },
    allowedHosts: ['cellular-debian-highlighted-rolled.trycloudflare.com']
  }
})
