import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api-gateway': {
				target: 'http://localhost:18080',
				rewrite: (path) => path.replace(/^\/api-gateway/, '')
			},
			'/ws': {
				target: 'ws://localhost:18080',
				ws: true
			}
		}
	}
});
